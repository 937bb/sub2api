package kiro

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNormalizeNullableAccountManagerExport(t *testing.T) {
	raw := []byte(`[{"email":"test@example.invalid","provider":"Google","refreshToken":"test-refresh","accessToken":"test-access","idToken":null,"expiresAt":null,"region":null,"clientId":null,"clientSecret":null,"authMethod":"social","machineId":"46e9eb47-a04f-4139-8745-2b8a803af5c9","status":"invalid","enabled":false}]`)
	rows, err := ParseImport(raw)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	c := rows[0].Credentials
	require.Equal(t, "test-refresh", c["refresh_token"])
	require.Equal(t, "46e9eb47-a04f-4139-8745-2b8a803af5c9", c["machine_id"])
	require.Equal(t, "social", c["auth_method"])
	for _, key := range []string{"id_token", "expires_at", "region", "client_id", "client_secret"} {
		require.NotContains(t, c, key)
	}
}

func TestNormalizeNullableAliasesAndNestedFields(t *testing.T) {
	for _, raw := range []map[string]any{
		{"refreshToken": nil, "refresh_token": "test"},
		{"refreshToken": "test", "refresh_token": nil},
		{"refreshToken": "test", "credentials": map[string]any{"refreshToken": nil}},
		{"refreshToken": "test", "credentials": nil},
	} {
		c, err := Normalize(raw)
		require.NoError(t, err)
		require.Equal(t, "test", c["refresh_token"])
	}
	for _, raw := range []map[string]any{
		{"refreshToken": nil},
		{"refreshToken": "test", "idToken": 42},
		{"refreshToken": "test", "refresh_token": "different"},
		{"refreshToken": "test", "credentials": false},
	} {
		_, err := Normalize(raw)
		require.Error(t, err)
	}
}

func TestNullableUpdatePreservesEnvironment(t *testing.T) {
	existing, err := Normalize(map[string]any{"refreshToken": "old", "machineId": "original-machine-id", "region": "eu-central-1", "clientId": "client", "clientSecret": "secret"})
	require.NoError(t, err)
	nested := map[string]any{"refreshToken": "new", "machineId": nil, "region": nil, "clientId": nil, "kiroVersion": nil}
	input := map[string]any{"credentials": nested}
	preserved := PreserveEnvironment(existing, input)
	preserved["client_secret"] = "secret"
	c, err := Normalize(preserved)
	require.NoError(t, err)
	require.Equal(t, AccountEnvironment(existing), AccountEnvironment(c))
	require.Equal(t, "client", c["client_id"])
	require.Nil(t, nested["machineId"])
	require.Contains(t, nested, "machineId")
}

func TestProfileRegionOnlyChangesDataPlane(t *testing.T) {
	c := map[string]any{"region": "us-east-1", "profile_arn": "arn:aws:codewhisperer:eu-central-1:123456789012:profile/REAL123"}
	require.Equal(t, "eu-central-1", Region(c, "api"))
	require.Equal(t, "us-east-1", Region(c, "auth"))
	c["api_region"] = "us-west-2"
	require.Equal(t, "us-west-2", Region(c, "api"))
	delete(c, "api_region")
	c["region"] = "eu-west-1"
	for _, arn := range []string{"arn:aws:codewhisperer:us-east-1:638616132270:profile/AAAACCCCXXXX", "arn:aws:codewhisperer:bad:123456789012:profile/A", "arn:aws:codewhisperer:us-east-1:123456789012:profile/", "arn:aws:s3:us-east-1:123456789012:profile/A"} {
		c["profile_arn"] = arn
		require.Equal(t, "eu-west-1", Region(c, "api"), arn)
	}
}
