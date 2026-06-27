//go:build unit

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type openAIOAuthConfigValidationAccountRepoStub struct {
	accountRepoStub
	created                            *Account
	updated                            *Account
	updateExtraCalled                  bool
	updateExtraPayload                 map[string]any
	resetOpenAICodexFingerprintCalled  bool
	resetOpenAICodexFingerprintID      int64
	resetOpenAICodexFingerprintPayload OpenAICodexFingerprint
	resetOpenAICodexFingerprintErr     error
	bulkUpdateCalled                   bool
	bulkUpdatePayload                  AccountBulkUpdate
	getByIDAccount                     *Account
	getByIDAccounts                    []*Account
	getByIDCalls                       int
	getByIDsAccounts                   []*Account
	getByCRSAccountIDResult            *Account
	listByPlatform                     string
	listByPlatformResult               []Account
	listByPlatformForValidation        string
	listByPlatformForValidationResult  []Account
}

type openAIOAuthConfigValidationAuthExtraRepoStub struct {
	openAIOAuthConfigValidationAccountRepoStub
	updateAuthAndMergeExtraCalled     bool
	updateAuthAndMergeExtraType       string
	updateAuthAndMergeExtraCredential map[string]any
	updateAuthAndMergeExtraPayload    map[string]any
	updateAuthAndMergeExtraDeleteKeys []string
}

func (s *openAIOAuthConfigValidationAccountRepoStub) Create(_ context.Context, account *Account) error {
	s.created = account
	return nil
}

func (s *openAIOAuthConfigValidationAccountRepoStub) GetByID(_ context.Context, _ int64) (*Account, error) {
	s.getByIDCalls++
	if len(s.getByIDAccounts) > 0 {
		account := s.getByIDAccounts[0]
		s.getByIDAccounts = s.getByIDAccounts[1:]
		return account, nil
	}
	return s.getByIDAccount, nil
}

func (s *openAIOAuthConfigValidationAccountRepoStub) GetByIDs(_ context.Context, _ []int64) ([]*Account, error) {
	return s.getByIDsAccounts, nil
}

func (s *openAIOAuthConfigValidationAccountRepoStub) GetByCRSAccountID(_ context.Context, _ string) (*Account, error) {
	return s.getByCRSAccountIDResult, nil
}

func (s *openAIOAuthConfigValidationAccountRepoStub) ListByPlatform(_ context.Context, platform string) ([]Account, error) {
	s.listByPlatform = platform
	return s.listByPlatformResult, nil
}

func (s *openAIOAuthConfigValidationAccountRepoStub) ListByPlatformForValidation(_ context.Context, platform string) ([]Account, error) {
	s.listByPlatformForValidation = platform
	return s.listByPlatformForValidationResult, nil
}

func (s *openAIOAuthConfigValidationAccountRepoStub) Update(_ context.Context, account *Account) error {
	s.updated = account
	return nil
}

func (s *openAIOAuthConfigValidationAccountRepoStub) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	s.updateExtraCalled = true
	s.updateExtraPayload = updates
	return nil
}

func (s *openAIOAuthConfigValidationAccountRepoStub) ResetOpenAICodexFingerprint(_ context.Context, id int64, fingerprint OpenAICodexFingerprint) error {
	s.resetOpenAICodexFingerprintCalled = true
	s.resetOpenAICodexFingerprintID = id
	s.resetOpenAICodexFingerprintPayload = fingerprint
	if s.resetOpenAICodexFingerprintErr == nil && s.getByIDAccount != nil {
		if s.getByIDAccount.Extra == nil {
			s.getByIDAccount.Extra = map[string]any{}
		}
		s.getByIDAccount.Extra[OpenAICodexFingerprintExtraKey] = fingerprint
	}
	return s.resetOpenAICodexFingerprintErr
}

func (s *openAIOAuthConfigValidationAccountRepoStub) BulkUpdate(_ context.Context, ids []int64, updates AccountBulkUpdate) (int64, error) {
	s.bulkUpdateCalled = true
	s.bulkUpdatePayload = updates
	return int64(len(ids)), nil
}

func (s *openAIOAuthConfigValidationAuthExtraRepoStub) UpdateAuthAndMergeExtra(_ context.Context, _ int64, accountType string, credentials, extraUpdates map[string]any, extraDeleteKeys []string) error {
	s.updateAuthAndMergeExtraCalled = true
	s.updateAuthAndMergeExtraType = accountType
	s.updateAuthAndMergeExtraCredential = credentials
	s.updateAuthAndMergeExtraPayload = extraUpdates
	s.updateAuthAndMergeExtraDeleteKeys = extraDeleteKeys
	return nil
}

