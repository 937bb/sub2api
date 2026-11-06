//go:build unit

package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type affinityIngressDialer struct {
	target string
	mu     sync.Mutex
	routes []string
}

func (d *affinityIngressDialer) Dial(ctx context.Context, _ string, headers http.Header, route string) (openAIWSClientConn, int, http.Header, error) {
	d.mu.Lock()
	d.routes = append(d.routes, route)
	d.mu.Unlock()
	// The fixture records the chosen proxy and sends traffic only to localhost.
	return newDefaultOpenAIWSClientDialer().Dial(ctx, d.target, headers, "")
}

func TestAccountProxyAffinityIngressTwoTurns(t *testing.T) {
	for _, mode := range []string{OpenAIWSIngressModeCtxPool, OpenAIWSIngressModeHTTPBridge, OpenAIWSIngressModePassthrough} {
		t.Run(mode, func(t *testing.T) {
			event := func(turn int) string {
				return fmt.Sprintf(`{"type":"response.completed","response":{"id":"resp_fixture_%d","model":"gpt-5.1","status":"completed","output":[],"usage":{"input_tokens":2,"output_tokens":1}}}`, turn)
			}
			upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := coderws.Accept(w, r, nil)
				if err != nil {
					return
				}
				defer conn.CloseNow()
				for turn := 1; ; turn++ {
					if _, _, err := conn.Read(r.Context()); err != nil {
						return
					}
					if err := conn.Write(r.Context(), coderws.MessageText, []byte(event(turn))); err != nil {
						return
					}
				}
			}))
			defer upstreamServer.Close()
			d := &affinityIngressDialer{target: "ws" + strings.TrimPrefix(upstreamServer.URL, "http")}
			u := &httpUpstreamRecorder{}
			for turn := 1; turn <= 2; turn++ {
				u.responses = append(u.responses, &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: " + event(turn) + "\n\n"))})
			}
			cfg := rawChatCompletionsTestConfig()
			cfg.Gateway.OpenAIWS.Enabled = true
			cfg.Gateway.OpenAIWS.APIKeyEnabled = true
			cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
			cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
			cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 3
			cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 3
			cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
			cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3
			pool := newOpenAIWSConnPool(cfg)
			defer pool.Close()
			pool.setClientDialerForTest(d)
			s := &OpenAIGatewayService{cfg: cfg, httpUpstream: u, cache: &stubGatewayCache{}, openaiWSResolver: NewOpenAIWSProtocolResolver(cfg), toolCorrector: NewCodexToolCorrector(), openaiWSPool: pool, openaiWSPassthroughDialer: d}
			a := affinityTestAccount()
			a.Type = AccountTypeAPIKey
			a.Status = StatusActive
			a.Concurrency = 3
			a.Schedulable = true
			a.Credentials = map[string]any{"api_key": "fixture"}
			a.Extra["openai_apikey_responses_websockets_v2_mode"] = mode
			done := make(chan error, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := coderws.Accept(w, r, nil)
				if err != nil {
					done <- err
					return
				}
				defer conn.CloseNow()
				_, first, err := conn.Read(r.Context())
				if err != nil {
					done <- err
					return
				}
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = r.Clone(r.Context())
				c.Set("api_key", &APIKey{ID: 7})
				done <- s.ProxyResponsesWebSocketFromClient(r.Context(), c, conn, a, "fixture", first, nil)
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			conn, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
			require.NoError(t, err)
			defer conn.CloseNow()
			for turn := 1; turn <= 2; turn++ {
				metadata := ""
				if turn == 1 {
					metadata = `,"client_metadata":{"thread_id":"original"}`
				}
				body := fmt.Sprintf(`{"type":"response.create","model":"gpt-5.1","stream":true,"input":"turn %d"%s}`, turn, metadata)
				require.NoError(t, conn.Write(ctx, coderws.MessageText, []byte(body)))
				for {
					_, raw, err := conn.Read(ctx)
					require.NoError(t, err)
					if gjson.GetBytes(raw, "type").String() == "response.completed" {
						break
					}
					require.NotEqual(t, "error", gjson.GetBytes(raw, "type").String(), "%s", raw)
				}
			}
			require.NoError(t, conn.Close(coderws.StatusNormalClosure, "done"))
			select {
			case err := <-done:
				require.NoError(t, err)
			case <-ctx.Done():
				t.Fatal("ingress did not finish")
			}
			expected, err := accountProxyRoutesForContext(affinityTestContext("original", 7), a, "")
			require.NoError(t, err)
			if mode == OpenAIWSIngressModeHTTPBridge {
				require.Len(t, u.requests, 2)
				require.Equal(t, expected[0], u.lastProxyURL)
				for _, request := range u.requests {
					require.Equal(t, affinityTestContext("original", 7).Value(accountProxyAffinityKey{}), request.Context().Value(accountProxyAffinityKey{}))
				}
			} else {
				d.mu.Lock()
				routes := append([]string(nil), d.routes...)
				d.mu.Unlock()
				require.Equal(t, expected, routes, "both turns must use one upstream socket")
			}
		})
	}
}
