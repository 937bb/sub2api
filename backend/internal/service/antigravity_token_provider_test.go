//go:build unit

package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAntigravityTokenProvider_GetAccessToken_Upstream(t *testing.T) {
	provider := &AntigravityTokenProvider{}

	t.Run("upstream account with valid api_key", func(t *testing.T) {
		account := &Account{
			Platform: PlatformAntigravity,
			Type:     AccountTypeUpstream,
			Credentials: map[string]any{
				"api_key": "sk-test-key-12345",
			},
		}
		token, err := provider.GetAccessToken(context.Background(), account)
		require.NoError(t, err)
		require.Equal(t, "sk-test-key-12345", token)
	})

	t.Run("upstream account missing api_key", func(t *testing.T) {
		account := &Account{
			Platform:    PlatformAntigravity,
			Type:        AccountTypeUpstream,
			Credentials: map[string]any{},
		}
		token, err := provider.GetAccessToken(context.Background(), account)
		require.Error(t, err)
		require.Contains(t, err.Error(), "upstream account missing api_key")
		require.Empty(t, token)
	})

	t.Run("upstream account with empty api_key", func(t *testing.T) {
		account := &Account{
			Platform: PlatformAntigravity,
			Type:     AccountTypeUpstream,
			Credentials: map[string]any{
				"api_key": "",
			},
		}
		token, err := provider.GetAccessToken(context.Background(), account)
		require.Error(t, err)
		require.Contains(t, err.Error(), "upstream account missing api_key")
		require.Empty(t, token)
	})

	t.Run("upstream account with nil credentials", func(t *testing.T) {
		account := &Account{
			Platform: PlatformAntigravity,
			Type:     AccountTypeUpstream,
		}
		token, err := provider.GetAccessToken(context.Background(), account)
		require.Error(t, err)
		require.Contains(t, err.Error(), "upstream account missing api_key")
		require.Empty(t, token)
	})
}

func TestAntigravityTokenProvider_GetAccessToken_Guards(t *testing.T) {
	provider := &AntigravityTokenProvider{}

	t.Run("nil account", func(t *testing.T) {
		token, err := provider.GetAccessToken(context.Background(), nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "account is nil")
		require.Empty(t, token)
	})

	t.Run("non-antigravity platform", func(t *testing.T) {
		account := &Account{
			Platform: PlatformAnthropic,
			Type:     AccountTypeOAuth,
		}
		token, err := provider.GetAccessToken(context.Background(), account)
		require.Error(t, err)
		require.Contains(t, err.Error(), "not an antigravity account")
		require.Empty(t, token)
	})

	t.Run("unsupported account type", func(t *testing.T) {
		account := &Account{
			Platform: PlatformAntigravity,
			Type:     AccountTypeAPIKey,
		}
		token, err := provider.GetAccessToken(context.Background(), account)
		require.Error(t, err)
		require.Contains(t, err.Error(), "not an antigravity oauth account")
		require.Empty(t, token)
	})
}

func TestAntigravityTokenProvider_GetAccessToken_ConfiguredFallbackSkipsBackfillAndKeepsAccountCacheKey(t *testing.T) {
	cache := &recordingAntigravityTokenCache{}
	probe := newAntigravityV1InternalProbe(t)
	account := &Account{
		ID:       501,
		Platform: PlatformAntigravity,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":                          "oauth-token",
			"expires_at":                            time.Now().Add(30 * time.Minute).Format(time.RFC3339),
			antigravityProjectFallbackCredentialKey: " configured-project ",
		},
	}
	repo := &antigravityTokenProviderAccountRepoStub{account: account}
	provider := NewAntigravityTokenProvider(repo, cache, NewAntigravityOAuthService(nil))

	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "oauth-token", token)

	require.Equal(t, []string{"ag:account:501"}, cache.getCalls)
	require.Equal(t, []string{"ag:account:501"}, cache.setCalls)
	require.Empty(t, cache.acquireCalls)
	require.Empty(t, cache.releaseCalls)
	require.Empty(t, probe.paths, "configured fallback must not call FillProjectID/loadCodeAssist")
	require.Empty(t, account.GetCredential("project_id"), "configured fallback must not mutate project_id")
	require.Zero(t, repo.updateCredentialsCalls, "configured fallback must not persist backfilled credentials")
	require.Zero(t, repo.updateCalls, "configured fallback must not persist backfilled credentials")
}

