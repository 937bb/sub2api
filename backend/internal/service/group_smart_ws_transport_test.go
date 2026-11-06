package service

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/smartws"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestGroupSmartWSMeasuredDialerTwoTurnsAndToolPayload(t *testing.T) {
	var opened atomic.Int32
	requests := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		opened.Add(1)
		defer ws.Close()
		for turn := 1; turn <= 2; turn++ {
			_, data, err := ws.ReadMessage()
			if err != nil {
				return
			}
			requests <- string(data)
			id := fmt.Sprintf("response_%d", turn)
			for _, event := range []string{
				fmt.Sprintf(`{"type":"response.created","response":{"id":%q}}`, id),
				fmt.Sprintf(`{"type":"response.output_text.delta","response_id":%q,"delta":"ok"}`, id),
				fmt.Sprintf(`{"type":"response.completed","response":{"id":%q,"status":"completed","usage":{"input_tokens":10,"output_tokens":1}}}`, id),
			} {
				if ws.WriteMessage(websocket.TextMessage, []byte(event)) != nil {
					return
				}
			}
		}
		// Keep the socket alive until the pool closes it.
		_, _, _ = ws.ReadMessage()
	}))
	defer server.Close()
	pool := smartws.NewPoolWithDialer(func(ctx context.Context, target string, headers http.Header, route string) (smartws.Conn, *http.Response, error) {
		require.Equal(t, "wss://chatgpt.com/backend-api/codex/responses", target)
		require.Equal(t, "http://account:fixture@127.0.0.1:26443", route)
		require.Equal(t, "device-fixture", headers.Get("X-Codex-Installation-Id"))
		return dialGroupSmartWS(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), headers, "")
	})
	defer pool.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, input := range []string{`[{"role":"user","content":"hi"}]`, `[{"type":"function_call_output","call_id":"tool_1","output":"<result>&ok"}]`} {
		req, err := http.NewRequestWithContext(ctx, "POST", "https://chatgpt.com/backend-api/codex/responses", strings.NewReader(`{"model":"fixture","stream":true,"input":`+input+`}`))
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer fixture")
		req.Header.Set("Session_id", "session-fixture")
		req.Header.Set("X-Codex-Installation-Id", "device-fixture")
		resp, handled, err := pool.Try(req, "http://account:fixture@127.0.0.1:26443", "group:7/user:11/key:31")
		require.NoError(t, err)
		require.True(t, handled)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		require.Contains(t, string(body), "event: response.completed\n")
		require.Contains(t, string(body), `"input_tokens":10`)
		require.JSONEq(t, `{"type":"response.create","model":"fixture","input":`+input+`}`, <-requests)
	}
	require.Equal(t, int32(1), opened.Load())
	require.Equal(t, uint64(1), pool.Stats()["reused"])
}

func TestGroupSmartWSNestedAttemptPreservesDispatchFence(t *testing.T) {
	ctx, first := beginGroupSmartWSAttempt(context.Background(), nil)
	first.sent.Store(true)
	_, nested := beginGroupSmartWSAttempt(ctx, nil)
	require.Same(t, first, nested)
	require.True(t, nested.sent.Load())
}

func TestAccountFixedProxyNeverUsesStaleOrDirectRoute(t *testing.T) {
	id := int64(12)
	account := &Account{Platform: PlatformOpenAI, ProxyID: &id}
	for name, proxy := range map[string]*Proxy{
		"missing":    nil,
		"mismatch":   {ID: 13, Host: "127.0.0.1", Port: 26443, Status: StatusActive},
		"disabled":   {ID: 12, Host: "127.0.0.1", Port: 26443, Status: "inactive"},
		"empty_host": {ID: 12, Port: 26443, Status: StatusActive},
		"empty_port": {ID: 12, Host: "127.0.0.1", Status: StatusActive},
	} {
		t.Run(name, func(t *testing.T) {
			account.Proxy = proxy
			routes, err := accountProxyRoutes(account, "http://stale.invalid:8080")
			require.ErrorContains(t, err, "unavailable")
			require.Empty(t, routes)
		})
	}
	account.Proxy = &Proxy{ID: 12, Protocol: "http", Host: "127.0.0.1", Port: 26443, Username: "account-12", Password: "fixture", Status: StatusActive}
	routes, err := accountProxyRoutes(account, "http://stale.invalid:8080")
	require.NoError(t, err)
	require.Equal(t, []string{account.Proxy.URL()}, routes)
	called := false
	account.Proxy = nil
	_, err = refreshWithAccountProxyFallback(context.Background(), account, "", func(context.Context, string) (*OpenAITokenInfo, error) {
		called = true
		return nil, nil
	})
	require.Error(t, err)
	require.False(t, called)
}

func TestGroupSmartWSUsesExistingTLSFingerprint(t *testing.T) {
	hellos := make(chan []uint16, 1)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(400) }))
	server.TLS = &tls.Config{GetConfigForClient: func(h *tls.ClientHelloInfo) (*tls.Config, error) {
		hellos <- append([]uint16(nil), h.CipherSuites...)
		return nil, nil
	}}
	server.StartTLS()
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	headers := make(http.Header)
	headers.Set("User-Agent", codexCLIUserAgent)
	_, _, err := dialGroupSmartWS(ctx, "wss"+strings.TrimPrefix(server.URL, "https"), headers, "")
	require.Error(t, err, "untrusted certificates must remain rejected")
	select {
	case hello := <-hellos:
		require.Equal(t, tlsfingerprint.CodexLocalWSProfile().CipherSuites, hello)
	case <-ctx.Done():
		t.Fatal("SmartWS did not use the measured WS TLS profile")
	}
}
