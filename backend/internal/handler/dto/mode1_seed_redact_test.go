package dto

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestMode1SeedStaysServerManaged(t *testing.T) {
	account := &service.Account{
		ID: 1,
		Extra: map[string]any{
			"codex_fingerprint_seed": "11111111-1111-4111-8111-111111111111",
			"codex_fingerprint_mode": "device",
			"anti_degrade":           map[string]any{"enabled": true},
		},
	}

	out := AccountFromService(account)

	require.NotContains(t, out.Extra, "codex_fingerprint_seed")
	require.Contains(t, account.Extra, "codex_fingerprint_seed")
	require.Equal(t, "device", out.Extra["codex_fingerprint_mode"])
}