func TestAntigravityTokenProvider_GetAccessToken_ExpiringConfiguredFallbackRefreshesTokenWithoutProjectBackfill(t *testing.T) {
	cache := &recordingAntigravityTokenCache{}
	probe := newAntigravityV1InternalProbe(t)
	account := &Account{
		ID:       503,
		Platform: PlatformAntigravity,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":                          "stale-token",
			"refresh_token":                         "old-refresh",
			"expires_at":                            time.Now().Add(time.Minute).Format(time.RFC3339),
			"project_id":                            "  ",
			antigravityProjectFallbackCredentialKey: " configured-project ",
		},
	}
	refreshHTTP := withAntigravityRefreshHTTPStub(t, "unexpected-project")
	repo := &antigravityTokenProviderAccountRepoStub{account: account}
	executor := NewAntigravityTokenRefresher(NewAntigravityOAuthService(nil))
	provider := NewAntigravityTokenProvider(repo, cache, NewAntigravityOAuthService(nil))
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), executor)

	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "refreshed-token", token)

	require.Equal(t, []string{"ag:account:503"}, cache.getCalls)
	require.Equal(t, []string{"ag:account:503"}, cache.acquireCalls)
	require.Equal(t, []string{"ag:account:503"}, cache.releaseCalls)
	require.Equal(t, []string{"ag:account:503"}, cache.setCalls)
	require.Equal(t, []string{"/token"}, refreshHTTP.requestPaths)
	require.Zero(t, refreshHTTP.loadCodeAssistCalls)
	require.Zero(t, refreshHTTP.onboardUserCalls)
	require.Empty(t, probe.paths, "fallback-only refresh must not call LoadCodeAssist/OnboardUser or provider backfill")
	require.Empty(t, account.GetCredential("project_id"), "configured fallback must not mutate project_id during refresh")
	require.Equal(t, "configured-project", strings.TrimSpace(account.GetCredential(antigravityProjectFallbackCredentialKey)))
	require.Equal(t, 1, repo.updateCredentialsCalls)
	require.Equal(t, "refreshed-token", repo.updateCredentialsPayload[0]["access_token"])
	require.Equal(t, "refreshed-refresh", repo.updateCredentialsPayload[0]["refresh_token"])
	require.Equal(t, " configured-project ", repo.updateCredentialsPayload[0][antigravityProjectFallbackCredentialKey])
	require.NotContains(t, repo.updateCredentialsPayload[0], "project_id")
	require.Contains(t, repo.updateCredentialsPayload[0], "_token_version")
}

func TestAntigravityTokenProvider_GetAccessToken_ExpiringPrimaryProjectKeepsProjectCacheKey(t *testing.T) {
	cache := &recordingAntigravityTokenCache{}
	account := &Account{
		ID:       504,
		Platform: PlatformAntigravity,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":                          "stale-token",
			"refresh_token":                         "old-refresh",
			"expires_at":                            time.Now().Add(time.Minute).Format(time.RFC3339),
			"project_id":                            "primary-project",
			antigravityProjectFallbackCredentialKey: " configured-project ",
		},
	}
	refreshHTTP := withAntigravityRefreshHTTPStub(t, "primary-project")
	repo := &antigravityTokenProviderAccountRepoStub{account: account}
	executor := NewAntigravityTokenRefresher(NewAntigravityOAuthService(nil))
	provider := NewAntigravityTokenProvider(repo, cache, NewAntigravityOAuthService(nil))
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), executor)

	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "refreshed-token", token)

	require.Equal(t, []string{"ag:primary-project"}, cache.getCalls)
	require.Equal(t, []string{"ag:primary-project"}, cache.acquireCalls)
	require.Equal(t, []string{"ag:primary-project"}, cache.releaseCalls)
	require.Equal(t, []string{"ag:primary-project"}, cache.setCalls)
	require.Contains(t, refreshHTTP.requestPaths, "/v1internal:loadCodeAssist")
	require.Equal(t, 1, refreshHTTP.loadCodeAssistCalls)
	require.Equal(t, 1, repo.updateCredentialsCalls)
	require.Equal(t, "primary-project", repo.updateCredentialsPayload[0]["project_id"])
	require.Equal(t, " configured-project ", repo.updateCredentialsPayload[0][antigravityProjectFallbackCredentialKey])
}

