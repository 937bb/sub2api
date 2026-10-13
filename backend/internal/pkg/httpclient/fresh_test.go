package httpclient

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
)

func TestFreshConnectionDoesNotReuseOrEvictSharedConnection(t *testing.T) {
	for _, h2 := range []bool{false, true} {
		name := "http1"
		if h2 {
			name = "http2"
		}
		t.Run(name, func(t *testing.T) {
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Peer", r.RemoteAddr)
				w.Header().Set("X-Protocol", r.Proto)
				_, _ = io.WriteString(w, "ok")
			}))
			server.EnableHTTP2 = h2
			server.StartTLS()
			defer server.Close()
			client := server.Client()
			base, ok := client.Transport.(*http.Transport)
			require.True(t, ok)
			if h2 {
				//nolint:staticcheck // Cover the explicit x/net HTTP/2 pool used by production.
				_, err := http2.ConfigureTransports(base)
				require.NoError(t, err)
			}
			defer base.CloseIdleConnections()
			client.Transport = FreshTransport{Base: base}
			call := func(fresh bool) string {
				ctx := context.Background()
				if fresh {
					ctx = WithFreshConnection(ctx)
				}
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
				require.NoError(t, err)
				resp, err := client.Do(req)
				require.NoError(t, err)
				_, err = io.Copy(io.Discard, resp.Body)
				require.NoError(t, err)
				require.NoError(t, resp.Body.Close())
				if h2 {
					require.Equal(t, "HTTP/2.0", resp.Header.Get("X-Protocol"))
				}
				return resp.Header.Get("X-Peer")
			}
			shared := call(false)
			require.Equal(t, shared, call(false))
			require.NotEqual(t, shared, call(true), "retry must establish a new TCP/TLS connection")
			require.Equal(t, shared, call(false), "other requests must retain the shared connection")
		})
	}
}

func TestFreshClientDoesNotReuseInstrumentedPool(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Peer", r.RemoteAddr)
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()
	shared, err := GetClient(Options{})
	require.NoError(t, err)
	fresh, err := NewFreshClient(Options{})
	require.NoError(t, err)
	call := func(client *http.Client) string {
		resp, callErr := client.Get(server.URL)
		require.NoError(t, callErr)
		_, callErr = io.Copy(io.Discard, resp.Body)
		require.NoError(t, callErr)
		require.NoError(t, resp.Body.Close())
		return resp.Header.Get("X-Peer")
	}
	old := call(shared)
	require.Equal(t, old, call(shared))
	require.NotEqual(t, old, call(fresh))
	require.Equal(t, old, call(shared))
}
