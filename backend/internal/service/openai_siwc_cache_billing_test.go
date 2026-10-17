//go:build unit

package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/siwc"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSIWCCacheCreationForwardAndBilling(t *testing.T) {
	cases := []struct {
		name                       string
		usage                      string
		wantInput, wantRead, write int
	}{
		{"reported", `{"input_tokens":1000,"output_tokens":50,"input_tokens_details":{"cached_tokens":100,"cache_write_tokens":200}}`, 700, 100, 200},
		{"missing_write", `{"input_tokens":1000,"output_tokens":50,"input_tokens_details":{"cached_tokens":100}}`, 900, 100, 0},
		{"explicit_zero_over_alias", `{"input_tokens":1000,"output_tokens":50,"cache_creation_input_tokens":200,"input_tokens_details":{"cached_tokens":100,"cache_write_tokens":0}}`, 900, 100, 0},
		{"all_read", `{"input_tokens":1000,"output_tokens":50,"input_tokens_details":{"cached_tokens":1000,"cache_write_tokens":0}}`, 0, 1000, 0},
		{"all_write", `{"input_tokens":1000,"output_tokens":50,"input_tokens_details":{"cached_tokens":0,"cache_write_tokens":1000}}`, 0, 0, 1000},
	}
	for _, model := range []string{"gpt-6-astra", "gpt-6.1-sol"} {
		for _, protocol := range []string{"responses", "chat", "messages"} {
			for _, stream := range []bool{false, true} {
				for _, tt := range cases {
					t.Run(fmt.Sprintf("%s/%s/stream=%t/%s", model, protocol, stream, tt.name), func(t *testing.T) {
						path := "/v1/responses"
						body := []byte(fmt.Sprintf(`{"model":%q,"input":[{"role":"user","content":"hello"}],"stream":%t}`, model, stream))
						if protocol != "responses" {
							path = "/v1/chat/completions"
							if protocol == "messages" {
								path = "/v1/messages"
							}
							body = []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"hello"}],"max_tokens":64,"stream":%t}`, model, stream))
						}
						recorder := httptest.NewRecorder()
						c, _ := gin.CreateTestContext(recorder)
						c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
						account := siwcFixtureAccount()
						account.Credentials["siwc_models"] = []string{model}
						account.Extra["openai_excel_bps_cache_creation_as_input"] = true
						service := newOpenAIRejectedFieldTestService(nil)
						calls := 0
						service.siwcTransport = siwcCatalogRoundTripper(func(req *http.Request) (*http.Response, error) {
							calls++
							require.Equal(t, siwc.ResponsesURL, req.URL.String())
							wire := "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"output_index\":0,\"content_index\":0,\"delta\":\"hello\"}\n\n"
							wire += fmt.Sprintf("event: response.completed\ndata: "+`{"type":"response.completed","response":{"id":"resp_siwc_cache","object":"response","model":%q,"status":"completed","output":[{"id":"msg_siwc","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"hello"}]}],"usage":%s}}`+"\n\n", model, tt.usage)
							return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(wire))}, nil
						})
						var result *OpenAIForwardResult
						var err error
						switch protocol {
						case "responses":
							result, err = service.Forward(context.Background(), c, account, body)
						case "chat":
							result, err = service.ForwardAsChatCompletions(context.Background(), c, account, body, "cache-key", "")
						case "messages":
							result, err = service.ForwardAsAnthropic(context.Background(), c, account, body, "cache-key", "")
						}
						require.NoError(t, err)
						require.Equal(t, 1, calls)
						require.Equal(t, http.StatusOK, recorder.Code)
						require.NotNil(t, result)
						require.Equal(t, 1000, result.Usage.InputTokens)
						require.Equal(t, 50, result.Usage.OutputTokens)
						require.Equal(t, tt.wantRead, result.Usage.CacheReadInputTokens)
						require.Equal(t, tt.write, result.Usage.CacheCreationInputTokens)
						require.Equal(t, stream, result.Stream)

						usage := siwcDownstreamUsage(t, recorder.Body.String(), protocol, stream)
						if protocol == "messages" {
							require.Equal(t, int64(tt.wantInput), usage.Get("input_tokens").Int())
							require.Equal(t, int64(tt.write), usage.Get("cache_creation_input_tokens").Int())
							require.Equal(t, int64(tt.wantRead), usage.Get("cache_read_input_tokens").Int())
						} else {
							parsed, ok := openAIUsageFromGJSON(usage)
							require.True(t, ok)
							require.Equal(t, result.Usage, parsed, "downstream usage must retain the measured token categories")
						}
						for _, subscription := range []bool{false, true} {
							t.Run(fmt.Sprintf("subscription=%t", subscription), func(t *testing.T) {
								assertSIWCCacheBilling(t, account, result, tt.wantInput, tt.wantRead, tt.write, 8e-6, subscription)
							})
						}
					})
				}
			}
		}
	}
}

