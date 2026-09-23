-- Persist daily quota advances in the same transaction as the subscription
-- mutation so retries cannot deduct another 24 hours after an ambiguous response.
CREATE TABLE IF NOT EXISTS subscription_daily_quota_advances (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    subscription_id BIGINT NOT NULL REFERENCES user_subscriptions(id) ON DELETE CASCADE,
    idempotency_key_hash VARCHAR(64) NOT NULL,
    response_snapshot JSONB,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT subscription_daily_quota_advances_user_key_unique
        UNIQUE (user_id, idempotency_key_hash)
);

CREATE INDEX IF NOT EXISTS idx_subscription_daily_quota_advances_subscription
    ON subscription_daily_quota_advances (subscription_id);

CREATE INDEX IF NOT EXISTS idx_subscription_daily_quota_advances_expires_at
    ON subscription_daily_quota_advances (expires_at);
