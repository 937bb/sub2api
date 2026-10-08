package admin

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSIWCCredentialsCannotEnterCodexImporter(t *testing.T) {
	for _, raw := range []map[string]any{
		{"auth_mode": "siwc", "access_token": "private"},
		{"client_id": "oaiapp_test", "access_token": "private"},
		{"credentials": map[string]any{"auth_mode": "siwc", "access_token": "private"}},
	} {
		_, err := normalizeCodexImportEntry(codexImportEntry{Value: raw})
		require.ErrorContains(t, err, "SIWC")
		require.NotContains(t, err.Error(), "private")
	}
}
