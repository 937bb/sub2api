ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS openai_upstream_mode VARCHAR(20) NOT NULL DEFAULT 'codex';

UPDATE groups
SET openai_upstream_mode = 'codex'
WHERE openai_upstream_mode IS NULL
   OR openai_upstream_mode NOT IN ('codex', 'word');

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'groups_openai_upstream_mode_check'
    ) THEN
        ALTER TABLE groups
            ADD CONSTRAINT groups_openai_upstream_mode_check
            CHECK (openai_upstream_mode IN ('codex', 'word'));
    END IF;
END $$;
