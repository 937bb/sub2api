package openai

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTransportIdentityIncludesDeviceButExcludesTurns(t *testing.T) {
	ctx := context.Background()
	h := http.Header{"User-Agent": {"fixture"}, "Chatgpt-Account-Id": {"account"}, "X-Codex-Installation-Id": {"device"}, "Authorization": {"Bearer secret"}}
	key := TransportIdentityKey(ctx, h)
	for _, name := range []string{"Session-Id", "Thread-Id", "X-Client-Request-Id", "X-Codex-Turn-Metadata"} {
		h.Set(name, "changed")
	}
	h.Set("Authorization", "Bearer refreshed")
	require.Equal(t, key, TransportIdentityKey(ctx, h))
	require.Len(t, key, 64)
	require.NotContains(t, key, "secret")
	for _, name := range []string{"User-Agent", "Originator", "Version", "Chatgpt-Account-Id", "X-Codex-Installation-Id", "X-Openai-Fedramp"} {
		clone := h.Clone()
		clone.Set(name, "other")
		require.NotEqual(t, key, TransportIdentityKey(ctx, clone), name)
	}
	require.Equal(t, TransportIdentityKey(ctx, http.Header{"User-Agent": {"fixture"}}), TransportIdentityKey(ctx, http.Header{"user-agent": {"fixture"}}))
	require.NotEqual(t, TransportIdentityKey(WithTransportCredential(ctx, "refresh", "first"), nil), TransportIdentityKey(WithTransportCredential(ctx, "refresh", "second"), nil))
	require.NotEqual(t, TransportIdentityKey(ctx, http.Header{"Authorization": {"Bearer one"}}), TransportIdentityKey(ctx, http.Header{"Authorization": {"Bearer two"}}))
}
