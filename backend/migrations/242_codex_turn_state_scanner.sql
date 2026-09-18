CREATE TABLE IF NOT EXISTS codex_turn_state_proxies (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL DEFAULT '',
    proxy_url TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    health_status VARCHAR(20) NOT NULL DEFAULT 'unknown',
    consecutive_failures INT NOT NULL DEFAULT 0,
    last_checked_at TIMESTAMPTZ,
    last_success_at TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_codex_turn_state_proxies_url
    ON codex_turn_state_proxies (proxy_url);
CREATE INDEX IF NOT EXISTS idx_codex_turn_state_proxies_enabled_health
    ON codex_turn_state_proxies (enabled, health_status, last_success_at);

CREATE TABLE IF NOT EXISTS codex_turn_state_scans (
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    model VARCHAR(255) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    attempt_count INT NOT NULL DEFAULT 0,
    last_proxy_id BIGINT REFERENCES codex_turn_state_proxies(id) ON DELETE SET NULL,
    last_proxy_url TEXT NOT NULL DEFAULT '',
    last_state_length INT NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    last_attempt_at TIMESTAMPTZ,
    last_success_at TIMESTAMPTZ,
    next_attempt_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (account_id, model)
);

ALTER TABLE codex_turn_state_scans
    ADD COLUMN IF NOT EXISTS last_proxy_url TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_codex_turn_state_scans_due
    ON codex_turn_state_scans (status, next_attempt_at, updated_at);
