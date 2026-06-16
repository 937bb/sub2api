package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/imroc/req/v3"
	"github.com/stretchr/testify/require"
)

type openaiOAuthClientRefreshStub struct {
	refreshCalls     int32
	lastRefreshToken string
	lastProxyURL     string
	lastOpts         OpenAIOAuthTokenOptions
	refreshResponse  *openai.TokenResponse
	refreshErr       error
}

func (s *openaiOAuthClientRefreshStub) ExchangeCode(ctx context.Context, code, codeVerifier, redirectURI, proxyURL string, opts OpenAIOAuthTokenOptions) (*openai.TokenResponse, error) {
	return nil, errors.New("not implemented")
}

func (s *openaiOAuthClientRefreshStub) RefreshToken(ctx context.Context, refreshToken, proxyURL string) (*openai.TokenResponse, error) {
	return s.RefreshTokenWithOptions(ctx, refreshToken, proxyURL, OpenAIOAuthTokenOptions{})
}

func (s *openaiOAuthClientRefreshStub) RefreshTokenWithClientID(ctx context.Context, refreshToken, proxyURL string, clientID string) (*openai.TokenResponse, error) {
	return s.RefreshTokenWithOptions(ctx, refreshToken, proxyURL, OpenAIOAuthTokenOptions{ClientID: clientID})
}

func (s *openaiOAuthClientRefreshStub) RefreshTokenWithOptions(ctx context.Context, refreshToken, proxyURL string, opts OpenAIOAuthTokenOptions) (*openai.TokenResponse, error) {
	atomic.AddInt32(&s.refreshCalls, 1)
	s.lastRefreshToken = refreshToken
	s.lastProxyURL = proxyURL
	s.lastOpts = opts
	if s.refreshErr != nil {
		return nil, s.refreshErr
	}
	if s.refreshResponse != nil {
		return s.refreshResponse, nil
	}
	return &openai.TokenResponse{
		AccessToken:  "refreshed-access-token",
		RefreshToken: "refreshed-refresh-token",
		ExpiresIn:    3600,
	}, nil
}

func TestOpenAIOAuthService_RefreshAccountToken_NoRefreshTokenUsesExistingAccessToken(t *testing.T) {
	client := &openaiOAuthClientRefreshStub{}
	svc := NewOpenAIOAuthService(nil, client)
	var privacyClientCalls int32
	svc.SetPrivacyClientFactory(func(proxyURL string) (*req.Client, error) {
		atomic.AddInt32(&privacyClientCalls, 1)
		return nil, errors.New("stop before request")
	})

	expiresAt := time.Now().Add(30 * time.Minute).UTC().Format(time.RFC3339)
	account := &Account{
		ID:       77,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "existing-access-token",
			"expires_at":   expiresAt,
			"client_id":    "client-id-1",
		},
	}

	info, err := svc.RefreshAccountToken(context.Background(), account)
	require.NoError(t, err)
	require.NotNil(t, info)
	require.Equal(t, "existing-access-token", info.AccessToken)
	require.Equal(t, "client-id-1", info.ClientID)
	require.Zero(t, atomic.LoadInt32(&client.refreshCalls), "existing access token should be reused without calling refresh")
	require.Positive(t, atomic.LoadInt32(&privacyClientCalls), "existing access token should still run enrichment")
}

func TestOpenAIOAuthService_RefreshAccountToken_SetupTokenWithAccessTokenUsesExistingAccessToken(t *testing.T) {
	client := &openaiOAuthClientRefreshStub{}
	svc := NewOpenAIOAuthService(nil, client)
	var privacyClientCalls int32
	svc.SetPrivacyClientFactory(func(proxyURL string) (*req.Client, error) {
		atomic.AddInt32(&privacyClientCalls, 1)
		return nil, errors.New("stop before request")
	})

	account := &Account{
		ID:       78,
		Platform: PlatformOpenAI,
		Type:     AccountTypeSetupToken,
		Credentials: map[string]any{
			"access_token": "setup-access-token",
		},
	}

	info, err := svc.RefreshAccountToken(context.Background(), account)
	require.NoError(t, err)
	require.NotNil(t, info)
	require.Equal(t, "setup-access-token", info.AccessToken)
	require.NotNil(t, info.CodexFingerprint)
	require.Zero(t, atomic.LoadInt32(&client.refreshCalls), "setup-token access token reuse must not call token refresh")
	require.Positive(t, atomic.LoadInt32(&privacyClientCalls), "setup-token access token reuse should still run enrichment")
}

