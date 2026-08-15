ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS openai_model_mapping JSONB NOT NULL DEFAULT '{}'::jsonb;

COMMENT ON COLUMN groups.openai_model_mapping IS
    'OpenAI group-level request model mapping; matched targets are final upstream model IDs';
