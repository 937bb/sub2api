ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS quota_bypass_applied BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS quota_bypass_inject_pairs INTEGER NOT NULL DEFAULT 0;

ALTER TABLE usage_logs
    DROP CONSTRAINT IF EXISTS usage_logs_quota_bypass_inject_pairs_check;

ALTER TABLE usage_logs
    ADD CONSTRAINT usage_logs_quota_bypass_inject_pairs_check
    CHECK (quota_bypass_inject_pairs >= 0 AND quota_bypass_inject_pairs <= 16);
