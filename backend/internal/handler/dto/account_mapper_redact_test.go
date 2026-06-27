package dto

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestAccountFromServiceShallow_RedactsSensitiveCredentials(t *testing.T) {
	src := &service.Account{
		ID:       42,
		Name:     "demo",
		Platform: "anthropic",
		Type:     "oauth",
		Credentials: map[string]any{
			"access_token":  "at-secret",
			"refresh_token": "rt-secret",
			"id_token":      "id-secret",
			"api_key":       "sk-secret",
			"base_url":      "https://api.example.com",
			"model_mapping": map[string]any{"foo": "bar"},
		},
	}

	got := AccountFromServiceShallow(src)
	require.NotNil(t, got)

	// 敏感键不在 Credentials 里
	require.NotContains(t, got.Credentials, "access_token")
	require.NotContains(t, got.Credentials, "refresh_token")
	require.NotContains(t, got.Credentials, "id_token")
	require.NotContains(t, got.Credentials, "api_key")
	// 非敏感键保留
	require.Equal(t, "https://api.example.com", got.Credentials["base_url"])
	require.Equal(t, map[string]any{"foo": "bar"}, got.Credentials["model_mapping"])

	// 状态 map 标记敏感键存在
	require.True(t, got.CredentialsStatus["has_access_token"])
	require.True(t, got.CredentialsStatus["has_refresh_token"])
	require.True(t, got.CredentialsStatus["has_id_token"])
	require.True(t, got.CredentialsStatus["has_api_key"])

	// JSON 序列化校验：响应体里不会出现敏感子串
	raw, err := json.Marshal(got)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "rt-secret")
	require.NotContains(t, string(raw), "at-secret")
	require.NotContains(t, string(raw), "sk-secret")
	require.NotContains(t, string(raw), "id-secret")
	// 状态标识应序列化进 JSON
	require.Contains(t, string(raw), "credentials_status")
	require.Contains(t, string(raw), "has_refresh_token")

	// 原始 service.Account 不应被改动
	require.Equal(t, "rt-secret", src.Credentials["refresh_token"])
}

func TestAccountFromServiceShallow_NilCredentialsOmitsStatus(t *testing.T) {
	src := &service.Account{ID: 1, Name: "n", Platform: "anthropic", Type: "oauth"}
	got := AccountFromServiceShallow(src)
	require.NotNil(t, got)
	require.Nil(t, got.Credentials)
	require.Nil(t, got.CredentialsStatus)
}

func TestAccountFromServiceShallow_RedactsOpenAICodexFingerprintExtra(t *testing.T) {
	src := &service.Account{
		ID:       42,
		Name:     "demo",
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeOAuth,
		Extra: map[string]any{
			service.OpenAICodexFingerprintExtraKey: map[string]any{
				"schema_version":  1,
				"installation_id": "550e8400-e29b-41d4-a716-446655440000",
				"ua_profile": map[string]any{
					"originator":     "codex-tui",
					"codex_version":  "0.136.0",
					"os_fingerprint": "Mac OS 26.5.0; arm64",
					"terminal_token": "Apple_Terminal/470.2",
					"raw_user_agent": "codex-tui/0.136.0 (Mac OS 26.5.0; arm64) Apple_Terminal/470.2 (codex-tui; 0.136.0)",
				},
				"created_at": "2026-06-12T00:00:00Z",
				"updated_at": "2026-06-13T00:00:00Z",
			},
			"safe": "value",
		},
	}

	got := AccountFromServiceShallow(src)
	require.NotNil(t, got)
	require.Equal(t, "value", got.Extra["safe"])
	fingerprint, ok := got.Extra[service.OpenAICodexFingerprintExtraKey].(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, fingerprint["present"])
	require.Equal(t, 1, fingerprint["schema_version"])
	require.Equal(t, "2026-06-12T00:00:00Z", fingerprint["created_at"])
	require.NotContains(t, fingerprint, "installation_id")
	require.NotContains(t, fingerprint, "ua_profile")

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "550e8400-e29b-41d4-a716-446655440000")
	require.NotContains(t, string(raw), "Apple_Terminal/470.2")
}

func TestAccountFromServiceShallow_DropsOpenAICodexFingerprintForAPIKey(t *testing.T) {
	src := &service.Account{
		ID:       43,
		Name:     "apikey",
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeAPIKey,
		Extra: map[string]any{
			service.OpenAICodexFingerprintExtraKey: map[string]any{
				"schema_version":  1,
				"installation_id": "550e8400-e29b-41d4-a716-446655440000",
			},
			"safe": "value",
		},
	}

	got := AccountFromServiceShallow(src)
	require.NotNil(t, got)
	require.Equal(t, "value", got.Extra["safe"])
	require.NotContains(t, got.Extra, service.OpenAICodexFingerprintExtraKey)
}