func TestAntigravityTokenProvider_GetAccessToken_MissingFallbackBackfillsProjectID(t *testing.T) {
	cache := &recordingAntigravityTokenCache{}
	probe := newAntigravityV1InternalProbe(t)
	account := &Account{
		ID:       502,
		Platform: PlatformAntigravity,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "oauth-token",
			"expires_at":   time.Now().Add(30 * time.Minute).Format(time.RFC3339),
		},
	}
	repo := &antigravityTokenProviderAccountRepoStub{account: account}
	provider := NewAntigravityTokenProvider(repo, cache, NewAntigravityOAuthService(nil))

	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "oauth-token", token)

	require.Contains(t, probe.paths, "/v1internal:loadCodeAssist")
	require.Equal(t, "backfilled-project", account.GetCredential("project_id"))
	require.Equal(t, 1, repo.updateCredentialsCalls)
	require.Equal(t, "backfilled-project", repo.updateCredentialsPayload[0]["project_id"])
	require.Equal(t, []string{"ag:account:502"}, cache.getCalls)
	require.Equal(t, []string{"ag:backfilled-project"}, cache.setCalls)
}

func TestAntigravityTokenProvider_GetAccessToken_RefreshThenBackfillKeepsFreshTokenFields(t *testing.T) {
	cache := &recordingAntigravityTokenCache{}
	requestAccount := &Account{
		ID:       505,
		Platform: PlatformAntigravity,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":  "stale-token",
			"refresh_token": "old-refresh",
			"expires_at":    time.Now().Add(time.Minute).Format(time.RFC3339),
		},
	}
	repoAccount := &Account{
		ID:       requestAccount.ID,
		Platform: requestAccount.Platform,
		Type:     requestAccount.Type,
		Credentials: map[string]any{
			"access_token":  "stale-token",
			"refresh_token": "old-refresh",
			"expires_at":    requestAccount.GetCredential("expires_at"),
		},
	}
	refreshHTTP := withAntigravityRefreshHTTPStub(t, "")
	repo := &antigravityTokenProviderAccountRepoStub{account: repoAccount}
	executor := NewAntigravityTokenRefresher(NewAntigravityOAuthService(nil))
	provider := NewAntigravityTokenProvider(repo, cache, NewAntigravityOAuthService(nil))
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), executor)

	token, err := provider.GetAccessToken(context.Background(), requestAccount)
	require.NoError(t, err)
	require.Equal(t, "refreshed-token", token)

	require.Equal(t, []string{"/token", "/v1internal:loadCodeAssist"}, refreshHTTP.requestPaths)
	require.Equal(t, 1, refreshHTTP.loadCodeAssistCalls)
	require.Equal(t, 1, repo.updateCredentialsCalls)
	require.Equal(t, "refreshed-token", repo.updateCredentialsPayload[0]["access_token"])
	require.Equal(t, "refreshed-refresh", repo.updateCredentialsPayload[0]["refresh_token"])
	require.Equal(t, "backfilled-project", repo.updateCredentialsPayload[0]["project_id"])
	require.Contains(t, repo.updateCredentialsPayload[0], "_token_version")
	require.Equal(t, "refreshed-token", requestAccount.GetCredential("access_token"))
	require.Equal(t, "refreshed-refresh", requestAccount.GetCredential("refresh_token"))
	require.Equal(t, "backfilled-project", requestAccount.GetCredential("project_id"))
	require.Equal(t, []string{"ag:account:505"}, cache.getCalls)
	require.Equal(t, []string{"ag:account:505"}, cache.acquireCalls)
	require.Equal(t, []string{"ag:account:505"}, cache.releaseCalls)
	require.Equal(t, []string{"ag:backfilled-project"}, cache.setCalls)
}
