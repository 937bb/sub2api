package service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

type fingerprintViewRepo struct {
	AccountRepository
	accounts map[int64]*Account
}

func (r *fingerprintViewRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	if a := r.accounts[id]; a != nil {
		return a, nil
	}
	return nil, ErrAccountNotFound
}

func TestAccountFingerprintMatchesOutboundIdentityWithoutSecrets(t *testing.T) {
	for _, tc := range []struct {
		name, kind, source string
		extra, credentials map[string]any
	}{
		{"explicit", AccountTypeOAuth, "explicit_device", map[string]any{"openai_device_id": "fixed-device"}, nil},
		{"seed", AccountTypeOAuth, "persisted_seed", map[string]any{codexFingerprintSeedExtraKey: "11111111-1111-4111-8111-aaaaaaaaaaaa"}, nil},
		{"metadata", AccountTypeOAuth, "account_metadata", nil, map[string]any{"chatgpt_account_id": "upstream-private-account"}},
		{"setup", AccountTypeSetupToken, "setup_token", nil, map[string]any{"access_token": "private-token"}},
		{"missing", AccountTypeOAuth, "unavailable", nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := &Account{ID: 9, Platform: PlatformOpenAI, Type: tc.kind, Extra: tc.extra, Credentials: tc.credentials}
			s := &adminServiceImpl{accountRepo: &fingerprintViewRepo{accounts: map[int64]*Account{9: a}}}
			result, err := s.GetAccountFingerprint(context.Background(), 9)
			require.NoError(t, err)
			require.Equal(t, tc.source, result.Source)
			headers := http.Header{}
			applyCodexAccountIdentityHeaders(headers, a, 33)
			require.Equal(t, headers.Get("x-codex-installation-id"), result.InstallationID)
			metadata := map[string]any{}
			applyCodexAccountIdentityFields(metadata, a, 33)
			if result.InstallationID != "" {
				require.Equal(t, result.InstallationID, metadata["installation_id"])
			}
			raw, err := json.Marshal(result)
			require.NoError(t, err)
			for _, secret := range []string{"private-token", "upstream-private-account", "11111111-1111-4111-8111-aaaaaaaaaaaa", "credentials", "codex_fingerprint_seed"} {
				require.NotContains(t, string(raw), secret)
			}
			require.Equal(t, "default", result.WSAccountMode)
			require.True(t, result.ConvergenceReady)
			require.Equal(t, tc.extra, a.Extra)
		})
	}
}

func TestAccountFingerprintShadowUsesParentAndOwnPolicy(t *testing.T) {
	parentID := int64(1)
	parent := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{"openai_device_id": "parent-device"}}
	shadow := &Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth, ParentAccountID: &parentID, Extra: map[string]any{"openai_device_id": "ignored-child-device", "codex_fingerprint_mode": "full", "openai_oauth_responses_websockets_v2_mode": "off"}}
	repo := &fingerprintViewRepo{accounts: map[int64]*Account{1: parent, 2: shadow}}
	s := &adminServiceImpl{accountRepo: repo}
	result, err := s.GetAccountFingerprint(context.Background(), 2)
	require.NoError(t, err)
	require.Equal(t, "parent-device", result.InstallationID)
	require.Equal(t, int64(1), result.CredentialAccountID)
	require.True(t, result.Inherited)
	require.Equal(t, "full", result.ConvergenceMode)
	require.False(t, result.ConvergenceReady)
	require.Equal(t, "off", result.WSAccountMode)
	delete(repo.accounts, 1)
	_, err = s.GetAccountFingerprint(context.Background(), 2)
	require.Error(t, err)
	repo.accounts[1] = shadow
	_, err = s.GetAccountFingerprint(context.Background(), 2)
	require.Error(t, err)
	repo.accounts[1] = &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	_, err = s.GetAccountFingerprint(context.Background(), 2)
	require.Error(t, err)
}

func TestAccountFingerprintRejectsUnsupportedAndMissingAccounts(t *testing.T) {
	s := &adminServiceImpl{accountRepo: &fingerprintViewRepo{accounts: map[int64]*Account{1: {ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}}}}
	_, err := s.GetAccountFingerprint(context.Background(), 1)
	require.Error(t, err)
	_, err = s.GetAccountFingerprint(context.Background(), 2)
	require.ErrorIs(t, err, ErrAccountNotFound)
}
