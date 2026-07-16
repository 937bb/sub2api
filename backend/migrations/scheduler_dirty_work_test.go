package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSchedulerDirtyWorkMigrationKeepsSourcePromotionContracts(t *testing.T) {
	raw, err := FS.ReadFile("161_scheduler_dirty_work.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(raw))

	require.Contains(t, sql, "primary key (kind, entity_id)")
	require.Contains(t, sql, "kind in (1, 2, 3)")
	require.Contains(t, sql, "on scheduler_dirty_work (updated_at, kind, entity_id)")
	require.Contains(t, sql, "on scheduler_dirty_account_sources (updated_at, account_id)")
	require.Contains(t, sql, "on scheduler_dirty_group_sources (updated_at, group_id)")
	require.Contains(t, sql, "on scheduler_dirty_membership_sources (updated_at, account_id, group_id)")
	for _, table := range []string{
		"scheduler_dirty_account_sources",
		"scheduler_dirty_group_sources",
		"scheduler_dirty_membership_sources",
	} {
		require.Contains(t, sql, "create table if not exists "+table)
	}
	require.Contains(t, sql, "primary key (account_id, group_id)")
	require.Contains(t, sql, "rebuild_buckets boolean not null default false")
	require.Contains(t, sql, "bucket_dirty boolean not null default false")
	require.Contains(t, sql, "group_cursor bigint not null default 0")
	require.Contains(t, sql, "bucket_dirty = public.scheduler_dirty_account_sources.bucket_dirty or excluded.bucket_dirty")
	require.Contains(t, sql, "updated_at = statement_timestamp()")
	require.Contains(t, sql, "referencing old table as old_rows")
	require.Contains(t, sql, "referencing new table as new_rows")
	require.Contains(t, sql, "select id from old_rows order by id")
	require.Contains(t, sql, "select account_id, group_id from old_rows order by account_id, group_id")
	require.Contains(t, sql, "select account_id, group_id from new_rows order by account_id, group_id")
	require.Contains(t, sql, "old_rows as o using (id)")
	require.Contains(t, sql, "o.extra -> 'mixed_scheduling'")
	require.Contains(t, sql, "o.credentials is distinct from n.credentials")
	require.NotContains(t, sql, "old.extra is distinct from new.extra")
	require.NotContains(t, sql, "old.credentials is distinct from new.credentials")
	require.NotContains(t, sql, "quota_used")
	require.NotContains(t, sql, "insert into public.scheduler_dirty_work")
	require.NotContains(t, sql, "scheduler_mark_dirty(")
	require.NotContains(t, sql, "scheduler_dirty_pending")
	require.NotContains(t, sql, "scheduler_outbox")
	require.NotContains(t, sql, "payload")
	require.NotContains(t, sql, "claim")
	require.NotContains(t, sql, "xid")
	require.NotContains(t, sql, "event_id")
}