func TestAdminServiceResetOpenAICodexFingerprintRotatesOAuthFingerprint(t *testing.T) {
	existingCredentials := map[string]any{"access_token": "secret-access", "refresh_token": "secret-refresh"}
	account := &Account{
		ID:          260,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Credentials: existingCredentials,
		Extra:       map[string]any{"keep": "value"},
	}
	repo := &openAIOAuthConfigValidationAccountRepoStub{getByIDAccount: account}
	svc := &adminServiceImpl{accountRepo: repo}

	updated, err := svc.ResetOpenAICodexFingerprint(context.Background(), account.ID)

	require.NoError(t, err)
	require.Same(t, account, updated)
	require.True(t, repo.resetOpenAICodexFingerprintCalled)
	require.Equal(t, account.ID, repo.resetOpenAICodexFingerprintID)
	require.False(t, repo.updateExtraCalled)
	require.Equal(t, existingCredentials, account.Credentials)
	require.Equal(t, "value", account.Extra["keep"])
	fingerprint := repo.resetOpenAICodexFingerprintPayload
	require.Equal(t, openAICodexFingerprintSchemaV1, fingerprint.SchemaVersion)
	require.NotEmpty(t, fingerprint.InstallationID)
	require.Equal(t, normalizeOpenAICodexUAProfile(ParseOpenAICodexUAProfile(DefaultOpenAICodexUserAgent)), fingerprint.UAProfile)
	require.Equal(t, fingerprint, account.Extra[OpenAICodexFingerprintExtraKey])
}

func TestAdminServiceResetOpenAICodexFingerprintUsesConfiguredUserAgent(t *testing.T) {
	configuredUA := "codex-tui/0.200.0 (Windows 11.0.0; x86_64) Windows_Terminal/1.22 (codex-tui; 0.200.0)"
	account := &Account{ID: 261, Platform: PlatformOpenAI, Type: AccountTypeSetupToken, Extra: map[string]any{}}
	repo := &openAIOAuthConfigValidationAccountRepoStub{getByIDAccount: account}
	svc := &adminServiceImpl{
		accountRepo: repo,
		settingService: NewSettingService(&settingRepoStub{values: map[string]string{
			SettingKeyOpenAICodexUserAgent: configuredUA,
		}}, nil),
	}

	_, err := svc.ResetOpenAICodexFingerprint(context.Background(), account.ID)

	require.NoError(t, err)
	require.True(t, repo.resetOpenAICodexFingerprintCalled)
	require.Equal(t, normalizeOpenAICodexUAProfile(ParseOpenAICodexUAProfile(configuredUA)), repo.resetOpenAICodexFingerprintPayload.UAProfile)
}

func TestAdminServiceResetOpenAICodexFingerprintRejectsAPIKeyAccount(t *testing.T) {
	repo := &openAIOAuthConfigValidationAccountRepoStub{
		getByIDAccount: &Account{ID: 262, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{}},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	_, err := svc.ResetOpenAICodexFingerprint(context.Background(), 262)

	require.Error(t, err)
	require.Contains(t, err.Error(), "OPENAI_CODEX_FINGERPRINT_RESET_UNSUPPORTED")
	require.False(t, repo.resetOpenAICodexFingerprintCalled)
	require.False(t, repo.updateExtraCalled)
}

func TestAdminServiceResetOpenAICodexFingerprintRejectsConcurrentTypeChange(t *testing.T) {
	repo := &openAIOAuthConfigValidationAccountRepoStub{
		getByIDAccounts: []*Account{
			{ID: 263, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{}},
			{ID: 263, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{}},
		},
		resetOpenAICodexFingerprintErr: ErrAccountNotFound,
	}
	svc := &adminServiceImpl{accountRepo: repo}

	_, err := svc.ResetOpenAICodexFingerprint(context.Background(), 263)

	require.Error(t, err)
	require.Contains(t, err.Error(), "OPENAI_CODEX_FINGERPRINT_RESET_UNSUPPORTED")
	require.True(t, repo.resetOpenAICodexFingerprintCalled)
	require.Equal(t, 2, repo.getByIDCalls)
}

func TestValidateOpenAIOAuthAccountsStartupConfigRejectsLegacyFields(t *testing.T) {
	accounts := []Account{
		{
			ID:       42,
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"openai_oauth_passthrough": true,
			},
		},
	}

	err := ValidateOpenAIOAuthAccountsStartupConfig(accounts, &config.Config{})

	require.Error(t, err)
	require.Contains(t, err.Error(), "account_id=42")
	require.Contains(t, err.Error(), "openai_oauth_passthrough")
	require.Contains(t, err.Error(), "openai_oauth_ws_mode=managed_session|off")
}

func TestValidateOpenAIOAuthAccountsStartupConfigAllowsAPIKeyPassthrough(t *testing.T) {
	accounts := []Account{
		{
			ID:       43,
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Extra: map[string]any{
				"openai_passthrough":                         true,
				"openai_apikey_responses_websockets_v2_mode": OpenAIWSIngressModePassthrough,
			},
		},
	}

	require.NoError(t, ValidateOpenAIOAuthAccountsStartupConfig(accounts, &config.Config{}))
}

func TestValidateOpenAIOAuthAccountsStartupConfigRejectsSetupTokenLegacyFields(t *testing.T) {
	accounts := []Account{
		{
			ID:       143,
			Platform: PlatformOpenAI,
			Type:     AccountTypeSetupToken,
			Extra: map[string]any{
				"responses_websockets_v2_enabled": true,
				"openai_ws_enabled":               true,
			},
		},
	}

	err := ValidateOpenAIOAuthAccountsStartupConfig(accounts, &config.Config{})

	require.Error(t, err)
	require.Contains(t, err.Error(), "account_id=143")
	require.Contains(t, err.Error(), "responses_websockets_v2_enabled")
	require.Contains(t, err.Error(), "openai_ws_enabled")
}