func TestSIWCCacheCreationExplicitZeroPrice(t *testing.T) {
	for _, model := range []string{"gpt-6-astra", "gpt-6.1-sol"} {
		t.Run(model, func(t *testing.T) {
			result := &OpenAIForwardResult{
				RequestID: "resp_siwc_free_cache", Model: model, UpstreamEndpoint: "/v1/responses",
				Usage: OpenAIUsage{InputTokens: 1000, OutputTokens: 50, CacheReadInputTokens: 100, CacheCreationInputTokens: 200},
			}
			assertSIWCCacheBilling(t, siwcFixtureAccount(), result, 700, 100, 200, 0, false)
		})
	}
}

func assertSIWCCacheBilling(t *testing.T, account *Account, result *OpenAIForwardResult, input, read, write int, writePrice float64, subscription bool) {
	t.Helper()
	const groupID = int64(777)
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	billingRepo := &openAIRecordUsageBillingRepoStub{}
	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
	pricing := ChannelModelPricing{
		Platform: PlatformOpenAI, Models: []string{result.Model}, BillingMode: BillingModeToken,
		InputPrice: float64Ptr(5e-6), OutputPrice: float64Ptr(30e-6),
		CacheReadPrice: float64Ptr(0.5e-6), CacheWritePrice: &writePrice,
	}
	svc.channelService = newChannelServiceWithPricings(groupID, []ChannelModelPricing{pricing})
	svc.resolver = NewModelPricingResolver(svc.channelService, svc.billingService)
	group := &Group{ID: groupID, Platform: PlatformOpenAI, RateMultiplier: 0.35}
	request := &OpenAIRecordUsageInput{
		Result: result, Account: account, User: &User{ID: 2001},
		APIKey: &APIKey{ID: 1001, GroupID: i64p(groupID), Group: group},
	}
	if subscription {
		group.SubscriptionType = SubscriptionTypeSubscription
		request.Subscription = &UserSubscription{ID: 4001}
	}
	original := result.Usage
	require.NoError(t, svc.RecordUsage(context.Background(), request))
	require.Equal(t, original, result.Usage, "pricing must not rewrite upstream measurements")
	require.Equal(t, 1, usageRepo.calls)
	require.NotNil(t, usageRepo.lastLog)
	log := usageRepo.lastLog
	require.Equal(t, input, log.InputTokens)
	require.Equal(t, read, log.CacheReadTokens)
	require.Equal(t, write, log.CacheCreationTokens)
	require.Equal(t, 1050, log.TotalTokens())
	require.InDelta(t, float64(input)*5e-6, log.InputCost, 1e-12)
	require.InDelta(t, float64(read)*0.5e-6, log.CacheReadCost, 1e-12)
	require.InDelta(t, float64(write)*writePrice, log.CacheCreationCost, 1e-12)
	require.InDelta(t, 50*30e-6, log.OutputCost, 1e-12)
	wantTotal := float64(input)*5e-6 + float64(read)*0.5e-6 + float64(write)*writePrice + 50*30e-6
	require.InDelta(t, wantTotal, log.TotalCost, 1e-12)
	require.InDelta(t, wantTotal*0.35, log.ActualCost, 1e-12)
	require.Equal(t, 1, billingRepo.calls)
	require.NotNil(t, billingRepo.lastCmd)
	cmd := billingRepo.lastCmd
	require.Equal(t, input, cmd.InputTokens)
	require.Equal(t, read, cmd.CacheReadTokens)
	require.Equal(t, write, cmd.CacheCreationTokens)
	if subscription {
		require.Zero(t, cmd.BalanceCost)
		require.InDelta(t, wantTotal*0.35, cmd.SubscriptionCost, 1e-12)
	} else {
		require.Zero(t, cmd.SubscriptionCost)
		require.InDelta(t, wantTotal*0.35, cmd.BalanceCost, 1e-12)
	}
}

func siwcDownstreamUsage(t *testing.T, body, protocol string, stream bool) gjson.Result {
	t.Helper()
	if !stream {
		usage := gjson.Get(body, "usage")
		require.True(t, usage.IsObject())
		return usage
	}
	var usage gjson.Result
	for line := range strings.SplitSeq(body, "\n") {
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		event := gjson.Parse(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		path := "usage"
		if protocol == "responses" {
			path = "response.usage"
		}
		if value := event.Get(path); value.IsObject() {
			usage = value
		}
	}
	require.True(t, usage.IsObject(), "the completed stream must expose usage")
	return usage
}
