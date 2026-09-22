ALTER TABLE codex_turn_states
    ADD COLUMN IF NOT EXISTS route_cookie TEXT;

COMMENT ON COLUMN codex_turn_states.route_cookie IS
    'Filtered ChatGPT affinity cookies bound to the verified account/model/session/egress route ticket';
