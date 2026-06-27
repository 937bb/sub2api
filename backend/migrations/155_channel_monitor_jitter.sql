ALTER TABLE channel_monitors
    ADD COLUMN IF NOT EXISTS jitter_seconds INTEGER NOT NULL DEFAULT 0;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints
        WHERE constraint_name = 'channel_monitors_jitter_check'
          AND table_name = 'channel_monitors'
    ) THEN
        ALTER TABLE channel_monitors
            ADD CONSTRAINT channel_monitors_jitter_check
            CHECK (jitter_seconds >= 0 AND interval_seconds - jitter_seconds >= 15);
    END IF;
END $$;
