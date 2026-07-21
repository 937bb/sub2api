package service

import (
	"encoding/json"
	"math"
	"strings"
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
		LeaderboardDisplayLimit:     20,
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
		{"zero leaderboard display limit", func(c *GrowthConfig) { c.LeaderboardDisplayLimit = 0 }},
		{"excessive leaderboard display limit", func(c *GrowthConfig) { c.LeaderboardDisplayLimit = 101 }},
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

func TestGrowthLeaderboardDisplayNameMasksIdentity(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		rank      int
		anonymous bool
		want      string
	}{
		{name: "long email", value: "599155162@qq.com", rank: 1, want: "59***62@qq.com"},
		{name: "short email", value: "ab@example.com", rank: 2, want: "a***b@example.com"},
		{name: "single character local", value: "a@example.com", rank: 3, want: "a***@example.com"},
		{name: "unicode local", value: "用户名字@example.com", rank: 4, want: "用***字@example.com"},
		{name: "already masked", value: "59***62@qq.com", rank: 5, want: "59***62@qq.com"},
		{name: "invalid identity", value: "User #42", rank: 6, want: "User #6"},
		{name: "anonymous mode", value: "599155162@qq.com", rank: 7, anonymous: true, want: "Anonymous #7"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := growthLeaderboardDisplayName(test.value, test.rank, test.anonymous); got != test.want {
				t.Fatalf("growthLeaderboardDisplayName() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestSanitizeGrowthLeaderboardCoversItemsAndCurrentUser(t *testing.T) {
	result := &GrowthLeaderboard{
		Items: []GrowthLeaderboardItem{
			{Rank: 1, DisplayName: "first@example.com"},
			{Rank: 2, DisplayName: "second@example.com"},
		},
		CurrentUser: &GrowthLeaderboardItem{Rank: 2, DisplayName: "second@example.com"},
	}

	sanitizeGrowthLeaderboard(result, false)

	if result.Items[0].DisplayName != "fi***st@example.com" {
		t.Fatalf("first item was not masked: %q", result.Items[0].DisplayName)
	}
	if result.Items[1].DisplayName != "se***nd@example.com" {
		t.Fatalf("second item was not masked: %q", result.Items[1].DisplayName)
	}
	if result.CurrentUser == nil || result.CurrentUser.DisplayName != "se***nd@example.com" {
		t.Fatalf("current user was not masked: %#v", result.CurrentUser)
	}
	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "first@example.com") || strings.Contains(string(payload), "second@example.com") {
		t.Fatalf("serialized leaderboard leaked a full email: %s", payload)
	}
}
