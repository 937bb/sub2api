ALTER TABLE codex_turn_states
    ADD COLUMN IF NOT EXISTS source_model VARCHAR(255) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS issued_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_codex_turn_states_account_model_active
    ON codex_turn_states (source_account_id, source_model, expires_at DESC, last_seen_at DESC)
    WHERE source_account_id IS NOT NULL AND source_model <> '';
