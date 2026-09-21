ALTER TABLE codex_turn_states
    ADD COLUMN IF NOT EXISTS route_ipv6 VARCHAR(64);

CREATE INDEX IF NOT EXISTS idx_codex_turn_states_route_ipv6
    ON codex_turn_states (route_ipv6)
    WHERE route_ipv6 IS NOT NULL;
