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
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type kiroReasoningUpstream struct{ t *testing.T }

func (u *kiroReasoningUpstream) Do(req *http.Request, proxy string, id int64, _ int) (*http.Response, error) {
	require.Equal(u.t, "http://test:secret@192.0.2.17:8000", proxy)
	require.EqualValues(u.t, 19, id)
	response := []byte(`{"models":[{"modelId":"claude-opus-4.6","additionalModelRequestFieldsSchema":{"properties":{"thinking":{"properties":{"type":{"enum":["adaptive","disabled"]}}},"output_config":{"properties":{"effort":{"enum":["low","medium","high","max"]}}},"max_tokens":{}}}}]}`)
	if req.URL.Host == "runtime.us-east-1.kiro.dev" {
		body, err := io.ReadAll(req.Body)
		require.NoError(u.t, err)
		require.Contains(u.t, string(body), `"thinking"`)
		require.Contains(u.t, string(body), `"type":"adaptive"`)
		require.NotContains(u.t, string(body), `"budget_tokens"`)
		response = bytes.Join([][]byte{
			kiroTestFrame("reasoningContentEvent", `{"text":"CHECK_REASONING","signature":"opaque-signature"}`),
			kiroTestFrame("assistantResponseEvent", `{"content":"CHECK_ANSWER"}`),
		}, nil)
	}
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(response))}, nil
}

func (u *kiroReasoningUpstream) DoWithTLS(req *http.Request, proxy string, id int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxy, id, concurrency)
}

func TestKiroReasoningAcrossProtocolsWithPrefixBilling(t *testing.T) {
	for _, protocol := range []string{"messages", "messages_manual", "chat", "responses", "messages_alias", "chat_alias", "responses_alias"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%v", protocol, stream), func(t *testing.T) {
				cache := &kiroPrefixTestCache{}
				groupID := int64(7)
				policy := KiroCachePolicy{Enabled: true, Mode: "prefix", AutoPrefix: true, TargetPercent: 90, TTLSeconds: 300, MaxEntries: 128}
				svc := &GatewayService{httpUpstream: &kiroReasoningUpstream{t}, cache: cache, cfg: &config.Config{}, groupRepo: &kiroGroupRepo{group: &Group{ID: groupID, KiroResponseCache: policy}}}
				account := fixedKiroAccount()
				account.Credentials["access_token"] = t.Name()
				account.Credentials["expires_at"] = time.Now().Add(time.Hour).Format(time.RFC3339)
				for turn := 0; turn < 2; turn++ {
					raw := fmt.Sprintf(`{"model":"claude-opus-4.6","max_tokens":2048,"stream":%t,"thinking":{"type":"adaptive"},"reasoning_effort":"high","messages":[{"role":"user","content":"check"}]}`, stream)
					if strings.HasPrefix(protocol, "responses") {
						raw = fmt.Sprintf(`{"model":"claude-opus-4.6","stream":%t,"reasoning":{"effort":"high"},"input":"check"}`, stream)
					}
					if protocol == "messages_manual" {
						raw = strings.Replace(raw, `"thinking":{"type":"adaptive"}`, `"thinking":{"type":"enabled","budget_tokens":2048}`, 1)
					}
					if strings.HasSuffix(protocol, "_alias") {
						raw = strings.ReplaceAll(raw, "claude-opus-4.6", "claude-opus-4-6-thinking")
						raw = strings.ReplaceAll(raw, `"thinking":{"type":"adaptive"},`, "")
						raw = strings.ReplaceAll(raw, `"reasoning_effort":"high",`, "")
						raw = strings.ReplaceAll(raw, `"reasoning":{"effort":"high"},`, "")
					}
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest("POST", "/test", strings.NewReader(raw))
					c.Set("api_key", &APIKey{ID: 1, UserID: 2, GroupID: &groupID})
					var result *ForwardResult
					var err error
					switch strings.TrimSuffix(protocol, "_alias") {
					case "messages", "messages_manual":
						parsed, e := ParseGatewayRequest(NewRequestBodyRef([]byte(raw)), PlatformAnthropic)
						require.NoError(t, e)
						result, err = svc.Forward(context.Background(), c, account, parsed)
					case "chat":
						result, err = svc.ForwardAsChatCompletions(context.Background(), c, account, []byte(raw), nil)
					case "responses":
						result, err = svc.ForwardAsResponses(context.Background(), c, account, []byte(raw), nil)
					}
					require.NoError(t, err)
					require.Contains(t, rec.Body.String(), "CHECK_REASONING")
					require.Contains(t, rec.Body.String(), "CHECK_ANSWER")
					require.Equal(t, "claude-opus-4.6", result.UpstreamModel)
					if strings.HasSuffix(protocol, "_alias") {
						require.Equal(t, "claude-opus-4-6-thinking", result.Model)
					}
					require.Equal(t, 9, result.Usage.OutputTokens, "thinking and answer both contribute to estimated output")
					if turn == 0 {
						require.Positive(t, result.Usage.CacheCreationInputTokens)
					} else {
						require.Positive(t, result.Usage.CacheReadInputTokens)
					}
				}
			})
		}
	}
}
