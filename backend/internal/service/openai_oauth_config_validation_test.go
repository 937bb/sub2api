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
	created                           *Account
	updated                           *Account
	updateExtraCalled                 bool
	bulkUpdateCalled                  bool
	getByIDAccount                    *Account
	getByIDsAccounts                  []*Account
	getByCRSAccountIDResult           *Account
	listByPlatform                    string
	listByPlatformResult              []Account
	listByPlatformForValidation       string
	listByPlatformForValidationResult []Account
}

func (s *openAIOAuthConfigValidationAccountRepoStub) Create(_ context.Context, account *Account) error {
	s.created = account
	return nil
}

func (s *openAIOAuthConfigValidationAccountRepoStub) GetByID(_ context.Context, _ int64) (*Account, error) {
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

func (s *openAIOAuthConfigValidationAccountRepoStub) UpdateExtra(_ context.Context, _ int64, _ map[string]any) error {
	s.updateExtraCalled = true
	return nil
}

func (s *openAIOAuthConfigValidationAccountRepoStub) BulkUpdate(_ context.Context, ids []int64, _ AccountBulkUpdate) (int64, error) {
	s.bulkUpdateCalled = true
	return int64(len(ids)), nil
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
