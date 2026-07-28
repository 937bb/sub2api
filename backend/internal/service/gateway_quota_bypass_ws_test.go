package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestQuotaBypassWebSocketInjectsEveryTurn(t *testing.T) {
	for _, mode := range []string{OpenAIWSIngressModeCtxPool, OpenAIWSIngressModePassthrough} {
		t.Run(mode, func(t *testing.T) {
			gin.SetMode(gin.TestMode)

			cfg := &config.Config{}
			cfg.Security.URLAllowlist.Enabled = false
			cfg.Security.URLAllowlist.AllowInsecureHTTP = true
			cfg.Gateway.OpenAIWS.Enabled = true
			cfg.Gateway.OpenAIWS.OAuthEnabled = true
			cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
			cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
			cfg.Gateway.OpenAIWS.IngressModeDefault = mode
			cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
			cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
			cfg.Gateway.OpenAIWS.QueueLimitPerConn = 8
			cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
			cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
			cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3
			cfg.Gateway.OpenAIQuotaBypassInjectPairs = 3

			responses := []string{
				`{"type":"response.completed","response":{"id":"resp_bypass_1","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`,
				`{"type":"response.completed","response":{"id":"resp_bypass_2","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`,
			}
			var captureConn *openAIWSCaptureConn
			var stagedConn *stagedPassthroughConn
			var dialer openAIWSClientDialer
			if mode == OpenAIWSIngressModePassthrough {
				stagedConn = newStagedPassthroughConn()
				dialer = &stagedPassthroughDialer{conn: stagedConn}
			} else {
				captureConn = &openAIWSCaptureConn{events: [][]byte{
					[]byte(responses[0]),
					[]byte(responses[1]),
				}}
				dialer = &openAIWSCaptureDialer{conn: captureConn}
			}
			svc := &OpenAIGatewayService{
				cfg:                       cfg,
				httpUpstream:              &httpUpstreamRecorder{},
				cache:                     &stubGatewayCache{},
				openaiWSResolver:          NewOpenAIWSProtocolResolver(cfg),
				toolCorrector:             NewCodexToolCorrector(),
				openaiWSPassthroughDialer: dialer,
			}
			if mode == OpenAIWSIngressModeCtxPool {
				pool := newOpenAIWSConnPool(cfg)
				pool.setClientDialerForTest(dialer)
				svc.openaiWSPool = pool
				defer pool.Close()
			}

			bypassGroup := &Group{ID: 42, QuotaBypassEnabled: true}
			account := &Account{
				ID:          902,
				Name:        "openai-ws-quota-bypass",
				Platform:    PlatformOpenAI,
				Type:        AccountTypeOAuth,
				Status:      StatusActive,
				Schedulable: true,
				Concurrency: 1,
				Credentials: map[string]any{"access_token": "access-test"},
				Extra: map[string]any{
					"openai_oauth_responses_websockets_v2_mode": mode,
				},
				AccountGroups: []AccountGroup{
					{AccountID: 902, GroupID: bypassGroup.ID, Group: bypassGroup},
				},
			}
			hooks := &OpenAIWSIngressHooks{
				QuotaBypassEnabled:     IsQuotaBypassEligible(account, nil),
				QuotaBypassInjectPairs: ResolveOpenAIQuotaBypassInjectPairs(cfg),
			}
			require.True(t, hooks.QuotaBypassEnabled)

			serverErrCh := make(chan error, 1)
			wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeover})
				if err != nil {
					serverErrCh <- err
					return
				}
				defer func() { _ = conn.CloseNow() }()

				recorder := httptest.NewRecorder()
				ginCtx, _ := gin.CreateTestContext(recorder)
				ginCtx.Request = r.Clone(r.Context())
				readCtx, cancelRead := context.WithTimeout(r.Context(), 3*time.Second)
				msgType, firstMessage, readErr := conn.Read(readCtx)
				cancelRead()
				if readErr != nil {
					serverErrCh <- readErr
					return
				}
				if msgType != coderws.MessageText && msgType != coderws.MessageBinary {
					serverErrCh <- errors.New("unsupported websocket client message type")
					return
				}
				serverErrCh <- svc.ProxyResponsesWebSocketFromClient(r.Context(), ginCtx, conn, account, "access-test", firstMessage, hooks)
			}))
			defer wsServer.Close()

			dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
			clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http"), nil)
			cancelDial()
			require.NoError(t, err)
			defer func() { _ = clientConn.CloseNow() }()

			assertBypassSuffix := func(payload []byte, turn int) {
				input := gjson.GetBytes(payload, "input").Array()
				require.Len(t, input, 7, "turn %d must contain all configured bypass pairs", turn)
				for i := 0; i < 3; i++ {
					require.Equal(t, "function_call", input[1+i*2].Get("type").String())
					require.Equal(t, "function_call_output", input[2+i*2].Get("type").String())
				}
			}
			writeTurn := func(turn int, payload string) {
				writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
				require.NoError(t, clientConn.Write(writeCtx, coderws.MessageText, []byte(payload)))
				cancelWrite()
				if stagedConn != nil {
					select {
					case upstreamPayload := <-stagedConn.writes:
						assertBypassSuffix(upstreamPayload, turn)
					case <-time.After(3 * time.Second):
						t.Fatalf("turn %d was not forwarded upstream", turn)
					}
					stagedConn.Send(responses[turn-1])
				}
				readCtx, cancelRead := context.WithTimeout(context.Background(), 3*time.Second)
				_, event, readErr := clientConn.Read(readCtx)
				cancelRead()
				require.NoError(t, readErr)
				require.Equal(t, "response.completed", gjson.GetBytes(event, "type").String())
			}

			writeTurn(1, `{"type":"response.create","model":"gpt-5.1","input":[{"type":"message","role":"user","content":"first"}]}`)
			writeTurn(2, `{"type":"response.create","model":"gpt-5.1","previous_response_id":"resp_bypass_1","input":[{"type":"message","role":"user","content":"second"}]}`)
			_ = clientConn.Close(coderws.StatusNormalClosure, "done")

			select {
			case serverErr := <-serverErrCh:
				if serverErr != nil && !errors.Is(serverErr, errOpenAIWSConnClosed) {
					require.Contains(t, serverErr.Error(), "StatusNormalClosure")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("waiting for websocket proxy shutdown timed out")
			}

			if captureConn != nil {
				captureConn.mu.Lock()
				writes := append([]map[string]any(nil), captureConn.writes...)
				captureConn.mu.Unlock()
				require.Len(t, writes, 2)
				for turn, write := range writes {
					encoded, marshalErr := json.Marshal(write)
					require.NoError(t, marshalErr)
					assertBypassSuffix(encoded, turn+1)
				}
			}
		})
	}
}