func TestValidateOpenAIOAuthAccountsStartupConfigRejectsInvalidNewMode(t *testing.T) {
	accounts := []Account{
		{
			ID:       44,
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"openai_oauth_ws_mode": OpenAIWSIngressModePassthrough,
			},
		},
	}

	err := ValidateOpenAIOAuthAccountsStartupConfig(accounts, &config.Config{})

	require.Error(t, err)
	require.Contains(t, err.Error(), "account_id=44")
	require.Contains(t, err.Error(), "openai_oauth_ws_mode")
	require.Contains(t, err.Error(), "managed_session|off")
}

func TestValidateOpenAIOAuthAccountsStartupConfigRejectsGenericLegacyWSKeys(t *testing.T) {
	accounts := []Account{
		{
			ID:       144,
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"responses_websockets_v2_enabled": true,
				"openai_ws_enabled":               true,
			},
		},
	}

	err := ValidateOpenAIOAuthAccountsStartupConfig(accounts, &config.Config{})

	require.Error(t, err)
	require.Contains(t, err.Error(), "account_id=144")
	require.Contains(t, err.Error(), "responses_websockets_v2_enabled")
	require.Contains(t, err.Error(), "openai_ws_enabled")
}

func TestValidateOpenAIOAuthAccountsStartupConfigVerifiesPassthroughDefaultIsolation(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.IngressModeDefault = OpenAIWSIngressModePassthrough

	require.NoError(t, ValidateOpenAIOAuthAccountsStartupConfig(nil, cfg))
}

func TestProvideOpenAIOAuthStartupConfigValidationRejectsRepoLegacyFields(t *testing.T) {
	repo := &openAIOAuthConfigValidationAccountRepoStub{
		listByPlatformForValidationResult: []Account{
			{
				ID:       45,
				Platform: PlatformOpenAI,
				Type:     AccountTypeOAuth,
				Extra: map[string]any{
					"openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModePassthrough,
				},
			},
		},
	}

	_, err := ProvideOpenAIOAuthStartupConfigValidation(repo, &config.Config{})

	require.Error(t, err)
	require.Empty(t, repo.listByPlatform, "startup validation must not use active-only ListByPlatform")
	require.Equal(t, PlatformOpenAI, repo.listByPlatformForValidation)
	require.Contains(t, err.Error(), "account_id=45")
	require.Contains(t, err.Error(), "openai_oauth_responses_websockets_v2_mode")
}

func TestProvideOpenAIOAuthStartupConfigValidationRejectsDisabledRepoLegacyFields(t *testing.T) {
	repo := &openAIOAuthConfigValidationAccountRepoStub{
		listByPlatformForValidationResult: []Account{
			{
				ID:       46,
				Platform: PlatformOpenAI,
				Type:     AccountTypeOAuth,
				Status:   StatusDisabled,
				Extra: map[string]any{
					"openai_oauth_passthrough": true,
				},
			},
		},
	}

	_, err := ProvideOpenAIOAuthStartupConfigValidation(repo, &config.Config{})

	require.Error(t, err)
	require.Empty(t, repo.listByPlatform, "disabled OAuth accounts must not be skipped by active-only listing")
	require.Equal(t, PlatformOpenAI, repo.listByPlatformForValidation)
	require.Contains(t, err.Error(), "account_id=46")
	require.Contains(t, err.Error(), "openai_oauth_passthrough")
}

func TestAccountServiceCreateRejectsOAuthLegacyPassthrough(t *testing.T) {
	repo := &openAIOAuthConfigValidationAccountRepoStub{}
	svc := NewAccountService(repo, nil)

	_, err := svc.Create(context.Background(), CreateAccountRequest{
		Name:        "oauth-legacy",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "token"},
		Extra:       map[string]any{"openai_oauth_passthrough": true},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "openai_oauth_passthrough")
	require.Nil(t, repo.created)
}

func TestAccountServiceUpdateRejectsOAuthLegacyPassthrough(t *testing.T) {
	repo := &openAIOAuthConfigValidationAccountRepoStub{
		getByIDAccount: &Account{
			ID:       146,
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{},
		},
	}
	svc := NewAccountService(repo, nil)
	extra := map[string]any{"openai_oauth_passthrough": true}

	_, err := svc.Update(context.Background(), 146, UpdateAccountRequest{Extra: &extra})

	require.Error(t, err)
	require.Contains(t, err.Error(), "openai_oauth_passthrough")
	require.Nil(t, repo.updated)
}

func TestAdminServiceCreateAccountRejectsOAuthLegacyPassthrough(t *testing.T) {
	repo := &openAIOAuthConfigValidationAccountRepoStub{}
	svc := &adminServiceImpl{accountRepo: repo}

	_, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Name:                 "oauth-legacy",
		Platform:             PlatformOpenAI,
		Type:                 AccountTypeOAuth,
		Credentials:          map[string]any{"access_token": "token"},
		Extra:                map[string]any{"openai_oauth_passthrough": true},
		SkipDefaultGroupBind: true,
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "openai_oauth_passthrough")
	require.Nil(t, repo.created)
}

