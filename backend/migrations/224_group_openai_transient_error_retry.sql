ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS openai_transient_error_retry_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS openai_transient_error_retry_count INTEGER NOT NULL DEFAULT 3;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'groups_openai_transient_error_retry_count_check'
          AND conrelid = 'groups'::regclass
    ) THEN
        ALTER TABLE groups
            ADD CONSTRAINT groups_openai_transient_error_retry_count_check
            CHECK (openai_transient_error_retry_count BETWEEN 1 AND 10);
    END IF;
END
$$;

COMMENT ON COLUMN groups.openai_transient_error_retry_enabled IS
    'Whether to retry explicit OpenAI capacity and overload errors before downstream output';

COMMENT ON COLUMN groups.openai_transient_error_retry_count IS
    'Maximum same-account retries for explicit OpenAI capacity and overload errors';
