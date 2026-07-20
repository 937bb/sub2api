ALTER TABLE growth_configs
    DROP CONSTRAINT IF EXISTS growth_configs_rewards_nonnegative;

ALTER TABLE growth_configs
    ADD CONSTRAINT growth_configs_rewards_nonnegative CHECK (
        checkin_fixed_reward BETWEEN 0 AND 100
        AND checkin_min_reward BETWEEN 0 AND 100
        AND checkin_max_reward BETWEEN checkin_min_reward AND 100
    );

ALTER TABLE growth_reward_ledger
    DROP CONSTRAINT IF EXISTS growth_reward_ledger_amount_positive;

ALTER TABLE growth_reward_ledger
    ADD CONSTRAINT growth_reward_ledger_amount_nonnegative CHECK (amount >= 0);

ALTER TABLE growth_checkins
    DROP CONSTRAINT IF EXISTS growth_checkins_rewards_nonnegative;

ALTER TABLE growth_checkins
    ADD CONSTRAINT growth_checkins_rewards_nonnegative CHECK (
        base_reward >= 0 AND streak_reward >= 0 AND total_reward >= 0
    );