func TestAdminServiceCreateAccountAllowsAPIKeyPassthrough(t *testing.T) {
	repo := &openAIOAuthConfigValidationAccountRepoStub{}
	svc := &adminServiceImpl{accountRepo: repo}

	created, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Name:                 "apikey-passthrough",
		Platform:             PlatformOpenAI,
		Type:                 AccountTypeAPIKey,
		Credentials:          map[string]any{"api_key": "sk-test"},
		Extra:                map[string]any{"openai_passthrough": true},
		SkipDefaultGroupBind: true,
	})

	require.NoError(t, err)
	require.NotNil(t, created)
	require.NotNil(t, repo.created)
	require.True(t, repo.created.Extra["openai_passthrough"].(bool))
}

func TestAdminServiceCreateAccountStripsOpenAICodexFingerprintExtra(t *testing.T) {
	repo := &openAIOAuthConfigValidationAccountRepoStub{}
	svc := &adminServiceImpl{accountRepo: repo}

	created, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Name:                 "apikey-fingerprint",
		Platform:             PlatformOpenAI,
		Type:                 AccountTypeAPIKey,
		Credentials:          map[string]any{"api_key": "sk-test"},
		Extra:                map[string]any{OpenAICodexFingerprintExtraKey: map[string]any{"installation_id": "550e8400-e29b-41d4-a716-446655440000"}, "safe": "value"},
		SkipDefaultGroupBind: true,
	})

	require.NoError(t, err)
	require.NotNil(t, created)
	require.NotNil(t, repo.created)
	require.Equal(t, "value", repo.created.Extra["safe"])
	require.NotContains(t, repo.created.Extra, OpenAICodexFingerprintExtraKey)
}

