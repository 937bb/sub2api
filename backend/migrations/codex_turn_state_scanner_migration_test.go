package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCodexTurnStateScannerMigration(t *testing.T) {
	content, err := FS.ReadFile("242_codex_turn_state_scanner.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS codex_turn_state_proxies")
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS codex_turn_state_scans")
	require.Contains(t, sql, "PRIMARY KEY (account_id, model)")
	require.Contains(t, sql, "last_proxy_url TEXT NOT NULL DEFAULT ''")
	require.Contains(t, sql, "REFERENCES accounts(id) ON DELETE CASCADE")
}
