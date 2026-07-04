//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/imroc/req/v3"
	"github.com/stretchr/testify/require"
)

type openAIPATTestOAuthClient struct{}

func (openAIPATTestOAuthClient) ExchangeCode(ctx context.Context, code, codeVerifier, redirectURI, proxyURL string, opts OpenAIOAuthTokenOptions) (*openai.TokenResponse, error) {
	return nil, nil
}
func (openAIPATTestOAuthClient) RefreshToken(ctx context.Context, refreshToken, proxyURL string) (*openai.TokenResponse, error) {
	return nil, nil
}
func (openAIPATTestOAuthClient) RefreshTokenWithClientID(ctx context.Context, refreshToken, proxyURL string, clientID string) (*openai.TokenResponse, error) {
	return nil, nil
}
func (openAIPATTestOAuthClient) RefreshTokenWithOptions(ctx context.Context, refreshToken, proxyURL string, opts OpenAIOAuthTokenOptions) (*openai.TokenResponse, error) {
	return nil, nil
}

func TestHydratePersonalAccessTokenCallsOfficialWhoami(t *testing.T) {
	oldBaseURL := openAIAuthAPIBaseURL
	t.Cleanup(func() { openAIAuthAPIBaseURL = oldBaseURL })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, openAIWhoamiPath, r.URL.Path)
		require.Equal(t, "Bearer at-test-pat", r.Header.Get("Authorization"))
		require.Empty(t, r.Header.Values("Accept"))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"email":                      "user@example.com",
			"chatgpt_user_id":            "user-123",
			"chatgpt_account_id":         "acc-123",
			"chatgpt_plan_type":          "plus",
			"chatgpt_account_is_fedramp": true,
		})
	}))
	defer server.Close()
	openAIAuthAPIBaseURL = server.URL

	svc := NewOpenAIOAuthService(nil, openAIPATTestOAuthClient{})
	svc.SetPrivacyClientFactory(func(proxyURL string) (*req.Client, error) {
		return req.C(), nil
	})

	metadata, err := svc.HydratePersonalAccessToken(context.Background(), "at-test-pat", nil)
	require.NoError(t, err)
	require.Equal(t, "user@example.com", metadata.Email)
	require.Equal(t, "user-123", metadata.ChatGPTUserID)
	require.Equal(t, "acc-123", metadata.ChatGPTAccountID)
	require.Equal(t, "plus", metadata.ChatGPTPlanType)
	require.True(t, metadata.ChatGPTAccountIsFedRAMP)
}

func TestHydratePersonalAccessTokenAcceptsExplicitFedRAMPFalse(t *testing.T) {
	oldBaseURL := openAIAuthAPIBaseURL
	t.Cleanup(func() { openAIAuthAPIBaseURL = oldBaseURL })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, openAIWhoamiPath, r.URL.Path)
		require.Equal(t, "Bearer at-test-pat", r.Header.Get("Authorization"))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"email":                      "user@example.com",
			"chatgpt_user_id":            "user-123",
			"chatgpt_account_id":         "acc-123",
			"chatgpt_plan_type":          "plus",
			"chatgpt_account_is_fedramp": false,
		})
	}))
	defer server.Close()
	openAIAuthAPIBaseURL = server.URL

	svc := NewOpenAIOAuthService(nil, openAIPATTestOAuthClient{})
	svc.SetPrivacyClientFactory(func(proxyURL string) (*req.Client, error) {
		return req.C(), nil
	})

	metadata, err := svc.HydratePersonalAccessToken(context.Background(), "at-test-pat", nil)
	require.NoError(t, err)
	require.False(t, metadata.ChatGPTAccountIsFedRAMP)
}

func TestHydratePersonalAccessTokenRequiresExplicitFedRAMPMetadata(t *testing.T) {
	tests := []struct {
		name        string
		fedRAMP     any
		wantMessage string
	}{
		{
			name:        "missing",
			wantMessage: "missing required fields: chatgpt_account_is_fedramp",
		},
		{
			name:        "null",
			fedRAMP:     nil,
			wantMessage: "missing required fields: chatgpt_account_is_fedramp",
		},
		{
			name:        "string",
			fedRAMP:     "false",
			wantMessage: "malformed metadata",
		},
		{
			name:        "number",
			fedRAMP:     0,
			wantMessage: "malformed metadata",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldBaseURL := openAIAuthAPIBaseURL
			t.Cleanup(func() { openAIAuthAPIBaseURL = oldBaseURL })

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, openAIWhoamiPath, r.URL.Path)
				payload := map[string]any{
					"email":              "user@example.com",
					"chatgpt_user_id":    "user-123",
					"chatgpt_account_id": "acc-123",
					"chatgpt_plan_type":  "plus",
				}
				if tt.name != "missing" {
					payload["chatgpt_account_is_fedramp"] = tt.fedRAMP
				}
				_ = json.NewEncoder(w).Encode(payload)
			}))
			defer server.Close()
			openAIAuthAPIBaseURL = server.URL

			svc := NewOpenAIOAuthService(nil, openAIPATTestOAuthClient{})
			svc.SetPrivacyClientFactory(func(proxyURL string) (*req.Client, error) {
				return req.C(), nil
			})

			metadata, err := svc.HydratePersonalAccessToken(context.Background(), "at-test-pat", nil)
			require.Error(t, err)
			require.Nil(t, metadata)
			require.Contains(t, err.Error(), tt.wantMessage)
		})
	}
}

