package migrations

import (
	"regexp"
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

func TestSchedulerAccountDirtyProjectionExcludesOnlyRuntimeOverlays(t *testing.T) {
	raw, err := FS.ReadFile("164_scheduler_account_dirty_projection.sql")
	require.NoError(t, err)
	sql := strings.ToLower(string(raw))

	for _, column := range []string{
		"rate_limited_at",
		"rate_limit_reset_at",
		"overload_until",
		"temp_unschedulable_until",
		"temp_unschedulable_reason",
		"session_window_start",
		"session_window_end",
		"session_window_status",
	} {
		require.NotContains(t, sql, "o."+column)
		require.NotContains(t, sql, "n."+column)
	}

	exactKeys := []string{
		"codex_usage_updated_at",
		"model_rate_limits",
		"openai_codex_fingerprint",
		"session_window_utilization",
		"codex_primary_used_percent",
		"codex_primary_reset_after_seconds",
		"codex_primary_window_minutes",
		"codex_primary_over_secondary_percent",
		"codex_secondary_used_percent",
		"codex_secondary_reset_after_seconds",
		"codex_secondary_window_minutes",
		"codex_5h_used_percent",
		"codex_5h_reset_after_seconds",
		"codex_5h_window_minutes",
		"codex_5h_reset_at",
		"codex_7d_used_percent",
		"codex_7d_reset_after_seconds",
		"codex_7d_window_minutes",
		"codex_7d_reset_at",
		"passive_usage_7d_utilization",
		"passive_usage_7d_reset",
		"passive_usage_sampled_at",
	}
	require.Len(t, exactKeys, 22)
	for _, exactKey := range exactKeys {
		require.Equal(t, 1, strings.Count(sql, "'"+exactKey+"'"), "runtime Extra key must appear exactly once in the projection allowlist")
	}
	allowlistPattern := regexp.MustCompile(`(?s)where entry\.key not in \((.*?)\)`).FindStringSubmatch(sql)
	require.Len(t, allowlistPattern, 2)
	quotedKeyPattern := regexp.MustCompile(`'([^']+)'`)
	matches := quotedKeyPattern.FindAllStringSubmatch(allowlistPattern[1], -1)
	migrationKeys := make([]string, 0, len(matches))
	for _, match := range matches {
		migrationKeys = append(migrationKeys, match[1])
	}
	require.ElementsMatch(t, exactKeys, migrationKeys)
	require.NotContains(t, sql, "starts_with(entry.key")

	require.Contains(t, sql, "scheduler_account_lifecycle_extra(o.extra) is distinct from")
	require.Contains(t, sql, "o.extra -> 'mixed_scheduling'")
	require.Contains(t, sql, "o.extra is distinct from n.extra")
	require.Contains(t, sql, "jsonb_object_agg(entry.key, entry.value)")
	require.NotContains(t, sql, "where o.extra is distinct from n.extra")
}
