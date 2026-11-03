package repository

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpenAIAccountTransportIsolationOverridesSharedProxyPool(t *testing.T) {
	for _, useTLS := range []bool{false, true} {
		t.Run(map[bool]string{false: "http", true: "tls"}[useTLS], func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Gateway.ConnectionPoolIsolation = config.ConnectionPoolIsolationProxy
			svc := NewHTTPUpstream(cfg).(*httpUpstreamService)
			get := func(id int64) *upstreamClientEntry {
				var entry *upstreamClientEntry
				var err error
				if useTLS {
					entry, err = svc.getClientEntryWithTLS("", id, 1, tlsfingerprint.CodexLocalHTTPProfile(), service.HTTPUpstreamProfileOpenAI, false, false)
				} else {
					entry, err = svc.getClientEntry("", id, 1, service.HTTPUpstreamProfileOpenAI, false, false)
				}
				require.NoError(t, err)
				t.Cleanup(entry.client.CloseIdleConnections)
				return entry
			}
			first := get(11)
			require.Same(t, first, get(11))
			require.NotSame(t, first, get(22))
			require.Equal(t, config.ConnectionPoolIsolationProxy, svc.accountProfileIsolation(service.HTTPUpstreamProfileDefault, 11))
			require.Equal(t, config.ConnectionPoolIsolationProxy, svc.accountProfileIsolation(service.HTTPUpstreamProfileOpenAI, 0))
		})
	}
}

func TestAccountDeviceSchedulerProjectionPreservesIdentityInputs(t *testing.T) {
	account := service.Account{ID: 11, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		Credentials: map[string]any{"chatgpt_account_id": "account-a", "chatgpt_user_id": "user-a", "user_agent": "explicit-agent", "access_token": "secret"},
		Extra:       map[string]any{"openai_device_id": "device-a", "codex_fingerprint_seed": "11111111-1111-4111-8111-111111111111"}}
	projected := buildSchedulerMetadataAccount(account)
	require.Equal(t, account.GetChatGPTAccountID(), projected.GetChatGPTAccountID())
	require.Equal(t, account.GetCredential("chatgpt_user_id"), projected.GetCredential("chatgpt_user_id"))
	require.Equal(t, account.GetOpenAIUserAgent(), projected.GetOpenAIUserAgent())
	require.Equal(t, account.GetOpenAIDeviceID(), projected.GetOpenAIDeviceID())
	require.Equal(t, account.Extra["codex_fingerprint_seed"], projected.Extra["codex_fingerprint_seed"])
	require.NotContains(t, projected.Credentials, "access_token")
}
