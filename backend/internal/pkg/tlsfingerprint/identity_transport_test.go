package tlsfingerprint

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/stretchr/testify/require"
)

// Observe actual connection reuse through the native and Rustls H2 serializers.
func TestCodexIdentityPartitionsActualConnections(t *testing.T) {
	for _, h2 := range []bool{false, true} {
		t.Run(fmt.Sprint(h2), func(t *testing.T) {
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if h2 != (r.ProtoMajor == 2) {
					t.Errorf("unexpected HTTP protocol: %s", r.Proto)
				}
				_, _ = io.WriteString(w, r.RemoteAddr)
			}))
			server.EnableHTTP2 = h2
			server.StartTLS()
			defer server.Close()
			profile := CodexLocalHTTPProfile()
			if h2 {
				profile = CodexCustomCAHTTPProfile()
			}
			base := &http.Transport{DisableCompression: true}
			base.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
				conn, err := (&net.Dialer{}).DialContext(ctx, network, addr)
				if err != nil {
					return nil, err
				}
				return performTLSHandshakeWithConfig(ctx, conn, profile, addr, testTrustConfig(server.Certificate().Raw))
			}
			client := &http.Client{Transport: NewCodexHTTPTransport(base, profile), Timeout: 5 * time.Second}
			defer client.CloseIdleConnections()
			send := func(device, turn, token string) string {
				r, err := http.NewRequest("POST", server.URL, strings.NewReader("{}"))
				require.NoError(t, err)
				r.Header.Set("X-Codex-Installation-Id", device)
				r.Header.Set("Session-Id", turn)
				r.Header.Set("Authorization", token)
				response, err := client.Do(r)
				require.NoError(t, err)
				body, err := io.ReadAll(response.Body)
				require.NoError(t, err)
				require.NoError(t, response.Body.Close())
				return string(body)
			}
			first := send("device-a", "turn-a", "token-a")
			require.Equal(t, first, send("device-a", "turn-b", "refreshed"))
			second := send("device-b", "turn-a", "token-a")
			require.NotEqual(t, first, second)
			require.Equal(t, first, send("device-a", "turn-c", "refreshed"))
			anonymous := send("", "a", "token-one")
			require.NotEqual(t, anonymous, send("", "a", "token-two"))
		})
	}
}

func TestCodexIdentityEvictionPreservesActiveStream(t *testing.T) {
	release := make(chan struct{})
	finish := sync.OnceFunc(func() { close(release) })
	defer finish()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/stream" {
			_, _ = io.WriteString(w, "begin\n")
			if err := http.NewResponseController(w).Flush(); err != nil {
				t.Error(err)
				return
			}
			select {
			case <-release:
			case <-r.Context().Done():
			}
			return
		}
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()
	tx := newCodexIdentityTransport(func() http.RoundTripper { return &http.Transport{} })
	client := &http.Client{Transport: tx}
	defer client.CloseIdleConnections()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/stream", nil)
	response, err := client.Do(request)
	require.NoError(t, err)
	prefix := make([]byte, 6)
	_, err = io.ReadFull(response.Body, prefix)
	require.NoError(t, err)
	for i := 0; i < codexIdentityPoolCapacity+2; i++ {
		r, _ := http.NewRequest("GET", server.URL, nil)
		r.Header.Set("X-Codex-Installation-Id", fmt.Sprint(i))
		res, err := client.Do(r)
		require.NoError(t, err)
		_, err = io.Copy(io.Discard, res.Body)
		require.NoError(t, err)
		_ = res.Body.Close()
	}
	tx.mu.Lock()
	size := len(tx.entries)
	tx.mu.Unlock()
	require.Equal(t, codexIdentityPoolCapacity, size)
	// Releasing the server must still finish the active response after eviction.
	finish()
	_, err = io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
}

func TestCodexIdentityConcurrentAuthScopes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "ok") }))
	defer server.Close()
	tx := newCodexIdentityTransport(func() http.RoundTripper { return &http.Transport{} })
	defer tx.CloseIdleConnections()
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ctx := openai.WithTransportCredential(context.Background(), "refresh", fmt.Sprint(i%3))
			r, _ := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
			res, err := tx.RoundTrip(r)
			if err != nil {
				t.Error(err)
				return
			}
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
		}(i)
	}
	wg.Wait()
	tx.mu.Lock()
	size := len(tx.entries)
	tx.mu.Unlock()
	require.Equal(t, 3, size)
}
