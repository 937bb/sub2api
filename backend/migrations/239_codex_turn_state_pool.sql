CREATE TABLE IF NOT EXISTS codex_turn_states (
    id BIGSERIAL PRIMARY KEY,
    state_value TEXT NOT NULL,
    state_hash CHAR(64) NOT NULL UNIQUE,
    value_length INTEGER NOT NULL,
    source_account_id BIGINT REFERENCES accounts(id) ON DELETE SET NULL,
    source_session_hash CHAR(64),
    source_transport VARCHAR(16) NOT NULL DEFAULT 'http',
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_codex_turn_states_active_length
    ON codex_turn_states (expires_at, value_length DESC, last_seen_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_codex_turn_states_expires_at
    ON codex_turn_states (expires_at DESC);
