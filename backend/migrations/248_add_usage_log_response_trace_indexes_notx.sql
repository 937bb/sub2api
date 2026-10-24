CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_upstream_response_id
    ON usage_logs (upstream_response_id)
    WHERE upstream_response_id IS NOT NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_previous_response_id
    ON usage_logs (previous_response_id)
    WHERE previous_response_id IS NOT NULL;
