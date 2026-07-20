package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestGrowthRepositoryLeaderboardFallsBackToEmailPrefix(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	start := time.Date(2026, time.July, 20, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	mock.ExpectQuery(regexp.QuoteMeta("NULLIF(SPLIT_PART(BTRIM(u.email), '@', 1), '')")).
		WithArgs(start, end, 50, int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "display_name", "actual_cost", "requests", "rank"}).
			AddRow(1, "w97bb", 12.4133, 471, 1))

	result, err := NewGrowthRepository(db).GetLeaderboard(context.Background(), start, end, 1, 50, false)

	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Equal(t, "w97bb", result.Items[0].DisplayName)
	require.NotNil(t, result.CurrentUser)
	require.Equal(t, "w97bb", result.CurrentUser.DisplayName)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGrowthEligibleFundingIncludesNetAdminBalanceAdjustments(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectQuery(regexp.QuoteMeta("AND rc.type = 'admin_balance'")).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"eligible_funding"}).AddRow(75.5))

	amount, err := getGrowthEligibleFunding(context.Background(), db, 42)

	require.NoError(t, err)
	require.Equal(t, 75.5, amount)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Contains(t, growthEligibleFundingSQL, "SELECT SUM(rc.value)")
	require.NotContains(t, growthEligibleFundingSQL, "rc.value > 0")
	require.NotContains(t, growthEligibleFundingSQL, "type = 'balance'")
}

func TestGrowthRepositoryLeaderboardCapsVisibleRowsButKeepsCurrentUser(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	start := time.Date(2026, time.July, 20, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	mock.ExpectQuery(regexp.QuoteMeta("FROM ranked WHERE rank <= $3 OR user_id = $4")).
		WithArgs(start, end, 10, int64(75)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "display_name", "actual_cost", "requests", "rank"}).
			AddRow(1, "first", 100.0, 1000, 1).
			AddRow(75, "current", 5.0, 20, 75))

	result, err := NewGrowthRepository(db).GetLeaderboard(context.Background(), start, end, 75, 10, false)

	require.NoError(t, err)
	require.Equal(t, int64(1), result.Total)
	require.Equal(t, 1, result.Pages)
	require.Equal(t, 10, result.PageSize)
	require.Len(t, result.Items, 1)
	require.NotNil(t, result.CurrentUser)
	require.Equal(t, 75, result.CurrentUser.Rank)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGrowthLeaderboardRewardAllowed(t *testing.T) {
	tests := []struct {
		name            string
		periodSpend     float64
		eligibleFunding float64
		lifetimeReward  float64
		reward          float64
		ratio           float64
		want            bool
		wantMaximum     float64
	}{
		{name: "valid funded spend", periodSpend: 20, eligibleFunding: 100, lifetimeReward: 5, reward: 10, ratio: 0.5, want: true, wantMaximum: 50},
		{name: "unfunded spend", periodSpend: 20, eligibleFunding: 0, reward: 5, ratio: 1, want: false, wantMaximum: 0},
		{name: "reward exceeds spend", periodSpend: 4, eligibleFunding: 100, reward: 5, ratio: 1, want: false, wantMaximum: 100},
		{name: "lifetime funding cap", periodSpend: 20, eligibleFunding: 10, lifetimeReward: 8, reward: 3, ratio: 1, want: false, wantMaximum: 10},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			allowed, maximum := growthLeaderboardRewardAllowed(
				test.periodSpend, test.eligibleFunding, test.lifetimeReward,
				test.reward, test.ratio,
			)
			require.Equal(t, test.want, allowed)
			require.Equal(t, test.wantMaximum, maximum)
		})
	}
}

func TestGrowthClaimDeniedErrorReturnsSpecificPublicReason(t *testing.T) {
	tests := map[string]string{
		"account_inactive":        "GROWTH_ACCOUNT_INACTIVE",
		"account_too_new":         "GROWTH_ACCOUNT_TOO_NEW",
		"total_recharged_too_low": "GROWTH_RECHARGE_TOO_LOW",
		"recent_spend_too_low":    "GROWTH_RECENT_SPEND_TOO_LOW",
		"ip_account_limit":        "GROWTH_IDENTITY_RISK",
		"device_account_limit":    "GROWTH_IDENTITY_RISK",
		"missing_identity_signal": "GROWTH_IDENTITY_RISK",
		"lifetime_reward_cap":     "GROWTH_CHECKIN_REWARD_CAP",
		"total_growth_reward_cap": "GROWTH_TOTAL_REWARD_CAP",
		"unknown":                 "GROWTH_REWARD_INELIGIBLE",
	}
	for reason, expected := range tests {
		t.Run(reason, func(t *testing.T) {
			require.Equal(t, expected, infraerrors.Reason(growthClaimDeniedError(reason)))
		})
	}
}
