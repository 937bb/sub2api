package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestForwardOpenAIWSV2_BareErrorPreservesMessageAndClosesResponsesStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const overloadMessage = "Our servers are currently overloaded. Please try again later."
	tests := []struct {
		name           string
		afterOutput    bool
		failTerminal   bool
		message        string
		code           string
		wantMessage    string
		wantClientCode string
	}{
		{name: "overload_before_output", message: overloadMessage, code: "server_is_overloaded", wantMessage: overloadMessage, wantClientCode: "server_error"},
		{name: "overload_after_output", afterOutput: true, message: overloadMessage, code: "server_is_overloaded", wantMessage: overloadMessage, wantClientCode: "server_error"},
		{name: "other_provider_error", message: "This request is not permitted for this model.", code: "provider_error", wantMessage: "This request is not permitted for this model.", wantClientCode: "provider_error"},
		{name: "missing_message_uses_fallback", code: "provider_error", wantMessage: "Upstream websocket error", wantClientCode: "provider_error"},
		{name: "terminal_write_failure_is_not_committed", failTerminal: true, message: overloadMessage, code: "server_is_overloaded", wantMessage: overloadMessage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requestCount atomic.Int32
			upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := upgrader.Upgrade(w, r, http.Header{"X-Request-Id": []string{"upstream-message-test"}})
				if err != nil {
					t.Errorf("upgrade websocket: %v", err)
					return
				}
				defer conn.Close()
				var request map[string]any
				if err := conn.ReadJSON(&request); err != nil {
					t.Errorf("read request: %v", err)
					return
				}
				requestCount.Add(1)
				if tt.afterOutput {
					if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.output_text.delta","delta":"partial answer"}`)); err != nil {
						t.Errorf("write output: %v", err)
						return
					}
				}
				if err := conn.WriteJSON(map[string]any{
					"type": "error", "sequence_number": 2,
					"error": map[string]any{"code": tt.code, "type": "service_unavailable_error", "message": tt.message, "param": nil},
				}); err != nil {
					t.Errorf("write error: %v", err)
				}
			}))
			defer server.Close()

			recorder := newOpenAIResponseFlushRecorder()
			if tt.failTerminal {
				recorder.failAfterWrites = 1 // bare error succeeds; synthesized terminal fails
			}
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			cfg := newOpenAIWSV2TestConfig()
			cfg.Security.URLAllowlist.Enabled = false
			cfg.Security.URLAllowlist.AllowInsecureHTTP = true
			cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
			cfg.Gateway.LogUpstreamErrorBody = true
			upstream := &httpUpstreamRecorder{}
			svc := &OpenAIGatewayService{
				cfg: cfg, httpUpstream: upstream, cache: &stubGatewayCache{},
				openaiWSResolver: NewOpenAIWSProtocolResolver(cfg), toolCorrector: NewCodexToolCorrector(),
			}
			defer func() {
				if svc.openaiWSPool != nil {
					svc.openaiWSPool.Close()
				}
			}()
			account := &Account{
				ID: 5884, Name: "upstream-message", Platform: PlatformOpenAI,
				Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1,
				Credentials: map[string]any{"api_key": "sk-test", "base_url": server.URL},
				Extra:       map[string]any{"responses_websockets_v2_enabled": true},
			}
			_, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-6-astra","stream":true,"input":"hello"}`))
			require.ErrorContains(t, err, tt.wantMessage)
			var failoverErr *UpstreamFailoverError
			require.False(t, errors.As(err, &failoverErr), "visible failures must not introduce replay")
			require.Nil(t, upstream.lastReq, "the non-fallback WS error must not call HTTP")
			require.EqualValues(t, 1, requestCount.Load())

			attemptsValue, exists := c.Get(OpsUpstreamErrorsKey)
			require.True(t, exists)
			attempts, ok := attemptsValue.([]*OpsUpstreamErrorEvent)
			require.True(t, ok)
			require.Len(t, attempts, 1)
			require.Equal(t, tt.wantMessage, attempts[0].Message)
			require.Equal(t, "upstream-message-test", attempts[0].UpstreamRequestID)
			require.Equal(t, "unknown", attempts[0].ProxyName)
			require.Nil(t, attempts[0].ProxyID)
			require.Equal(t, tt.code, gjson.Get(attempts[0].UpstreamResponseBody, "error.code").String())

			body, _ := recorder.snapshot()
			require.NotContains(t, body, "Upstream request failed")
			if tt.afterOutput {
				require.Equal(t, 1, strings.Count(body, "partial answer"))
			}
			if tt.failTerminal {
				require.False(t, IsResponseCommitted(c))
				require.NotContains(t, body, `"type":"response.failed"`)
				return
			}
			require.True(t, IsResponseCommitted(c), "the handler must not append its generic terminal")
			terminalType, terminal, ok := extractOpenAISSETerminalEvent(body)
			require.True(t, ok)
			require.Equal(t, "response.failed", terminalType)
			require.True(t, json.Valid(terminal))
			require.Equal(t, 1, strings.Count(body, `"type":"response.failed"`))
			require.Equal(t, tt.wantMessage, gjson.GetBytes(terminal, "response.error.message").String())
			require.Equal(t, tt.wantClientCode, gjson.GetBytes(terminal, "response.error.code").String())
			require.NotEmpty(t, gjson.GetBytes(terminal, "response.id").String())
			require.Equal(t, "response", gjson.GetBytes(terminal, "response.object").String())
			require.Equal(t, "[]", gjson.GetBytes(terminal, "response.output").Raw)
			require.Equal(t, "failed", gjson.GetBytes(terminal, "response.status").String())
			require.WithinDuration(t, time.Now(), time.Unix(gjson.GetBytes(terminal, "response.created_at").Int(), 0), 5*time.Second)
			streamErr, exists := GetOpsStreamError(c)
			require.True(t, exists)
			if tt.message != "" {
				require.Equal(t, tt.message, streamErr.Message)
			}
			require.Equal(t, tt.code, streamErr.Code, "Ops keeps original upstream classification")
		})
	}
}
