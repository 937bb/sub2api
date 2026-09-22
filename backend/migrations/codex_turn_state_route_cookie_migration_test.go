package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCodexTurnStateRouteCookieMigration(t *testing.T) {
	content, err := FS.ReadFile("246_codex_turn_state_route_cookie.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(content))
	require.Contains(t, sql, "add column if not exists route_cookie text")
}