func TestAdminServiceUpdateAccountRejectsAPIKeyToOAuthWithResidualPassthrough(t *testing.T) {
	repo := &openAIOAuthConfigValidationAccountRepoStub{
		getByIDAccount: &Account{
			ID:       45,
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Extra:    map[string]any{"openai_passthrough": true},
		},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	_, err := svc.UpdateAccount(context.Background(), 45, &UpdateAccountInput{Type: AccountTypeOAuth})

	require.Error(t, err)
	require.Contains(t, err.Error(), "openai_passthrough")
	require.Nil(t, repo.updated)
}

func TestAdminServiceUpdateAccountRejectsAPIKeyToSetupTokenWithResidualPassthrough(t *testing.T) {
	repo := &openAIOAuthConfigValidationAccountRepoStub{
		getByIDAccount: &Account{
			ID:       245,
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Extra:    map[string]any{"openai_passthrough": true},
		},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	_, err := svc.UpdateAccount(context.Background(), 245, &UpdateAccountInput{Type: AccountTypeSetupToken})

	require.Error(t, err)
	require.Contains(t, err.Error(), "openai_passthrough")
	require.Nil(t, repo.updated)
}

func TestAdminServiceUpdateAccountRejectsAPIKeyToOAuthWithResidualAPIKeyWSMode(t *testing.T) {
	repo := &openAIOAuthConfigValidationAccountRepoStub{
		getByIDAccount: &Account{
			ID:       145,
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Extra:    map[string]any{"openai_apikey_responses_websockets_v2_mode": OpenAIWSIngressModePassthrough},
		},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	_, err := svc.UpdateAccount(context.Background(), 145, &UpdateAccountInput{Type: AccountTypeOAuth})

	require.Error(t, err)
	require.Contains(t, err.Error(), "openai_apikey_responses_websockets_v2_mode")
	require.Nil(t, repo.updated)
}

func TestAdminServiceUpdateAccountAllowsAPIKeyToOAuthAfterCleanup(t *testing.T) {
	repo := &openAIOAuthConfigValidationAccountRepoStub{
		getByIDAccount: &Account{
			ID:       46,
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Extra:    map[string]any{"openai_passthrough": true},
		},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	updated, err := svc.UpdateAccount(context.Background(), 46, &UpdateAccountInput{
		Type:  AccountTypeOAuth,
		Extra: map[string]any{"openai_oauth_ws_mode": OpenAIOAuthWSModeOff},
	})

	require.NoError(t, err)
	require.NotNil(t, updated)
	require.NotNil(t, repo.updated)
	require.Equal(t, AccountTypeOAuth, repo.updated.Type)
	require.Equal(t, OpenAIOAuthWSModeOff, repo.updated.Extra["openai_oauth_ws_mode"])
	require.NotContains(t, repo.updated.Extra, "openai_passthrough")
}

func TestAdminServiceUpdateAccountPreservesOAuthFingerprintOverRedactedPlaceholder(t *testing.T) {
	fingerprint := map[string]any{"schema_version": 1, "installation_id": "550e8400-e29b-41d4-a716-446655440000"}
	repo := &openAIOAuthConfigValidationAccountRepoStub{
		getByIDAccount: &Account{
			ID:       246,
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{OpenAICodexFingerprintExtraKey: fingerprint, "keep": "old"},
		},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	_, err := svc.UpdateAccount(context.Background(), 246, &UpdateAccountInput{
		Extra: map[string]any{OpenAICodexFingerprintExtraKey: map[string]any{"present": true}, "keep": "new"},
	})

	require.NoError(t, err)
	require.NotNil(t, repo.updated)
	require.Equal(t, "new", repo.updated.Extra["keep"])
	require.Equal(t, fingerprint, repo.updated.Extra[OpenAICodexFingerprintExtraKey])
}

func TestAdminServiceUpdateAccountStripsFingerprintWhenOAuthBecomesAPIKey(t *testing.T) {
	repo := &openAIOAuthConfigValidationAccountRepoStub{
		getByIDAccount: &Account{
			ID:       247,
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{OpenAICodexFingerprintExtraKey: map[string]any{"schema_version": 1}, "keep": "old"},
		},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	_, err := svc.UpdateAccount(context.Background(), 247, &UpdateAccountInput{Type: AccountTypeAPIKey})

	require.NoError(t, err)
	require.NotNil(t, repo.updated)
	require.Equal(t, "old", repo.updated.Extra["keep"])
	require.NotContains(t, repo.updated.Extra, OpenAICodexFingerprintExtraKey)
}

func TestAdminServiceUpdateAccountExtraRejectsOAuthLegacyExtra(t *testing.T) {
	repo := &openAIOAuthConfigValidationAccountRepoStub{
		getByIDAccount: &Account{
			ID:       147,
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{},
		},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	err := svc.UpdateAccountExtra(context.Background(), 147, map[string]any{"openai_oauth_passthrough": true})

	require.Error(t, err)
	require.Contains(t, err.Error(), "openai_oauth_passthrough")
	require.False(t, repo.updateExtraCalled)
}

func TestAdminServiceUpdateAccountExtraAllowsAPIKeyPassthroughExtra(t *testing.T) {
	repo := &openAIOAuthConfigValidationAccountRepoStub{
		getByIDAccount: &Account{
			ID:       148,
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Extra:    map[string]any{},
		},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	err := svc.UpdateAccountExtra(context.Background(), 148, map[string]any{"openai_passthrough": true})

	require.NoError(t, err)
	require.True(t, repo.updateExtraCalled)
}

func TestAdminServiceUpdateAccountExtraDropsOpenAICodexFingerprintOnlyUpdate(t *testing.T) {
	repo := &openAIOAuthConfigValidationAccountRepoStub{}
	svc := &adminServiceImpl{accountRepo: repo}

	err := svc.UpdateAccountExtra(context.Background(), 248, map[string]any{OpenAICodexFingerprintExtraKey: map[string]any{"present": true}})

	require.NoError(t, err)
	require.False(t, repo.updateExtraCalled)
}

func TestAdminServiceUpdateAccountExtraDropsOpenAICodexFingerprintMixedUpdate(t *testing.T) {
	repo := &openAIOAuthConfigValidationAccountRepoStub{
		getByIDAccount: &Account{ID: 249, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{}},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	err := svc.UpdateAccountExtra(context.Background(), 249, map[string]any{OpenAICodexFingerprintExtraKey: map[string]any{"present": true}, "org_uuid": "org"})

	require.NoError(t, err)
	require.True(t, repo.updateExtraCalled)
	require.Equal(t, map[string]any{"org_uuid": "org"}, repo.updateExtraPayload)
}

func TestAdminServiceApplyOAuthCredentialsAllowsOAuthLegacyExtraDeleteKeys(t *testing.T) {
	repo := &openAIOAuthConfigValidationAccountRepoStub{
		getByIDAccount: &Account{
			ID:       149,
			Platform: PlatformOpenAI,
			Type:     AccountTypeSetupToken,
			Credentials: map[string]any{
				"refresh_token": "redacted-old-refresh-token",
			},
			Extra: map[string]any{
				"keep":                     "value",
				"openai_oauth_passthrough": true,
				"openai_oauth_responses_websockets_v2_mode":  "passthrough",
				"openai_apikey_responses_websockets_v2_mode": OpenAIWSIngressModePassthrough,
			},
		},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	updated, err := svc.ApplyOAuthCredentials(context.Background(), 149, &ApplyOAuthCredentialsInput{
		Type:        AccountTypeSetupToken,
		Credentials: map[string]any{"access_token": "redacted-new-access-token"},
		ExtraDeleteKeys: []string{
			" openai_oauth_passthrough ",
			"openai_oauth_responses_websockets_v2_mode",
			"openai_apikey_responses_websockets_v2_mode",
		},
		Extra: map[string]any{"openai_oauth_ws_mode": OpenAIOAuthWSModeManagedSession},
	})

	require.NoError(t, err)
	require.NotNil(t, updated)
	require.NotNil(t, repo.updated)
	require.Equal(t, AccountTypeSetupToken, repo.updated.Type)
	require.Equal(t, "value", repo.updated.Extra["keep"])
	require.Equal(t, OpenAIOAuthWSModeManagedSession, repo.updated.Extra["openai_oauth_ws_mode"])
	require.NotContains(t, repo.updated.Extra, "openai_oauth_passthrough")
	require.NotContains(t, repo.updated.Extra, "openai_oauth_responses_websockets_v2_mode")
	require.NotContains(t, repo.updated.Extra, "openai_apikey_responses_websockets_v2_mode")
}

func TestAdminServiceApplyOAuthCredentialsDropsOpenAICodexFingerprintExtra(t *testing.T) {
	fingerprint := map[string]any{"schema_version": 1, "installation_id": "550e8400-e29b-41d4-a716-446655440000"}
	repo := &openAIOAuthConfigValidationAuthExtraRepoStub{
		openAIOAuthConfigValidationAccountRepoStub: openAIOAuthConfigValidationAccountRepoStub{
			getByIDAccount: &Account{
				ID:          250,
				Platform:    PlatformOpenAI,
				Type:        AccountTypeOAuth,
				Credentials: map[string]any{"refresh_token": "old"},
				Extra:       map[string]any{OpenAICodexFingerprintExtraKey: fingerprint, "keep": "value"},
			},
		},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	_, err := svc.ApplyOAuthCredentials(context.Background(), 250, &ApplyOAuthCredentialsInput{
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "new"},
		Extra:       map[string]any{OpenAICodexFingerprintExtraKey: map[string]any{"present": true}, "openai_oauth_ws_mode": OpenAIOAuthWSModeManagedSession},
	})

	require.NoError(t, err)
	require.True(t, repo.updateAuthAndMergeExtraCalled)
	require.Equal(t, AccountTypeOAuth, repo.updateAuthAndMergeExtraType)
	require.NotContains(t, repo.updateAuthAndMergeExtraPayload, OpenAICodexFingerprintExtraKey)
	require.Equal(t, OpenAIOAuthWSModeManagedSession, repo.updateAuthAndMergeExtraPayload["openai_oauth_ws_mode"])
	require.Nil(t, repo.updated)
}

func TestAdminServiceApplyOAuthCredentialsRejectsUnknownExtraDeleteKeys(t *testing.T) {
	repo := &openAIOAuthConfigValidationAccountRepoStub{
		getByIDAccount: &Account{ID: 150, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{}},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	updated, err := svc.ApplyOAuthCredentials(context.Background(), 150, &ApplyOAuthCredentialsInput{
		Type:            AccountTypeOAuth,
		Credentials:     map[string]any{"access_token": "redacted-new-access-token"},
		ExtraDeleteKeys: []string{"quota_limit"},
	})

	require.Nil(t, updated)
	require.Error(t, err)
	require.Contains(t, err.Error(), "legacy OpenAI OAuth passthrough/WS cleanup keys")
	require.Nil(t, repo.updated)
}

func TestAdminServiceApplyOAuthCredentialsRejectsAPIKeyExtraDeleteKeys(t *testing.T) {
	repo := &openAIOAuthConfigValidationAccountRepoStub{
		getByIDAccount: &Account{ID: 151, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{"openai_passthrough": true}},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	updated, err := svc.ApplyOAuthCredentials(context.Background(), 151, &ApplyOAuthCredentialsInput{
		Type:            AccountTypeOAuth,
		Credentials:     map[string]any{"access_token": "redacted-new-access-token"},
		ExtraDeleteKeys: []string{"openai_passthrough"},
	})

	require.Nil(t, updated)
	require.Error(t, err)
	require.Contains(t, err.Error(), "extra_delete_keys")
	require.Nil(t, repo.updated)
}

func TestAdminServiceBulkUpdateAccountsRejectsOAuthLegacyExtra(t *testing.T) {
	repo := &openAIOAuthConfigValidationAccountRepoStub{
		getByIDsAccounts: []*Account{
			{ID: 47, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{}},
		},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	result, err := svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
		AccountIDs: []int64{47},
		Extra:      map[string]any{"openai_oauth_responses_websockets_v2_enabled": true},
	})

	require.Nil(t, result)
	require.Error(t, err)
	require.Contains(t, err.Error(), "openai_oauth_responses_websockets_v2_enabled")
	require.False(t, repo.bulkUpdateCalled)
}

func TestAdminServiceBulkUpdateAccountsAllowsAPIKeyPassthroughExtra(t *testing.T) {
	repo := &openAIOAuthConfigValidationAccountRepoStub{
		getByIDsAccounts: []*Account{
			{ID: 48, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{}},
		},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	result, err := svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
		AccountIDs: []int64{48},
		Extra:      map[string]any{"openai_passthrough": true},
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, repo.bulkUpdateCalled)
}

func TestAdminServiceBulkUpdateAccountsDropsOpenAICodexFingerprintExtra(t *testing.T) {
	repo := &openAIOAuthConfigValidationAccountRepoStub{
		getByIDsAccounts: []*Account{
			{ID: 248, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{}},
		},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	result, err := svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
		AccountIDs: []int64{248},
		Extra:      map[string]any{OpenAICodexFingerprintExtraKey: map[string]any{"present": true}, "org_uuid": "org"},
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, repo.bulkUpdateCalled)
	require.Equal(t, map[string]any{"org_uuid": "org"}, repo.bulkUpdatePayload.Extra)
}

func TestAccountServiceCreateStripsOpenAICodexFingerprintExtra(t *testing.T) {
	repo := &openAIOAuthConfigValidationAccountRepoStub{}
	svc := NewAccountService(repo, nil)

	created, err := svc.Create(context.Background(), CreateAccountRequest{
		Name:        "apikey-service-fingerprint",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-test"},
		Extra:       map[string]any{OpenAICodexFingerprintExtraKey: map[string]any{"installation_id": "550e8400-e29b-41d4-a716-446655440000"}, "safe": "value"},
	})

	require.NoError(t, err)
	require.NotNil(t, created)
	require.NotNil(t, repo.created)
	require.Equal(t, "value", repo.created.Extra["safe"])
	require.NotContains(t, repo.created.Extra, OpenAICodexFingerprintExtraKey)
}

func TestAccountServiceUpdatePreservesOAuthFingerprintOverRedactedPlaceholder(t *testing.T) {
	fingerprint := map[string]any{"schema_version": 1, "installation_id": "550e8400-e29b-41d4-a716-446655440000"}
	repo := &openAIOAuthConfigValidationAccountRepoStub{
		getByIDAccount: &Account{
			ID:       251,
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{OpenAICodexFingerprintExtraKey: fingerprint, "keep": "old"},
		},
	}
	svc := NewAccountService(repo, nil)
	extra := map[string]any{OpenAICodexFingerprintExtraKey: map[string]any{"present": true}, "keep": "new"}

	_, err := svc.Update(context.Background(), 251, UpdateAccountRequest{Extra: &extra})

	require.NoError(t, err)
	require.NotNil(t, repo.updated)
	require.Equal(t, "new", repo.updated.Extra["keep"])
	require.Equal(t, fingerprint, repo.updated.Extra[OpenAICodexFingerprintExtraKey])
}

func TestAdminServiceBulkUpdateAccountsRejectsAPIKeyExtraDeleteKeys(t *testing.T) {
	repo := &openAIOAuthConfigValidationAccountRepoStub{
		getByIDsAccounts: []*Account{
			{
				ID:       50,
				Platform: PlatformOpenAI,
				Type:     AccountTypeAPIKey,
				Extra: map[string]any{
					"openai_passthrough":                         true,
					"openai_apikey_responses_websockets_v2_mode": OpenAIWSIngressModePassthrough,
				},
			},
		},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	result, err := svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
		AccountIDs:      []int64{50},
		ExtraDeleteKeys: []string{"openai_passthrough"},
		Extra:           map[string]any{"openai_oauth_ws_mode": OpenAIOAuthWSModeManagedSession},
	})

	require.Nil(t, result)
	require.Error(t, err)
	require.Contains(t, err.Error(), "extra_delete_keys")
	require.False(t, repo.bulkUpdateCalled)
}

func TestAdminServiceBulkUpdateAccountsRejectsUnknownExtraDeleteKeys(t *testing.T) {
	repo := &openAIOAuthConfigValidationAccountRepoStub{
		getByIDsAccounts: []*Account{
			{ID: 51, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{}},
		},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	result, err := svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
		AccountIDs:      []int64{51},
		ExtraDeleteKeys: []string{"quota_limit"},
		Extra:           map[string]any{"openai_oauth_ws_mode": OpenAIOAuthWSModeManagedSession},
	})

	require.Nil(t, result)
	require.Error(t, err)
	require.Contains(t, err.Error(), "legacy OpenAI OAuth passthrough/WS cleanup keys")
	require.False(t, repo.bulkUpdateCalled)
}

func TestAdminServiceBulkUpdateAccountsAllowsOAuthLegacyExtraDeleteKeys(t *testing.T) {
	repo := &openAIOAuthConfigValidationAccountRepoStub{
		getByIDsAccounts: []*Account{
			{
				ID:       49,
				Platform: PlatformOpenAI,
				Type:     AccountTypeSetupToken,
				Extra: map[string]any{
					"openai_oauth_passthrough":                     true,
					"openai_oauth_responses_websockets_v2_mode":    "passthrough",
					"openai_oauth_responses_websockets_v2_enabled": true,
				},
			},
		},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	result, err := svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
		AccountIDs: []int64{49},
		ExtraDeleteKeys: []string{
			" openai_oauth_passthrough ",
			"openai_oauth_responses_websockets_v2_mode",
			"openai_oauth_responses_websockets_v2_enabled",
			"openai_oauth_passthrough",
		},
		Extra: map[string]any{"openai_oauth_ws_mode": OpenAIOAuthWSModeManagedSession},
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, repo.bulkUpdateCalled)
	require.Equal(t, map[string]any{"openai_oauth_ws_mode": OpenAIOAuthWSModeManagedSession}, repo.bulkUpdatePayload.Extra)
	require.Equal(t, []string{
		"openai_oauth_passthrough",
		"openai_oauth_responses_websockets_v2_mode",
		"openai_oauth_responses_websockets_v2_enabled",
	}, repo.bulkUpdatePayload.ExtraDeleteKeys)
}

func TestCRSSyncFromCRSRejectsOpenAIOAuthLegacyExtraOnCreate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/web/auth/login":
			_, _ = w.Write([]byte(`{"success":true,"token":"admin-token"}`))
		case "/admin/sync/export-accounts":
			_, _ = w.Write([]byte(`{
				"success": true,
				"data": {
					"openaiOAuthAccounts": [
						{
							"kind":"openai-oauth",
							"id":"crs-oauth-legacy",
							"name":"legacy oauth",
							"isActive":true,
							"schedulable":true,
							"priority":50,
							"status":"active",
							"credentials":{"access_token":"token"},
							"extra":{"openai_oauth_passthrough":true}
						}
					]
				}
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	repo := &openAIOAuthConfigValidationAccountRepoStub{}
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	svc := NewCRSSyncService(repo, nil, nil, nil, nil, cfg)

	result, err := svc.SyncFromCRS(context.Background(), SyncFromCRSInput{
		BaseURL:  server.URL,
		Username: "admin",
		Password: "password",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, result.Failed)
	require.Len(t, result.Items, 1)
	require.Equal(t, "failed", result.Items[0].Action)
	require.Contains(t, result.Items[0].Error, "openai_oauth_passthrough")
	require.Nil(t, repo.created)
}

func TestCRSSyncFromCRSRejectsOpenAIOAuthResidualAPIKeyExtraOnTypeChange(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/web/auth/login":
			_, _ = w.Write([]byte(`{"success":true,"token":"admin-token"}`))
		case "/admin/sync/export-accounts":
			_, _ = w.Write([]byte(`{
				"success": true,
				"data": {
					"openaiOAuthAccounts": [
						{
							"kind":"openai-oauth",
							"id":"crs-oauth-existing",
							"name":"existing oauth",
							"isActive":true,
							"schedulable":true,
							"priority":50,
							"status":"active",
							"credentials":{"access_token":"token"},
							"extra":{}
						}
					]
				}
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	repo := &openAIOAuthConfigValidationAccountRepoStub{
		getByCRSAccountIDResult: &Account{
			ID:       149,
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Extra:    map[string]any{"openai_passthrough": true},
		},
	}
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	svc := NewCRSSyncService(repo, nil, nil, nil, nil, cfg)

	result, err := svc.SyncFromCRS(context.Background(), SyncFromCRSInput{
		BaseURL:  server.URL,
		Username: "admin",
		Password: "password",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, result.Failed)
	require.Len(t, result.Items, 1)
	require.Equal(t, "failed", result.Items[0].Action)
	require.Contains(t, result.Items[0].Error, "openai_passthrough")
	require.Nil(t, repo.updated)
}

func TestCRSSyncFromCRSPreservesExistingOpenAIOAuthFingerprintOverRedactedPlaceholder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/web/auth/login":
			_, _ = w.Write([]byte(`{"success":true,"token":"admin-token"}`))
		case "/admin/sync/export-accounts":
			_, _ = w.Write([]byte(`{
				"success": true,
				"data": {
					"openaiOAuthAccounts": [
						{
							"kind":"openai-oauth",
							"id":"crs-oauth-fingerprint",
							"name":"fingerprint oauth",
							"isActive":true,
							"schedulable":true,
							"priority":50,
							"status":"active",
							"credentials":{"access_token":"token"},
							"extra":{"openai_codex_fingerprint":{"present":true},"keep":"new"}
						}
					]
				}
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	fingerprint := map[string]any{"schema_version": 1, "installation_id": "550e8400-e29b-41d4-a716-446655440000"}
	repo := &openAIOAuthConfigValidationAccountRepoStub{
		getByCRSAccountIDResult: &Account{
			ID:       251,
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{OpenAICodexFingerprintExtraKey: fingerprint, "keep": "old"},
		},
	}
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	svc := NewCRSSyncService(repo, nil, nil, nil, nil, cfg)

	result, err := svc.SyncFromCRS(context.Background(), SyncFromCRSInput{BaseURL: server.URL, Username: "admin", Password: "password"})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, result.Updated)
	require.NotNil(t, repo.updated)
	require.Equal(t, "new", repo.updated.Extra["keep"])
	require.Equal(t, fingerprint, repo.updated.Extra[OpenAICodexFingerprintExtraKey])
}

func TestCRSSyncFromCRSStripsOpenAICodexFingerprintOnAPIKeyUpdate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/web/auth/login":
			_, _ = w.Write([]byte(`{"success":true,"token":"admin-token"}`))
		case "/admin/sync/export-accounts":
			_, _ = w.Write([]byte(`{
				"success": true,
				"data": {
					"openaiResponsesAccounts": [
						{
							"kind":"openai-responses",
							"id":"crs-openai-apikey",
							"name":"apikey",
							"isActive":true,
							"schedulable":true,
							"priority":50,
							"status":"active",
							"credentials":{"api_key":"sk-test"}
						}
					]
				}
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	repo := &openAIOAuthConfigValidationAccountRepoStub{
		getByCRSAccountIDResult: &Account{
			ID:       252,
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				OpenAICodexFingerprintExtraKey: map[string]any{"schema_version": 1, "installation_id": "550e8400-e29b-41d4-a716-446655440000"},
				"keep":                         "old",
			},
		},
	}
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	svc := NewCRSSyncService(repo, nil, nil, nil, nil, cfg)

	result, err := svc.SyncFromCRS(context.Background(), SyncFromCRSInput{BaseURL: server.URL, Username: "admin", Password: "password"})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, result.Updated)
	require.NotNil(t, repo.updated)
	require.Equal(t, AccountTypeAPIKey, repo.updated.Type)
	require.NotContains(t, repo.updated.Extra, OpenAICodexFingerprintExtraKey)
	require.Equal(t, "old", repo.updated.Extra["keep"])
}
