-- Bind up to five request-egress proxies to each Codex OAuth account.
-- The existing accounts.proxy_id remains the OAuth/login proxy and fallback.
CREATE TABLE IF NOT EXISTS account_codex_proxies (
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    proxy_id BIGINT NOT NULL REFERENCES proxies(id) ON DELETE CASCADE,
    position SMALLINT NOT NULL CHECK (position BETWEEN 1 AND 5),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (account_id, proxy_id),
    UNIQUE (account_id, position)
);

CREATE INDEX IF NOT EXISTS idx_account_codex_proxies_proxy_id
    ON account_codex_proxies (proxy_id);
