ALTER TABLE codex_turn_state_scans
    ADD COLUMN IF NOT EXISTS lease_id TEXT,
    ADD COLUMN IF NOT EXISTS lease_until TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_codex_turn_state_scans_lease
    ON codex_turn_state_scans (lease_until, account_id, model);