func TestHydratePersonalAccessTokenWhoamiErrorRedactsPAT(t *testing.T) {
	err := (&openAIPersonalAccessTokenWhoamiError{
		statusCode: http.StatusForbidden,
		body: `{"error":{"message":"denied token=at-message-token ` +
			`personal_access_token=at-kv-token Authorization=Bearer at-bearer-token","token":"at-json-token"}}`,
	}).Error()

	require.Contains(t, err, "OpenAI PAT whoami failed")
	require.NotContains(t, err, "at-message-token")
	require.NotContains(t, err, "at-kv-token")
	require.NotContains(t, err, "at-bearer-token")
	require.NotContains(t, err, "at-json-token")
	require.Contains(t, err, "personal_access_token=[redacted]")
	require.Contains(t, err, "Authorization=[redacted]")
}

func TestApplyOpenAIPersonalAccessTokenMetadataPreservesOAuthCredentials(t *testing.T) {
	credentials := map[string]any{
		"personal_access_token": "at-old",
		"access_token":          "oauth-at",
		"refresh_token":         "oauth-rt",
		"id_token":              "id-token",
		"client_id":             "client-id",
	}
	metadata := &OpenAIPersonalAccessTokenMetadata{
		Email:                   "user@example.com",
		ChatGPTUserID:           "user-123",
		ChatGPTAccountID:        "acc-123",
		ChatGPTPlanType:         "pro",
		ChatGPTAccountIsFedRAMP: true,
	}

	out := ApplyOpenAIPersonalAccessTokenMetadata(credentials, "at-new", metadata)
	require.Equal(t, "at-new", out["personal_access_token"])
	require.Equal(t, "oauth-at", out["access_token"])
	require.Equal(t, "oauth-rt", out["refresh_token"])
	require.Equal(t, "id-token", out["id_token"])
	require.Equal(t, "client-id", out["client_id"])
	require.Equal(t, "user@example.com", out["email"])
	require.Equal(t, "user-123", out["chatgpt_user_id"])
	require.Equal(t, "acc-123", out["chatgpt_account_id"])
	require.Equal(t, "pro", out["chatgpt_plan_type"])
	require.Equal(t, "pro", out["plan_type"])
	require.Equal(t, true, out["chatgpt_account_is_fedramp"])
	require.NotContains(t, out, "personal_access_token_sha256")
}

func TestAccountNeedsOpenAIPersonalAccessTokenMetadataHydration(t *testing.T) {
	account := &Account{Credentials: ApplyOpenAIPersonalAccessTokenMetadata(map[string]any{}, "at-token", &OpenAIPersonalAccessTokenMetadata{
		Email:            "user@example.com",
		ChatGPTUserID:    "user-123",
		ChatGPTAccountID: "acc-123",
		ChatGPTPlanType:  "plus",
	})}
	require.False(t, AccountNeedsOpenAIPersonalAccessTokenMetadataHydration(account, "at-token"))
	require.False(t, AccountNeedsOpenAIPersonalAccessTokenMetadataHydration(account, "at-other"))

	account.Credentials = map[string]any{"chatgpt_account_id": "acc-123"}
	require.False(t, AccountNeedsOpenAIPersonalAccessTokenMetadataHydration(account, "at-token"))

	account.Credentials = map[string]any{
		"chatgpt_user_id":    "user-123",
		"chatgpt_account_id": "user-123",
	}
	require.True(t, AccountNeedsOpenAIPersonalAccessTokenMetadataHydration(account, "at-token"))

	delete(account.Credentials, "chatgpt_account_id")
	require.True(t, AccountNeedsOpenAIPersonalAccessTokenMetadataHydration(account, "at-token"))
}

func TestHydratePersonalAccessTokenRequiresATPrefix(t *testing.T) {
	svc := NewOpenAIOAuthService(nil, openAIPATTestOAuthClient{})
	metadata, err := svc.HydratePersonalAccessToken(context.Background(), "oauth-access-token", nil)
	require.Error(t, err)
	require.Nil(t, metadata)
	require.Contains(t, err.Error(), "must start")
}
