package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSIWCLegacyIdentificationAndCreateGuard(t *testing.T) {
	ordinary := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"client_id": "app_codex"}}
	require.False(t, ordinary.IsOpenAISiwc())
	require.NoError(t, validateSIWCCreation(ordinary))
	for _, account := range []*Account{
		{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{"auth_protocol": "siwc"}},
		{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"client_id": "oaiapp_legacy"}},
	} {
		require.True(t, account.IsOpenAISiwc())
		require.Error(t, validateSIWCCreation(account))
	}
	require.Error(t, preserveSIWCAdminCredentials(ordinary, &UpdateAccountInput{Extra: map[string]any{"auth_protocol": "siwc"}}))
	require.Error(t, preserveSIWCAdminCredentials(ordinary, &UpdateAccountInput{Credentials: map[string]any{"client_id": "oaiapp_legacy"}}))
}
