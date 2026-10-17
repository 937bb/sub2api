package dto

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestSIWCQuotaWebTokenRedactionAndPreservation(t *testing.T) {
	credentials := map[string]any{"auth_mode": "siwc", "access_token": "inference-secret", "siwc_quota_access_token": "quota-secret"}
	account := &service.Account{Platform: "openai", Type: "oauth", Credentials: credentials}
	view := AccountFromServiceShallow(account)
	require.True(t, view.CredentialsStatus["has_siwc_quota_access_token"])
	require.NotContains(t, view.Credentials, "siwc_quota_access_token")
	raw, err := json.Marshal(view)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "quota-secret")
	require.NotContains(t, string(raw), "inference-secret")
	audit := service.RedactAuditBody([]byte(`{"credentials":{"siwc_quota_access_token":"quota-secret"}}`), "application/json")
	require.NotContains(t, audit, "quota-secret")
	merged := service.MergePreservingSensitiveCreds(credentials, map[string]any{"auth_mode": "siwc"})
	require.Equal(t, "quota-secret", merged["siwc_quota_access_token"])
	merged = service.MergePreservingSensitiveCreds(credentials, map[string]any{"siwc_quota_access_token": nil})
	require.Nil(t, merged["siwc_quota_access_token"])
	require.Equal(t, "inference-secret", merged["access_token"])
}
