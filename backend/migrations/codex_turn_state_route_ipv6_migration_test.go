package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCodexTurnStateRouteIPv6Migration(t *testing.T) {
	content, err := FS.ReadFile("245_codex_turn_state_route_ipv6.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(content))
	require.Contains(t, sql, "route_ipv6")
	require.Contains(t, sql, "add column if not exists")
}
