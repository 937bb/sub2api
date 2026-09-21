package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCodexTurnStateRouteTicketMigration(t *testing.T) {
	content, err := FS.ReadFile("244_codex_turn_state_route_ticket.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(content))

	for _, column := range []string{"source_session_id", "source_proxy_id", "source_proxy_url", "source_exit_ip"} {
		require.Contains(t, sql, "add column if not exists "+column)
	}
	require.Contains(t, sql, "source_transport = 'scanner'")
	require.Contains(t, sql, "source_session_id is not null")
	require.Contains(t, sql, "add column if not exists route_binding_enabled")
	require.Contains(t, sql, "default false")
}
