package service

import (
	"math"
	"testing"
	"time"
)

func validGrowthConfig() GrowthConfig {
	return GrowthConfig{
		CheckinRewardMode:           GrowthRewardModeRandom,
		CheckinFixedReward:          1,
		CheckinMinReward:            1,
		CheckinMaxReward:            100,
		CheckinMinTotalRecharged:    1,
		CheckinMaxRewardPaidRatio:   1,
		MaxTotalRewardPaidRatio:     1,
		CheckinRecentSpendDays:      30,
		CheckinMaxAccountsPerIP:     2,
		CheckinMaxAccountsPerDevice: 1,
		CheckinStreakRewards:        []GrowthStreakReward{{Days: 7, Amount: 2}},
		LeaderboardRewardRules: []GrowthLeaderboardRewardRule{{
			ID: "daily-top-3", Period: "daily", RankStart: 1, RankEnd: 3,
			RewardAmount: 5, Enabled: true,
		}},
	}
}

func TestGrowthValidateConfig(t *testing.T) {
	config := validGrowthConfig()
	if err := ValidateGrowthConfig(&config); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*GrowthConfig)
	}{
		{"negative fixed reward", func(c *GrowthConfig) { c.CheckinFixedReward = -0.01 }},
		{"negative random reward", func(c *GrowthConfig) { c.CheckinMinReward = -0.01 }},
		{"reward above one hundred", func(c *GrowthConfig) { c.CheckinMaxReward = 100.01 }},
		{"duplicate streak day", func(c *GrowthConfig) {
			c.CheckinStreakRewards = append(c.CheckinStreakRewards, GrowthStreakReward{Days: 7, Amount: 1})
		}},
		{"invalid rank range", func(c *GrowthConfig) { c.LeaderboardRewardRules[0].RankStart = 0 }},
		{"duplicate rule id", func(c *GrowthConfig) {
			c.LeaderboardRewardRules = append(c.LeaderboardRewardRules, c.LeaderboardRewardRules[0])
		}},
		{"fractional cent", func(c *GrowthConfig) { c.CheckinFixedReward = 1.001 }},
		{"invalid total paid ratio", func(c *GrowthConfig) { c.MaxTotalRewardPaidRatio = 1.01 }},
		{"overlapping ranks", func(c *GrowthConfig) {
			c.LeaderboardRewardRules = append(c.LeaderboardRewardRules, GrowthLeaderboardRewardRule{ID: "overlap", Period: "daily", RankStart: 3, RankEnd: 4, RewardAmount: 1, Enabled: true})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := validGrowthConfig()
			test.mutate(&candidate)
			if err := ValidateGrowthConfig(&candidate); err == nil {
				t.Fatal("invalid config accepted")
			}
		})
	}
}

func TestGrowthAllowsZeroCheckinReward(t *testing.T) {
	for _, mode := range []string{GrowthRewardModeFixed, GrowthRewardModeRandom} {
		t.Run(mode, func(t *testing.T) {
			config := validGrowthConfig()
			config.CheckinRewardMode = mode
			config.CheckinFixedReward = 0
			config.CheckinMinReward = 0
			config.CheckinMaxReward = 0

			if err := ValidateGrowthConfig(&config); err != nil {
				t.Fatalf("zero check-in reward rejected: %v", err)
			}
			amount, err := growthRewardAmount(config)
			if err != nil {
				t.Fatal(err)
			}
			if amount != 0 {
				t.Fatalf("expected zero reward, got %.2f", amount)
			}
		})
	}
}

func TestGrowthRandomRewardUsesTwoDecimalRange(t *testing.T) {
	config := validGrowthConfig()
	config.CheckinMinReward = 1.23
	config.CheckinMaxReward = 1.27
	for i := 0; i < 100; i++ {
		amount, err := growthRewardAmount(config)
		if err != nil {
			t.Fatal(err)
		}
		if amount < 1.23 || amount > 1.27 {
			t.Fatalf("amount %.4f outside configured range", amount)
		}
		if math.Abs(amount*100-math.Round(amount*100)) > 1e-9 {
			t.Fatalf("amount %.4f has more than two decimals", amount)
		}
	}
}

func TestGrowthPeriodBounds(t *testing.T) {
	now := time.Date(2026, time.July, 19, 15, 30, 0, 0, time.Local)
	for _, period := range []string{"daily", "weekly", "monthly"} {
		t.Run(period, func(t *testing.T) {
			currentStart, currentEnd, normalized, err := growthPeriodBounds(period, now, false)
			if err != nil {
				t.Fatal(err)
			}
			previousStart, previousEnd, _, err := growthPeriodBounds(period, now, true)
			if err != nil {
				t.Fatal(err)
			}
			if normalized != period || !previousEnd.Equal(currentStart) || !previousStart.Before(previousEnd) || !currentStart.Before(currentEnd) {
				t.Fatalf("invalid bounds: previous=[%s,%s), current=[%s,%s)", previousStart, previousEnd, currentStart, currentEnd)
			}
		})
	}
}
