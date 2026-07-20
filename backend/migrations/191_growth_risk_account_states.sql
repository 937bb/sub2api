-- Persistent account-level state for growth identity risk controls and audited admin overrides.

CREATE TABLE IF NOT EXISTS growth_risk_account_states (
    user_id          BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    status           VARCHAR(24) NOT NULL DEFAULT 'flagged',
    reason_code      VARCHAR(64) NOT NULL DEFAULT '',
    event_count      INT NOT NULL DEFAULT 1,
    first_flagged_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_flagged_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    action_note      VARCHAR(500) NOT NULL DEFAULT '',
    action_by        BIGINT REFERENCES users(id) ON DELETE SET NULL,
    action_at        TIMESTAMPTZ,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT growth_risk_account_states_status CHECK (
        status IN ('flagged', 'cleared_once', 'whitelisted', 'cleared')
    ),
    CONSTRAINT growth_risk_account_states_event_count CHECK (event_count >= 0)
);

CREATE INDEX IF NOT EXISTS idx_growth_risk_account_states_status_time
    ON growth_risk_account_states (status, last_flagged_at DESC);

INSERT INTO growth_risk_account_states (
    user_id, status, reason_code, event_count, first_flagged_at, last_flagged_at, updated_at
)
SELECT e.user_id,
       'flagged',
       (ARRAY_AGG(e.reason_code ORDER BY e.created_at DESC))[1],
       COUNT(*)::INT,
       MIN(e.created_at),
       MAX(e.created_at),
       NOW()
FROM growth_risk_events e
WHERE e.user_id IS NOT NULL
  AND e.decision = 'denied'
  AND e.reason_code IN ('ip_account_limit', 'device_account_limit', 'missing_identity_signal')
  AND NOT EXISTS (
      SELECT 1 FROM growth_checkins gc
      WHERE gc.user_id = e.user_id AND gc.created_at > e.created_at
  )
GROUP BY e.user_id
ON CONFLICT (user_id) DO NOTHING;
