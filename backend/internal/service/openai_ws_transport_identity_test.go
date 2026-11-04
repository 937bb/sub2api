package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/stretchr/testify/require"
)

func TestWSHTTPTransportPartitionsAccountsAndDeviceEdits(t *testing.T) {
	d, ok := newDefaultOpenAIWSClientDialer().(*coderOpenAIWSClientDialer)
	require.True(t, ok)
	defer func() {
		for _, entry := range d.proxyClients {
			closeOpenAIWSProxyClient(entry.client)
		}
	}()
	headers := http.Header{"X-Codex-Installation-Id": {"device-a"}, "User-Agent": {CodexCanonicalUserAgent()}}
	obtain := func(h http.Header) *http.Client {
		client, err := d.transportHTTPClient("", true, openai.TransportIdentityKey(context.Background(), h))
		require.NoError(t, err)
		return client
	}
	first := obtain(headers)
	headers.Set("Session-Id", "next-turn")
	headers.Set("Authorization", "refreshed-token")
	require.Same(t, first, obtain(headers))
	headers.Set("X-Codex-Installation-Id", "device-b")
	require.NotSame(t, first, obtain(headers))
	headers.Set("X-Codex-Installation-Id", "device-a")
	require.Same(t, first, obtain(headers))
	headers.Set("User-Agent", "other-client")
	require.NotSame(t, first, obtain(headers))
}
