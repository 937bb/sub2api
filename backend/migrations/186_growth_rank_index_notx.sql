CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_growth_rank
    ON usage_logs (created_at, user_id) INCLUDE (actual_cost);
