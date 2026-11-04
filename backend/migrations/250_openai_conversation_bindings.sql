-- Permanent routing identity, deliberately independent of expiring sticky caches.
-- Keep tombstones when accounts/keys/groups are removed: deletion must not rebind a conversation.
CREATE TABLE IF NOT EXISTS openai_conversation_bindings (
    binding_key VARCHAR(64) PRIMARY KEY,
    account_id BIGINT NOT NULL CHECK (account_id > 0),
    credential_account_id BIGINT NOT NULL CHECK (credential_account_id > 0),
    identity_hash VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_openai_conversation_bindings_owner
    ON openai_conversation_bindings (credential_account_id);
