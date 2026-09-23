package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSubscriptionDailyQuotaRollingWindowMigration(t *testing.T) {
	content, err := FS.ReadFile("239_subscription_daily_quota_rolling_window.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "SET daily_window_start = CURRENT_TIMESTAMP")
	require.Contains(t, sql, "daily_usage_usd = 0")
	require.Contains(t, sql, "status = 'active'")
	require.Contains(t, sql, "expires_at > starts_at + INTERVAL '1 day'")
}
