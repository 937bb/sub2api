ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS openai_model_mapping JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS openai_model_mapping_enabled BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN groups.openai_model_mapping_enabled IS
    'Whether OpenAI group-level request model mapping is enabled';
