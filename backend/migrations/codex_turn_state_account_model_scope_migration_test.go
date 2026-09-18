package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCodexTurnStateAccountModelScopeMigration(t *testing.T) {
	content, err := FS.ReadFile("241_codex_turn_state_account_model_scope.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS source_model VARCHAR(255) NOT NULL DEFAULT ''")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS issued_at TIMESTAMPTZ")
	require.Contains(t, sql, "idx_codex_turn_states_account_model_active")
	require.NotContains(t, sql, "DROP CONSTRAINT")
}
