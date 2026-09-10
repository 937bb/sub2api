package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestForwardOpenAIWSV2_PreservesHTTPResponsesLiteModePerRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeSetupToken, AccountTypeAPIKey} {
		for _, passthrough := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/passthrough=%t", accountType, passthrough), func(t *testing.T) {
				cfg := newOpenAIWSV2TestConfig()
				cfg.Gateway.OpenAIWS.HTTPIngressUpstreamWSEnabled = true
				cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
				cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
				cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
				capture := &openAIWSCaptureConn{}
				dialer := &openAIWSCaptureDialer{conn: capture}
				pool := newOpenAIWSConnPool(cfg)
				pool.setClientDialerForTest(dialer)
				t.Cleanup(pool.Close)
				upstream := &httpUpstreamRecorder{}
				svc := &OpenAIGatewayService{
					cfg: cfg, httpUpstream: upstream, cache: &stubGatewayCache{},
					openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
					toolCorrector:    NewCodexToolCorrector(), openaiWSPool: pool,
				}
				account := &Account{
					ID: 5890, Name: "lite-bridge", Platform: PlatformOpenAI, Type: accountType,
					Status: StatusActive, Schedulable: true, Concurrency: 1,
					Credentials: map[string]any{"api_key": "sk-test", "access_token": "oauth-test", "chatgpt_account_id": "chatgpt-test"},
					Extra:       map[string]any{"responses_websockets_v2_enabled": true, "openai_passthrough": passthrough},
				}
				for turn, header := range []string{"true", "", "false", " TrUe "} {
					lite := isOpenAIResponsesLiteHeader(header)
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
					c.Request.Header.Set("User-Agent", "codex_cli_rs/0.154.0")
					c.Request.Header.Set("session_id", "same-client-session")
					c.Request.Header.Set(responsesLiteHeader, header)
					SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
					upstream.resp = &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
						Body:       io.NopCloser(strings.NewReader("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_passthrough\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n")),
					}
					capture.mu.Lock()
					capture.events = append(capture.events, []byte(fmt.Sprintf(`{"type":"response.completed","response":{"id":"resp_lite_%d","model":"gpt-5.6-sol","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}`, turn)))
					capture.mu.Unlock()
					body := []byte(`{"model":"gpt-5.6-sol","stream":true,"instructions":"Keep the requested tool contract.","prompt_cache_key":"same-client-session","parallel_tool_calls":true,"client_metadata":{"custom":"preserved"},"tools":[{"type":"function","name":"lookup","parameters":{"type":"object","properties":{}}}],"input":[{"role":"user","content":"hello"}]}`)
					result, err := svc.Forward(context.Background(), c, account, body)
					require.NoError(t, err)
					require.NotNil(t, result)
					if passthrough {
						require.False(t, result.OpenAIWSMode)
						require.NotNil(t, upstream.lastReq)
						require.Equal(t, lite, isOpenAIResponsesLiteHeader(upstream.lastReq.Header.Get(responsesLiteHeader)))
						continue
					}
					require.True(t, result.OpenAIWSMode)
					require.Nil(t, upstream.lastReq, "the HTTP-to-WS bridge must handle this request")
					payload := requestToJSONString(capture.lastWrite)
					require.Equal(t, lite, isOpenAIResponsesLiteWebSocketPayload([]byte(payload)), "turn %d lost or leaked the HTTP Lite mode", turn)
					require.Equal(t, "preserved", gjson.Get(payload, "client_metadata.custom").String())
					require.Equal(t, "lookup", gjson.Get(payload, "tools.0.name").String())
					if lite {
						require.False(t, gjson.Get(payload, "parallel_tool_calls").Bool())
						if account.IsOpenAIOAuthLike() {
							require.Equal(t, "all_turns", gjson.Get(payload, "reasoning.context").String())
						}
					}
				}
				if !passthrough {
					require.Equal(t, 1, dialer.DialCount(), "Lite mode changes must preserve connection reuse")
				}
			})
		}
	}
}

func TestOpenAIWSClientMetadataDoesNotLeakIntoCanonicalBody(t *testing.T) {
	for _, existing := range []any{
		nil,
		map[string]any{"custom": "preserved"},
		map[string]string{"custom": "preserved"},
	} {
		t.Run(fmt.Sprintf("%T", existing), func(t *testing.T) {
			canonical := map[string]any{"model": "gpt-5.6-sol", "client_metadata": existing}
			svc := &OpenAIGatewayService{}
			first := svc.buildOpenAIWSCreatePayload(canonical, nil)
			setOpenAIWSClientMetadata(first, responsesLiteWSMetadataKey, "true")
			second := svc.buildOpenAIWSCreatePayload(canonical, nil)
			require.True(t, isOpenAIResponsesLiteWebSocketPayload([]byte(requestToJSONString(first))))
			require.False(t, isOpenAIResponsesLiteWebSocketPayload([]byte(requestToJSONString(canonical))))
			require.False(t, isOpenAIResponsesLiteWebSocketPayload([]byte(requestToJSONString(second))))
			if existing != nil {
				require.Equal(t, "preserved", gjson.Get(requestToJSONString(first), "client_metadata.custom").String())
			}
		})
	}
}
