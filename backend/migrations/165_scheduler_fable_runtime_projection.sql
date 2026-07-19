-- Extend the exact scheduler-neutral runtime overlay allowlist without editing
-- the published migration that originally created this function.
CREATE OR REPLACE FUNCTION scheduler_account_lifecycle_extra(extra_value jsonb)
RETURNS jsonb
LANGUAGE sql
IMMUTABLE
PARALLEL SAFE
SET search_path = pg_catalog
AS $$
    SELECT CASE
        WHEN extra_value IS NULL THEN '{}'::jsonb
        WHEN jsonb_typeof(extra_value) <> 'object' THEN extra_value
        ELSE COALESCE(
            (
                SELECT jsonb_object_agg(entry.key, entry.value)
                FROM jsonb_each(extra_value) AS entry
                WHERE entry.key NOT IN (
                    'codex_usage_updated_at',
                    'model_rate_limits',
                    'openai_codex_fingerprint',
                    'session_window_utilization',
                    'codex_primary_used_percent',
                    'codex_primary_reset_after_seconds',
                    'codex_primary_window_minutes',
                    'codex_primary_over_secondary_percent',
                    'codex_secondary_used_percent',
                    'codex_secondary_reset_after_seconds',
                    'codex_secondary_window_minutes',
                    'codex_5h_used_percent',
                    'codex_5h_reset_after_seconds',
                    'codex_5h_window_minutes',
                    'codex_5h_reset_at',
                    'codex_7d_used_percent',
                    'codex_7d_reset_after_seconds',
                    'codex_7d_window_minutes',
                    'codex_7d_reset_at',
                    'passive_usage_7d_utilization',
                    'passive_usage_7d_reset',
                    'passive_usage_7d_oi_utilization',
                    'passive_usage_7d_oi_reset',
                    'passive_usage_sampled_at'
                )
            ),
            '{}'::jsonb
        )
    END
$$;

REVOKE ALL ON FUNCTION scheduler_account_lifecycle_extra(jsonb) FROM PUBLIC;
