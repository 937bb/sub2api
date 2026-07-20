ALTER TABLE growth_configs
    ADD COLUMN IF NOT EXISTS leaderboard_display_limit INT NOT NULL DEFAULT 20;

ALTER TABLE growth_configs
    DROP CONSTRAINT IF EXISTS growth_configs_leaderboard_display_limit;

ALTER TABLE growth_configs
    ADD CONSTRAINT growth_configs_leaderboard_display_limit CHECK (
        leaderboard_display_limit BETWEEN 1 AND 100
    );
