package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSchedulerDirtyWorkDegradationMigrationIsBoundedCurrentState(t *testing.T) {
	raw, err := FS.ReadFile("162_scheduler_dirty_work_degradation.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(raw))

	require.Contains(t, sql, "failure_count between 0 and 31")
	require.Contains(t, sql, "last_error varchar(512)")
	require.Contains(t, sql, "create index if not exists idx_scheduler_dirty_work_retry_pending")
	require.Contains(t, sql, "on scheduler_dirty_work (retry_at, updated_at, kind, entity_id)")
	require.NotContains(t, sql, "drop index")
	require.NotContains(t, sql, "create table")
	require.NotContains(t, sql, "payload")
	require.NotContains(t, sql, "history")
}
