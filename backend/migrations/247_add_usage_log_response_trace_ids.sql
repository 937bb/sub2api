ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS upstream_response_id VARCHAR(128),
    ADD COLUMN IF NOT EXISTS previous_response_id VARCHAR(128);
