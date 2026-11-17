ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS openai_zero_injection BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN groups.openai_zero_injection IS
    'Disable Sub2API-generated OpenAI prompt injection while preserving client-provided instructions';
