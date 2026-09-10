-- Run in a transaction before starting v0.2.3 beside a pre-v0.2.3 instance.
-- Keep the legacy column for draining workers and rollback; migrations 235/236
-- see both columns and preserve them instead of renaming the legacy column.
SET LOCAL lock_timeout = '1s';
SET LOCAL statement_timeout = '8s';

ALTER TABLE groups ADD COLUMN IF NOT EXISTS model_allowlist JSONB NOT NULL DEFAULT '{}'::jsonb;
UPDATE groups SET model_allowlist = models_list_config
WHERE model_allowlist = '{}'::jsonb AND models_list_config <> '{}'::jsonb;

CREATE OR REPLACE FUNCTION sub2api_v023_group_models_sync() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.model_allowlist = '{}'::jsonb AND NEW.models_list_config <> '{}'::jsonb THEN
            NEW.model_allowlist := NEW.models_list_config;
        ELSE
            NEW.models_list_config := NEW.model_allowlist;
        END IF;
    ELSIF NEW.model_allowlist IS DISTINCT FROM OLD.model_allowlist THEN
        NEW.models_list_config := NEW.model_allowlist;
    ELSIF NEW.models_list_config IS DISTINCT FROM OLD.models_list_config THEN
        NEW.model_allowlist := NEW.models_list_config;
    END IF;
    RETURN NEW;
END
$$;

CREATE OR REPLACE TRIGGER sub2api_v023_group_models_sync
BEFORE INSERT OR UPDATE OF model_allowlist, models_list_config ON groups
FOR EACH ROW EXECUTE FUNCTION sub2api_v023_group_models_sync();
