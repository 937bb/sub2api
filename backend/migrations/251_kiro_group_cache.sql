-- Kiro is an account provider. Cache policy belongs to the request group.
ALTER TABLE groups ADD COLUMN IF NOT EXISTS kiro_response_cache JSONB NOT NULL
 DEFAULT '{"enabled":false,"target_percent":100,"ttl_seconds":300,"max_entries":128}'::jsonb;
-- Existing provider groups are converted through the admin API only after
-- cutover, so the old binary can keep serving them during migration.
