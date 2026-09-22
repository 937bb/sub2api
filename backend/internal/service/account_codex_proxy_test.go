package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSelectOpenAIOutboundProxyOAuthStaysDirect(t *testing.T) {
	legacy := &Proxy{ID: 1, Status: StatusActive, Protocol: "http", Host: "legacy.example", Port: 8001}
	first := &Proxy{ID: 2, Status: StatusActive, Protocol: "http", Host: "first.example", Port: 8002}
	account := &Account{
		Platform:     PlatformOpenAI,
		Type:         AccountTypeOAuth,
		Proxy:        legacy,
		CodexProxies: []*Proxy{first},
	}

	require.Nil(t, account.SelectOpenAIOutboundProxy())
	require.Empty(t, account.SelectOpenAIOutboundProxyURL())
	require.False(t, account.HasOpenAIOutboundProxy())
}

func TestSelectOpenAIOutboundProxySetupTokenStaysDirect(t *testing.T) {
	legacy := &Proxy{ID: 1, Status: StatusActive, Protocol: "http", Host: "legacy.example", Port: 8001}
	account := &Account{
		Platform:     PlatformOpenAI,
		Type:         AccountTypeSetupToken,
		Proxy:        legacy,
		CodexProxies: []*Proxy{{ID: 2, Status: "inactive"}},
	}

	require.Nil(t, account.SelectOpenAIOutboundProxy())
}

func TestSelectOpenAIOutboundProxyLegacyCodexStaysDirect(t *testing.T) {
	legacy := &Proxy{ID: 1, Status: StatusActive, Protocol: "http", Host: "legacy.example", Port: 8001}
	account := &Account{Type: AccountTypeOAuth, Proxy: legacy}

	require.Nil(t, account.SelectOpenAIOutboundProxy())
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

func TestOpenAIProxyStreamCircuitSkipsOAuthFallbackProxy(t *testing.T) {
	legacyID := int64(1)
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		ProxyID:  &legacyID,
		CodexProxies: []*Proxy{
			{ID: 2, Status: "inactive", Protocol: "http", Host: "pool.example", Port: 8002},
		},
	}

	_, ok := openAIProxyStreamCircuitProxyID(account)
	require.False(t, ok)
}
