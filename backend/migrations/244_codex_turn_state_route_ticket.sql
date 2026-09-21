ALTER TABLE codex_turn_states
    ADD COLUMN IF NOT EXISTS source_session_id VARCHAR(255),
    ADD COLUMN IF NOT EXISTS source_proxy_id BIGINT,
    ADD COLUMN IF NOT EXISTS source_proxy_url TEXT,
    ADD COLUMN IF NOT EXISTS source_exit_ip VARCHAR(64);

ALTER TABLE codex_turn_state_proxies
    ADD COLUMN IF NOT EXISTS route_binding_enabled BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_codex_turn_states_verified_ticket
    ON codex_turn_states (source_account_id, source_model, expires_at DESC, last_seen_at DESC)
    WHERE source_transport = 'scanner' AND source_session_id IS NOT NULL;
