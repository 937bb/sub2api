-- Growth incentives: daily check-in, auditable rewards, and leaderboard settlement.

CREATE TABLE IF NOT EXISTS growth_configs (
    id                              INT PRIMARY KEY DEFAULT 1,
    checkin_enabled                 BOOLEAN NOT NULL DEFAULT FALSE,
    checkin_reward_mode             VARCHAR(16) NOT NULL DEFAULT 'fixed',
    checkin_fixed_reward            DECIMAL(20,8) NOT NULL DEFAULT 0.10,
    checkin_min_reward              DECIMAL(20,8) NOT NULL DEFAULT 1.00,
    checkin_max_reward              DECIMAL(20,8) NOT NULL DEFAULT 1.00,
    checkin_streak_rewards          JSONB NOT NULL DEFAULT '[]'::jsonb,
    checkin_min_account_age_days    INT NOT NULL DEFAULT 1,
    checkin_min_total_recharged     DECIMAL(20,8) NOT NULL DEFAULT 1.00,
    checkin_max_reward_paid_ratio   DECIMAL(10,4) NOT NULL DEFAULT 0.20,
    max_total_reward_paid_ratio     DECIMAL(10,4) NOT NULL DEFAULT 0.20,
    checkin_min_recent_spend        DECIMAL(20,8) NOT NULL DEFAULT 0.01,
    checkin_recent_spend_days       INT NOT NULL DEFAULT 30,
    checkin_max_accounts_per_ip     INT NOT NULL DEFAULT 2,
    checkin_max_accounts_per_device INT NOT NULL DEFAULT 1,
    leaderboard_enabled             BOOLEAN NOT NULL DEFAULT FALSE,
    leaderboard_anonymous           BOOLEAN NOT NULL DEFAULT TRUE,
    leaderboard_reward_rules        JSONB NOT NULL DEFAULT '[]'::jsonb,
    updated_by                      BIGINT,
    created_at                      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT growth_configs_singleton CHECK (id = 1),
    CONSTRAINT growth_configs_reward_mode CHECK (checkin_reward_mode IN ('fixed', 'random')),
    CONSTRAINT growth_configs_rewards_nonnegative CHECK (
        checkin_fixed_reward > 0 AND checkin_fixed_reward <= 100
        AND checkin_min_reward BETWEEN 1 AND 100
        AND checkin_max_reward BETWEEN checkin_min_reward AND 100
    ),
    CONSTRAINT growth_configs_limits_valid CHECK (
        checkin_min_account_age_days >= 0 AND checkin_recent_spend_days BETWEEN 1 AND 365
        AND checkin_min_total_recharged >= 0 AND checkin_min_recent_spend >= 0
        AND checkin_max_reward_paid_ratio > 0 AND checkin_max_reward_paid_ratio <= 1
        AND max_total_reward_paid_ratio > 0 AND max_total_reward_paid_ratio <= 1
        AND checkin_max_accounts_per_ip >= 1 AND checkin_max_accounts_per_device >= 1
    )
);

INSERT INTO growth_configs (id) VALUES (1) ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS growth_reward_ledger (
    id              BIGSERIAL PRIMARY KEY,
    user_id         BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source_type     VARCHAR(32) NOT NULL,
    source_key      VARCHAR(160) NOT NULL,
    amount          DECIMAL(20,8) NOT NULL,
    balance_after   DECIMAL(20,8) NOT NULL,
    metadata        JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT growth_reward_ledger_amount_positive CHECK (amount > 0),
    CONSTRAINT growth_reward_ledger_source_unique UNIQUE (source_type, source_key)
);

CREATE INDEX IF NOT EXISTS idx_growth_reward_ledger_user_time
    ON growth_reward_ledger (user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS growth_checkins (
    id              BIGSERIAL PRIMARY KEY,
    user_id         BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    checkin_date    DATE NOT NULL,
    streak_days     INT NOT NULL,
    base_reward     DECIMAL(20,8) NOT NULL,
    streak_reward   DECIMAL(20,8) NOT NULL DEFAULT 0,
    total_reward    DECIMAL(20,8) NOT NULL,
    ip_hash         CHAR(64) NOT NULL,
    device_hash     CHAR(64) NOT NULL,
    ledger_id       BIGINT NOT NULL REFERENCES growth_reward_ledger(id) ON DELETE RESTRICT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT growth_checkins_user_date_unique UNIQUE (user_id, checkin_date),
    CONSTRAINT growth_checkins_streak_positive CHECK (streak_days > 0),
    CONSTRAINT growth_checkins_rewards_nonnegative CHECK (
        base_reward >= 0 AND streak_reward >= 0 AND total_reward > 0
    )
);

CREATE INDEX IF NOT EXISTS idx_growth_checkins_user_date
    ON growth_checkins (user_id, checkin_date DESC);
CREATE INDEX IF NOT EXISTS idx_growth_checkins_ip_date
    ON growth_checkins (ip_hash, checkin_date);
CREATE INDEX IF NOT EXISTS idx_growth_checkins_device_date
    ON growth_checkins (device_hash, checkin_date);

CREATE TABLE IF NOT EXISTS growth_risk_events (
    id              BIGSERIAL PRIMARY KEY,
    user_id         BIGINT REFERENCES users(id) ON DELETE SET NULL,
    event_type      VARCHAR(32) NOT NULL,
    decision        VARCHAR(16) NOT NULL,
    reason_code     VARCHAR(64) NOT NULL,
    ip_hash         CHAR(64) NOT NULL,
    device_hash     CHAR(64) NOT NULL,
    evidence        JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_growth_risk_events_user_time
    ON growth_risk_events (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_growth_risk_events_ip_time
    ON growth_risk_events (ip_hash, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_growth_risk_events_device_time
    ON growth_risk_events (device_hash, created_at DESC);

CREATE TABLE IF NOT EXISTS growth_leaderboard_settlements (
    id              BIGSERIAL PRIMARY KEY,
    period_type     VARCHAR(16) NOT NULL,
    period_start    TIMESTAMPTZ NOT NULL,
    period_end      TIMESTAMPTZ NOT NULL,
    rewarded_users  INT NOT NULL DEFAULT 0,
    total_reward    DECIMAL(20,8) NOT NULL DEFAULT 0,
    rule_snapshot   JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT growth_leaderboard_settlement_period CHECK (period_type IN ('daily', 'weekly', 'monthly')),
    CONSTRAINT growth_leaderboard_settlement_unique UNIQUE (period_type, period_start, period_end)
);
