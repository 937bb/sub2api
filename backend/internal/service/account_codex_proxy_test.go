package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSelectOpenAIOutboundProxyOAuthUsesActivePool(t *testing.T) {
	expiredAt := time.Now().Add(-time.Minute)
	legacy := &Proxy{ID: 1, Status: StatusActive, Protocol: "http", Host: "legacy.example", Port: 8001}
	first := &Proxy{ID: 2, Status: StatusActive, Protocol: "http", Host: "first.example", Port: 8002}
	second := &Proxy{ID: 3, Status: StatusActive, Protocol: "http", Host: "second.example", Port: 8003}
	inactive := &Proxy{ID: 4, Status: "inactive", Protocol: "http", Host: "inactive.example", Port: 8004}
	expired := &Proxy{ID: 5, Status: StatusActive, Protocol: "http", Host: "expired.example", Port: 8005, ExpiresAt: &expiredAt}
	account := &Account{
		Platform:     PlatformOpenAI,
		Type:         AccountTypeOAuth,
		Proxy:        legacy,
		CodexProxies: []*Proxy{first, inactive, second, expired, nil},
	}

	seen := map[int64]bool{}
	for range 256 {
		selected := account.SelectOpenAIOutboundProxy()
		require.NotNil(t, selected)
		require.Contains(t, []int64{first.ID, second.ID}, selected.ID)
		seen[selected.ID] = true
	}
	require.True(t, seen[first.ID])
	require.True(t, seen[second.ID])
}

func TestSelectOpenAIOutboundProxyOAuthFallsBackToLegacyProxy(t *testing.T) {
	legacy := &Proxy{ID: 1, Status: StatusActive, Protocol: "http", Host: "legacy.example", Port: 8001}
	account := &Account{
		Platform:     PlatformOpenAI,
		Type:         AccountTypeSetupToken,
		Proxy:        legacy,
		CodexProxies: []*Proxy{{ID: 2, Status: "inactive"}},
	}

	require.Same(t, legacy, account.SelectOpenAIOutboundProxy())
}

func TestSelectOpenAIOutboundProxyAPIKeyRetainsLegacyBehavior(t *testing.T) {
	legacy := &Proxy{ID: 1, Status: StatusActive, Protocol: "http", Host: "legacy.example", Port: 8001}
	pooled := &Proxy{ID: 2, Status: StatusActive, Protocol: "http", Host: "pooled.example", Port: 8002}
	account := &Account{
		Platform:     PlatformOpenAI,
		Type:         AccountTypeAPIKey,
		Proxy:        legacy,
		CodexProxies: []*Proxy{pooled},
	}

	for range 32 {
		require.Same(t, legacy, account.SelectOpenAIOutboundProxy())
	}
}

func TestNormalizeCodexProxyIDs(t *testing.T) {
	ids, err := normalizeCodexProxyIDs(PlatformOpenAI, AccountTypeOAuth, []int64{5, 3, 5, 1})
	require.NoError(t, err)
	require.Equal(t, []int64{5, 3, 1}, ids)

	_, err = normalizeCodexProxyIDs(PlatformOpenAI, AccountTypeAPIKey, []int64{1})
	require.Error(t, err)

	_, err = normalizeCodexProxyIDs(PlatformOpenAI, AccountTypeOAuth, []int64{1, 2, 3, 4, 5, 6})
	require.Error(t, err)
}

func TestOpenAIProxyStreamCircuitSkipsActiveCodexPool(t *testing.T) {
	legacyID := int64(1)
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		ProxyID:  &legacyID,
		CodexProxies: []*Proxy{
			{ID: 2, Status: StatusActive, Protocol: "http", Host: "pool.example", Port: 8002},
		},
	}

	_, ok := openAIProxyStreamCircuitProxyID(account)
	require.False(t, ok)
}

func TestOpenAIProxyStreamCircuitUsesFallbackWhenCodexPoolUnavailable(t *testing.T) {
	legacyID := int64(1)
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		ProxyID:  &legacyID,
		CodexProxies: []*Proxy{
			{ID: 2, Status: "inactive", Protocol: "http", Host: "pool.example", Port: 8002},
		},
	}

	proxyID, ok := openAIProxyStreamCircuitProxyID(account)
	require.True(t, ok)
	require.Equal(t, legacyID, proxyID)
}
