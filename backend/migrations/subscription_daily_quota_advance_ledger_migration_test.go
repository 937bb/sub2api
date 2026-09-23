package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSubscriptionDailyQuotaAdvanceLedgerMigration(t *testing.T) {
	content, err := FS.ReadFile("240_subscription_daily_quota_advance_ledger.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS subscription_daily_quota_advances")
	require.Contains(t, sql, "UNIQUE (user_id, idempotency_key_hash)")
	require.Contains(t, sql, "response_snapshot JSONB")
	require.Contains(t, sql, "expires_at TIMESTAMPTZ NOT NULL")
}