func TestOpenAIOAuthService_RefreshAccountToken_UsesAccountFingerprintUAProfile(t *testing.T) {
	client := &openaiOAuthClientRefreshStub{}
	svc := NewOpenAIOAuthService(nil, client)
	fingerprint, _ := NormalizeOpenAICodexFingerprint(OpenAICodexFingerprint{
		SchemaVersion:  openAICodexFingerprintSchemaV1,
		InstallationID: "550e8400-e29b-41d4-a716-446655440000",
		UAProfile: OpenAICodexUAProfile{
			Originator:    "account-originator",
			CodexVersion:  "9.8.7",
			OSFingerprint: "Test OS; amd64",
			TerminalToken: "Test_Terminal/1.0",
		},
		CreatedAt: "2026-06-16T00:00:00Z",
		UpdatedAt: "2026-06-16T00:00:00Z",
	}, ParseOpenAICodexUAProfile(DefaultOpenAICodexUserAgent), time.Now())
	account := &Account{
		ID:       79,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"refresh_token": "account-refresh-token",
			"client_id":     "account-client-id",
		},
		Extra: map[string]any{
			OpenAICodexFingerprintExtraKey: fingerprint,
		},
	}

	info, err := svc.RefreshAccountToken(context.Background(), account)
	require.NoError(t, err)
	require.NotNil(t, info)
	require.Equal(t, int32(1), atomic.LoadInt32(&client.refreshCalls))
	require.Equal(t, "account-refresh-token", client.lastRefreshToken)
	require.Equal(t, "account-client-id", client.lastOpts.ClientID)
	require.Equal(t, fingerprint.UAProfile, client.lastOpts.UAProfile)
	require.Equal(t, fingerprint, *info.CodexFingerprint)
}

func TestOpenAIOAuthService_RefreshTokenForNewAccount_UsesFreshFingerprintUAProfile(t *testing.T) {
	client := &openaiOAuthClientRefreshStub{}
	svc := NewOpenAIOAuthService(nil, client)
	profile := ParseOpenAICodexUAProfile("manual-originator/7.8.9 (Manual OS; arm64) Manual_Terminal/2.0 (manual-originator; 7.8.9)")
	svc.SetCodexFingerprintDependencies(nil, openAICodexFingerprintUAProviderStub{ua: profile.UserAgent()})

	info, err := svc.RefreshTokenForNewAccount(context.Background(), "manual-refresh-token", "http://proxy.example", "manual-client-id")
	require.NoError(t, err)
	require.NotNil(t, info)
	require.Equal(t, int32(1), atomic.LoadInt32(&client.refreshCalls))
	require.Equal(t, "manual-refresh-token", client.lastRefreshToken)
	require.Equal(t, "http://proxy.example", client.lastProxyURL)
	require.Equal(t, "manual-client-id", client.lastOpts.ClientID)
	require.Equal(t, profile, client.lastOpts.UAProfile)
	require.NotNil(t, info.CodexFingerprint)
	require.NotEmpty(t, info.CodexFingerprint.InstallationID)
	require.Equal(t, profile, info.CodexFingerprint.UAProfile)
}

func TestOpenAITokenRefresher_NeedsRefresh_SkipsAccountWithoutRefreshToken(t *testing.T) {
	refresher := NewOpenAITokenRefresher(nil, nil)
	expiresAt := time.Now().Add(time.Minute).UTC().Format(time.RFC3339)

	withoutRT := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "access-token",
			"expires_at":   expiresAt,
		},
	}
	require.False(t, refresher.NeedsRefresh(withoutRT, 5*time.Minute))

	withRT := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":  "access-token",
			"refresh_token": "refresh-token",
			"expires_at":    expiresAt,
		},
	}
	require.True(t, refresher.NeedsRefresh(withRT, 5*time.Minute))
}

func TestOpenAITokenProvider_NoRefreshTokenExpiredAccessTokenReturnsError(t *testing.T) {
	provider := NewOpenAITokenProvider(nil, nil, nil)
	expiresAt := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "expired-access-token",
			"expires_at":   expiresAt,
		},
	}

	token, err := provider.GetAccessToken(context.Background(), account)
	require.Error(t, err)
	require.Empty(t, token)
	require.Contains(t, err.Error(), "refresh_token is missing")
}
