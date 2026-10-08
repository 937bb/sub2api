package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/siwc"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func siwcFixtureAccount() *Account {
	return &Account{ID: 97, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Credentials: map[string]any{
		"auth_mode": "siwc", "access_token": "upstream-token", "granted_scope": siwc.Scopes, "siwc_models": []string{"gpt-6-astra"},
		"expires_at": time.Now().Add(time.Hour).Format(time.RFC3339),
	}, Extra: map[string]any{"openai_passthrough": true, "openai_ws_enabled": true, "openai_excel_bps": true, "openai_prism_browser": true, "openai_oauth_ws_sse_acceleration": true}}
}

func TestSIWCForwardRetainsUsageModelAndStatelessHistory(t *testing.T) {
	for _, stream := range []bool{true, false} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			body := []byte(fmt.Sprintf(`{"model":"astra-alias","stream":%t,"instructions":"keep instructions","input":[{"role":"system","content":"keep role text"},{"role":"developer","content":"keep developer"},{"role":"user","content":"hello"}],"tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}}]}`, stream))
			account := siwcFixtureAccount()
			account.Credentials["model_mapping"] = map[string]any{"astra-alias": "gpt-6-astra"}
			service := newOpenAIRejectedFieldTestService(nil)
			calls := 0
			service.siwcTransport = siwcCatalogRoundTripper(func(req *http.Request) (*http.Response, error) {
				calls++
				require.Equal(t, siwc.ResponsesURL, req.URL.String())
				require.Equal(t, siwc.UserAgent, req.UserAgent())
				require.Empty(t, req.Header.Get("Cookie"))
				require.Empty(t, req.Header.Get("session_id"))
				sent, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				require.Equal(t, "gpt-6-astra", gjson.GetBytes(sent, "model").String())
				require.Equal(t, "keep instructions", gjson.GetBytes(sent, "instructions").String())
				require.Equal(t, "developer", gjson.GetBytes(sent, "input.0.role").String())
				require.Equal(t, "keep role text", gjson.GetBytes(sent, "input.0.content").String())
				require.Equal(t, "keep developer", gjson.GetBytes(sent, "input.1.content").String())
				require.Equal(t, "lookup", gjson.GetBytes(sent, `input.#(type=="additional_tools").tools.0.name`).String())
				require.Equal(t, "developer", gjson.GetBytes(sent, `input.#(type=="additional_tools").role`).String())
				require.True(t, gjson.GetBytes(sent, "stream").Bool())
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(siwcForwardSSE))}, nil
			})
			c := newOpenAIRejectedFieldTestContext(body)
			result, err := service.Forward(context.Background(), c, account, body)
			require.NoError(t, err)
			require.Equal(t, 1, calls)
			require.Equal(t, 100, result.Usage.InputTokens)
			require.Equal(t, 5, result.Usage.OutputTokens)
			require.Equal(t, 60, result.Usage.CacheReadInputTokens)
			require.Equal(t, "gpt-6-astra", result.UpstreamModel)
			require.Equal(t, stream, result.Stream)
		})
	}
}

const siwcForwardSSE = "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n" +
	"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-siwc\",\"object\":\"response\",\"model\":\"gpt-6-astra\",\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"hello\"}]}],\"usage\":{\"input_tokens\":100,\"output_tokens\":5,\"input_tokens_details\":{\"cached_tokens\":60}}}}\n\n"

func TestSIWCForwardRejectsContinuationBeforeGenericTransforms(t *testing.T) {
	for _, body := range []string{
		`{"model":"gpt-6-astra","input":"hi","previous_response_id":"resp-old"}`,
		`{"model":"gpt-6-astra","input":"hi","conversation":{"id":"conv-old"}}`,
		`{"model":"gpt-6-astra","input":[{"type":"compaction_trigger"}]}`,
		`{"model":"gpt-6-astra","model":"other","input":"hi"}`,
	} {
		service := newOpenAIRejectedFieldTestService(nil)
		service.siwcTransport = siwcCatalogRoundTripper(func(*http.Request) (*http.Response, error) {
			t.Fatal("invalid request reached upstream")
			return nil, nil
		})
		c := newOpenAIRejectedFieldTestContext([]byte(body))
		_, err := service.Forward(context.Background(), c, siwcFixtureAccount(), []byte(body))
		require.Error(t, err)
		require.Equal(t, 400, c.Writer.Status())
	}
}

func TestSIWCBillingUsesPublicResponseTier(t *testing.T) {
	resolution := ResolveOpenAIServiceTierBilling(siwcFixtureAccount(), "priority", "default")
	require.True(t, resolution.Downgraded)
	require.Equal(t, "default", resolution.Billing)
	account := siwcFixtureAccount()
	delete(account.Credentials, "auth_mode")
	require.Equal(t, "priority", ResolveOpenAIServiceTierBilling(account, "priority", "default").Billing)
}

type siwcFlushRecorder struct {
	*httptest.ResponseRecorder
	deltaFlushed chan struct{}
	once         sync.Once
}

func (r *siwcFlushRecorder) Flush() {
	r.ResponseRecorder.Flush()
	if strings.Contains(r.Body.String(), "response.output_text.delta") {
		r.once.Do(func() { close(r.deltaFlushed) })
	}
}

