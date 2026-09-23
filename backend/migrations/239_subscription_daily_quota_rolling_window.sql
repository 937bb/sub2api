-- Move legacy calendar-day subscription windows to rolling 24-hour windows.
-- One-time daily cards keep their original quota semantics.
UPDATE user_subscriptions
SET daily_window_start = CURRENT_TIMESTAMP,
    daily_usage_usd = 0,
    updated_at = CURRENT_TIMESTAMP
WHERE status = 'active'
  AND expires_at > CURRENT_TIMESTAMP
  AND daily_window_start IS NOT NULL
  AND expires_at > starts_at + INTERVAL '1 day';
