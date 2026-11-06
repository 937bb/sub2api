package kiro

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKiroEnvironmentSurvivesCredentialReplacement(t *testing.T) {
	original, err := Normalize(map[string]any{"refresh_token": "old", "machine_id": strings.Repeat("a", 64), "system_version": "win32#10.0.22631", "api_region": "eu-central-1"})
	require.NoError(t, err)
	for _, incoming := range []map[string]any{{"refresh_token": "new"}, {"credentials": map[string]any{"refreshToken": "new"}}} {
		updated, err := Normalize(PreserveEnvironment(original, incoming))
		require.NoError(t, err)
		require.Equal(t, AccountEnvironment(original), AccountEnvironment(updated))
		require.Equal(t, "new", updated["refresh_token"])
		require.NotContains(t, incoming, "machine_id")
	}
	for _, incoming := range []map[string]any{
		{"refresh_token": "new", "machineId": strings.Repeat("b", 64)},
		{"credentials": map[string]any{"refreshToken": "new", "machineId": strings.Repeat("b", 64)}},
	} {
		updated, err := Normalize(PreserveEnvironment(original, incoming))
		require.NoError(t, err)
		require.Equal(t, strings.Repeat("b", 64), updated["machine_id"])
	}
	require.Equal(t, strings.Repeat("a", 64), original["machine_id"])
}

func TestKiroEnvironmentKeyTracksIdentityNotTokenRotation(t *testing.T) {
	c, err := Normalize(map[string]any{"refresh_token": "old"})
	require.NoError(t, err)
	before := EnvironmentKey(c)
	c["refresh_token"] = "new"
	c["access_token"] = "access"
	c["expires_at"] = "new-expiry"
	require.Equal(t, before, EnvironmentKey(c))
	for _, key := range []string{"machine_id", "kiro_version", "node_version", "system_version", "api_region", "auth_region"} {
		next := map[string]any{}
		for k, v := range c {
			next[k] = v
		}
		values := map[string]string{"machine_id": strings.Repeat("b", 64), "kiro_version": "9.9.9", "node_version": "24.0.0", "system_version": "linux#6.8.0", "api_region": "eu-central-1", "auth_region": "us-west-2"}
		next[key] = values[key]
		require.NotEqual(t, before, EnvironmentKey(next), key)
	}
}

func TestKiroOperationHeadersDoNotLeakAcrossOperations(t *testing.T) {
	c, err := Normalize(map[string]any{"kiroApiKey": "private"})
	require.NoError(t, err)
	h := http.Header{}
	OperationHeaders(h, c, "private", OperationModels)
	require.NotEmpty(t, h.Get("X-Amz-Target"))
	require.Equal(t, "API_KEY", h.Get("tokentype"))
	OperationHeaders(h, c, "private", OperationGeneration)
	require.Equal(t, "KiroRuntimeService.GenerateAssistantResponse", h.Get("X-Amz-Target"))
	require.Equal(t, "true", h.Get("x-amzn-codewhisperer-optout"))
	OperationHeaders(h, c, "", OperationRefresh)
	for _, key := range []string{"Authorization", "tokentype", "X-Amz-Target", "x-amzn-kiro-agent-mode", "x-amzn-codewhisperer-optout", "x-amz-user-agent", "amz-sdk-invocation-id"} {
		require.Empty(t, h.Get(key), key)
	}
	OperationHeaders(h, c, "private", OperationUsage)
	require.Empty(t, h.Get("Content-Type"))
	require.Contains(t, h.Get("User-Agent"), String(c, "machine_id"))
}

func TestKiroEnvironmentImportCanExplicitlyChangeAuthentication(t *testing.T) {
	existing, err := Normalize(map[string]any{"refresh_token": "old", "client_id": "client", "client_secret": "secret", "profile_arn": "profile"})
	require.NoError(t, err)
	updated, err := Normalize(PreserveEnvironment(existing, map[string]any{"kiroApiKey": "new-api-key"}))
	require.NoError(t, err)
	require.Equal(t, "api_key", updated["auth_method"])
	require.Equal(t, existing["machine_id"], updated["machine_id"])
	require.Equal(t, existing["profile_arn"], updated["profile_arn"])
}