func TestSIWCStreamsBeforeCompletion(t *testing.T) {
	recorder := &siwcFlushRecorder{ResponseRecorder: httptest.NewRecorder(), deltaFlushed: make(chan struct{})}
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"model":"gpt-6-astra","input":"hello","stream":true}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(string(body)))
	service := newOpenAIRejectedFieldTestService(nil)
	reader, writer := io.Pipe()
	t.Cleanup(func() { _ = reader.Close(); _ = writer.Close() })
	service.siwcTransport = siwcCatalogRoundTripper(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: reader}, nil
	})
	producer := make(chan error, 1)
	go func() {
		parts := strings.SplitN(siwcForwardSSE, "event: response.completed", 2)
		if _, err := io.WriteString(writer, parts[0]); err != nil {
			producer <- err
			return
		}
		select {
		case <-recorder.deltaFlushed:
		case <-time.After(5 * time.Second):
			err := errors.New("SIWC buffered the delta until completion")
			_ = writer.CloseWithError(err)
			producer <- err
			return
		}
		_, err := io.WriteString(writer, "event: response.completed"+parts[1])
		_ = writer.Close()
		producer <- err
	}()
	result, err := service.Forward(context.Background(), c, siwcFixtureAccount(), body)
	require.NoError(t, err)
	require.NoError(t, <-producer)
	require.Equal(t, 100, result.Usage.InputTokens)
}

func TestSIWCRequestPreservesRolesToolsAndIdentityIsolation(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	for _, header := range []string{"Authorization", "Cookie", "User-Agent", "ChatGPT-Account-Id", "X-Codex-Turn-State", "Originator"} {
		c.Request.Header.Set(header, "customer-private-value")
	}
	body := []byte(`{"model":"gpt-6-astra","instructions":"original","input":[{"role":"system","content":"keep system text"},{"role":"developer","content":"keep developer text"},{"role":"user","content":"hi"},{"type":"reasoning","encrypted_content":"opaque","summary":[]}],"tools":[{"type":"function","name":"lookup","parameters":{}},{"type":"namespace","name":"functions","tools":[{"type":"custom","name":"patch"}]},{"type":"web_search"}],"stream":false,"store":true,"temperature":0.3}`)
	req, err := buildSIWCResponsesRequest(context.Background(), c, siwcFixtureAccount(), body, "upstream-token")
	require.NoError(t, err)
	require.Equal(t, siwc.ResponsesURL, req.URL.String())
	require.Equal(t, siwc.UserAgent, req.UserAgent())
	require.Equal(t, "Bearer upstream-token", req.Header.Get("Authorization"))
	for _, header := range []string{"Cookie", "ChatGPT-Account-Id", "X-Codex-Turn-State", "Originator"} {
		require.Empty(t, req.Header.Get(header))
	}
	encoded, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	require.Equal(t, "gpt-6-astra", gjson.GetBytes(encoded, "model").String())
	require.Equal(t, "original", gjson.GetBytes(encoded, "instructions").String())
	require.Equal(t, "developer", gjson.GetBytes(encoded, "input.0.role").String())
	require.Equal(t, "keep developer text", gjson.GetBytes(encoded, "input.1.content").String())
	require.Equal(t, "opaque", gjson.GetBytes(encoded, "input.3.encrypted_content").String())
	require.Equal(t, "additional_tools", gjson.GetBytes(encoded, "input.4.type").String())
	require.Equal(t, "developer", gjson.GetBytes(encoded, "input.4.role").String())
	require.Equal(t, "namespace", gjson.GetBytes(encoded, "tools.0.type").String())
	require.True(t, gjson.GetBytes(encoded, "stream").Bool())
	require.False(t, gjson.GetBytes(encoded, "store").Bool())
	require.False(t, gjson.GetBytes(encoded, "temperature").Exists())
	again, err := normalizeSIWCResponsesBody(encoded)
	require.NoError(t, err)
	require.JSONEq(t, string(encoded), string(again))
}

func TestSIWCRejectsAmbiguousAndStatefulRequests(t *testing.T) {
	for _, body := range []string{
		`{"model":"gpt-6-astra","model":"other","input":"hi"}`,
		`{"model":"gpt-6-astra","previous_response_id":"resp-old","input":"hi"}`,
		`{"model":"gpt-6-astra","input":[{"type":"item_reference","id":"old"}]}`,
		`{"model":"gpt-6-astra","input":"hi","tools":[{"type":"computer_use"}]}`,
	} {
		_, err := normalizeSIWCResponsesBody([]byte(body))
		require.Error(t, err)
	}
}

func TestSIWCDoesNotSelectCodexTransports(t *testing.T) {
	account := siwcFixtureAccount()
	require.True(t, account.IsOpenAIOAuth())
	require.False(t, account.UsesOpenAICodexProtocol())
	require.False(t, account.IsOpenAIOAuthLike())
	require.False(t, account.IsOpenAIPassthroughEnabled())
	require.False(t, account.IsExcelBPSEnabled())
	require.False(t, accountHasPrismBrowser(account))
	require.False(t, account.IsOpenAIOAuthWSSSEAccelerationEnabled())
	require.False(t, account.IsOpenAIResponsesWebSocketV2Enabled())
	require.False(t, isOpenAICodexTicketAccount(account))
	require.Equal(t, OpenAIWSIngressModeOff, account.ResolveOpenAIResponsesWebSocketV2Mode(OpenAIWSIngressModeCtxPool))
	require.Equal(t, OpenAIUpstreamTransportHTTPSSE, NewOpenAIWSProtocolResolver(&config.Config{}).Resolve(account).Transport)
	require.True(t, account.IsModelSupported("gpt-6-astra"))
	require.False(t, account.IsModelSupported("other-model"))
	service := &OpenAIGatewayService{}
	_, err := service.buildOpenAIResponsesWSURL(account)
	require.Error(t, err)
	account.Credentials["auth_mode"] = "oauth"
	require.True(t, account.UsesOpenAICodexProtocol())
}
