-- Runtime overlays are published separately and must not advance lifecycle
-- generations. Keep the Extra allowlist exact so unknown prefixed keys stay in
-- the projection and remain conservatively dirty.
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
                    'passive_usage_sampled_at'
                )
            ),
            '{}'::jsonb
        )
    END
$$;

REVOKE ALL ON FUNCTION scheduler_account_lifecycle_extra(jsonb) FROM PUBLIC;

CREATE OR REPLACE FUNCTION scheduler_accounts_source_from_update()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog, public
AS $$
BEGIN
    INSERT INTO public.scheduler_dirty_account_sources (account_id, bucket_dirty)
    SELECT
        COALESCE(n.id, o.id),
        CASE
            WHEN n.id IS NULL OR o.id IS NULL THEN true
            ELSE
                o.platform IS DISTINCT FROM n.platform OR
                o.priority IS DISTINCT FROM n.priority OR
                o.status IS DISTINCT FROM n.status OR
                o.expires_at IS DISTINCT FROM n.expires_at OR
                o.schedulable IS DISTINCT FROM n.schedulable OR
                o.deleted_at IS DISTINCT FROM n.deleted_at OR
                o.extra -> 'mixed_scheduling' IS DISTINCT FROM n.extra -> 'mixed_scheduling'
        END
    FROM new_rows AS n
    FULL JOIN old_rows AS o USING (id)
    WHERE
        n.id IS NULL OR
        o.id IS NULL OR
        o.name IS DISTINCT FROM n.name OR
        o.platform IS DISTINCT FROM n.platform OR
        o.type IS DISTINCT FROM n.type OR
        o.credentials IS DISTINCT FROM n.credentials OR
        CASE
            WHEN o.extra IS DISTINCT FROM n.extra THEN
                public.scheduler_account_lifecycle_extra(o.extra) IS DISTINCT FROM
                    public.scheduler_account_lifecycle_extra(n.extra)
            ELSE false
        END OR
        o.proxy_id IS DISTINCT FROM n.proxy_id OR
        o.concurrency IS DISTINCT FROM n.concurrency OR
        o.load_factor IS DISTINCT FROM n.load_factor OR
        o.priority IS DISTINCT FROM n.priority OR
        o.rate_multiplier IS DISTINCT FROM n.rate_multiplier OR
        o.status IS DISTINCT FROM n.status OR
        o.expires_at IS DISTINCT FROM n.expires_at OR
        o.auto_pause_on_expired IS DISTINCT FROM n.auto_pause_on_expired OR
        o.schedulable IS DISTINCT FROM n.schedulable OR
        o.deleted_at IS DISTINCT FROM n.deleted_at
    ORDER BY COALESCE(n.id, o.id)
    ON CONFLICT (account_id) DO UPDATE
    SET generation = public.scheduler_dirty_account_sources.generation + 1,
        bucket_dirty = public.scheduler_dirty_account_sources.bucket_dirty OR EXCLUDED.bucket_dirty,
        group_cursor = 0,
        updated_at = statement_timestamp();
    RETURN NULL;
END
$$;
