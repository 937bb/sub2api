package service

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCoderOpenAIWSClientDialer_ProxyHTTPClientReuse(t *testing.T) {
	dialer := newDefaultOpenAIWSClientDialer()
	impl, ok := dialer.(*coderOpenAIWSClientDialer)
	require.True(t, ok)

	c1, err := impl.proxyHTTPClient("http://127.0.0.1:8080")
	require.NoError(t, err)
	c2, err := impl.proxyHTTPClient("http://127.0.0.1:8080")
	require.NoError(t, err)
	require.Same(t, c1, c2, "同一代理地址应复用同一个 HTTP 客户端")

	c3, err := impl.proxyHTTPClient("http://127.0.0.1:8081")
	require.NoError(t, err)
	require.NotSame(t, c1, c3, "不同代理地址应分离客户端")
}

func TestCoderOpenAIWSClientDialer_ProxyHTTPClientInvalidURL(t *testing.T) {
	dialer := newDefaultOpenAIWSClientDialer()
	impl, ok := dialer.(*coderOpenAIWSClientDialer)
	require.True(t, ok)

	_, err := impl.proxyHTTPClient("://bad")
	require.Error(t, err)
}

func TestCoderOpenAIWSClientDialer_TransportMetricsSnapshot(t *testing.T) {
	dialer := newDefaultOpenAIWSClientDialer()
	impl, ok := dialer.(*coderOpenAIWSClientDialer)
	require.True(t, ok)

	_, err := impl.proxyHTTPClient("http://127.0.0.1:18080")
	require.NoError(t, err)
	_, err = impl.proxyHTTPClient("http://127.0.0.1:18080")
	require.NoError(t, err)
	_, err = impl.proxyHTTPClient("http://127.0.0.1:18081")
	require.NoError(t, err)

	snapshot := impl.SnapshotTransportMetrics()
	require.Equal(t, int64(1), snapshot.ProxyClientCacheHits)
	require.Equal(t, int64(2), snapshot.ProxyClientCacheMisses)
	require.InDelta(t, 1.0/3.0, snapshot.TransportReuseRatio, 0.0001)
}

func TestCoderOpenAIWSClientDialer_ProxyClientCacheCapacity(t *testing.T) {
	dialer := newDefaultOpenAIWSClientDialer()
	impl, ok := dialer.(*coderOpenAIWSClientDialer)
	require.True(t, ok)

	total := openAIWSProxyClientCacheMaxEntries + 32
	for i := 0; i < total; i++ {
		_, err := impl.proxyHTTPClient(fmt.Sprintf("http://127.0.0.1:%d", 20000+i))
		require.NoError(t, err)
	}

	impl.proxyMu.Lock()
	cacheSize := len(impl.proxyClients)
	impl.proxyMu.Unlock()

	require.LessOrEqual(t, cacheSize, openAIWSProxyClientCacheMaxEntries, "代理客户端缓存应受容量上限约束")
}

func TestCoderOpenAIWSClientDialer_ProxyClientCacheIdleTTL(t *testing.T) {
	dialer := newDefaultOpenAIWSClientDialer()
	impl, ok := dialer.(*coderOpenAIWSClientDialer)
	require.True(t, ok)

	oldProxy := "http://127.0.0.1:28080"
	_, err := impl.proxyHTTPClient(oldProxy)
	require.NoError(t, err)

	impl.proxyMu.Lock()
	oldEntry := impl.proxyClients[oldProxy]
	require.NotNil(t, oldEntry)
	oldEntry.lastUsedUnixNano = time.Now().Add(-openAIWSProxyClientCacheIdleTTL - time.Minute).UnixNano()
	impl.proxyMu.Unlock()

	// 触发一次新的代理获取，驱动 TTL 清理。
	_, err = impl.proxyHTTPClient("http://127.0.0.1:28081")
	require.NoError(t, err)

	impl.proxyMu.Lock()
	_, exists := impl.proxyClients[oldProxy]
	impl.proxyMu.Unlock()

	require.False(t, exists, "超过空闲 TTL 的代理客户端应被回收")
}

func TestCoderOpenAIWSClientDialer_ProxyTransportTLSHandshakeTimeout(t *testing.T) {
	dialer := newDefaultOpenAIWSClientDialer()
	impl, ok := dialer.(*coderOpenAIWSClientDialer)
	require.True(t, ok)

	client, err := impl.proxyHTTPClient("http://127.0.0.1:38080")
	require.NoError(t, err)
	require.NotNil(t, client)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	require.NotNil(t, transport)
	require.Equal(t, 10*time.Second, transport.TLSHandshakeTimeout)
}

func TestCoderOpenAIWSClientConn_DoesNotSupportIdlePingWithoutReader(t *testing.T) {
	require.False(t, (&coderOpenAIWSClientConn{}).SupportsIdlePingWithoutReader())
}

func TestCoderOpenAIWSClientDialer_LocalTLSCacheIsolation(t *testing.T) {
	impl := newDefaultOpenAIWSClientDialer().(*coderOpenAIWSClientDialer)
	generic, err := impl.transportHTTPClient("http://127.0.0.1:8080", false)
	require.NoError(t, err)
	measured, err := impl.transportHTTPClient("http://127.0.0.1:8080", true)
	require.NoError(t, err)
	require.NotSame(t, generic, measured)
	repeated, err := impl.transportHTTPClient("http://127.0.0.1:8080", true)
	require.NoError(t, err)
	require.Same(t, measured, repeated)
	direct, err := impl.transportHTTPClient("", true)
	require.NoError(t, err)
	for _, client := range []*http.Client{measured, direct} {
		transport := client.Transport.(*codexWSHTTPTransport).transport
		require.NotNil(t, transport.DialTLSContext)
		proxy, err := transport.Proxy(httptest.NewRequest(http.MethodGet, "https://example.com", nil))
		require.NoError(t, err)
		require.Nil(t, proxy)
		require.True(t, transport.DisableCompression)
		require.ErrorIs(t, client.CheckRedirect(nil, nil), http.ErrUseLastResponse)
		client.CloseIdleConnections()
	}
	generic.CloseIdleConnections()
}

func TestCoderOpenAIWSClientDialerActualLocalTLSSelection(t *testing.T) {
	hellos := make(chan []uint16, 1)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusBadRequest) }))
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
	_, _, _, err := newDefaultOpenAIWSClientDialer().Dial(ctx, "wss"+strings.TrimPrefix(server.URL, "https"), headers, "")
	require.Error(t, err, "untrusted local certificate must be rejected")
	select {
	case hello := <-hellos:
		require.Equal(t, tlsfingerprint.CodexLocalWSProfile().CipherSuites, hello)
	case <-ctx.Done():
		t.Fatal("measured WS TLS profile was not sent")
	}
}
