ALTER TABLE growth_configs
    ALTER COLUMN leaderboard_anonymous SET DEFAULT FALSE;

UPDATE growth_configs
SET leaderboard_anonymous = FALSE,
    updated_at = NOW()
WHERE leaderboard_anonymous = TRUE;
