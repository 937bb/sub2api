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

func TestGrowthRepositoryLeaderboardUsesFullEmail(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	start := time.Date(2026, time.July, 20, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	mock.ExpectQuery(regexp.QuoteMeta("NULLIF(BTRIM(u.email), '')")).
		WithArgs(start, end, 50, int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "display_name", "actual_cost", "requests", "rank"}).
			AddRow(1, "w97bb@example.com", 12.4133, 471, 1))

	result, err := NewGrowthRepository(db).GetLeaderboard(context.Background(), start, end, 1, 50)

	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Equal(t, "w97bb@example.com", result.Items[0].DisplayName)
	require.NotNil(t, result.CurrentUser)
	require.Equal(t, "w97bb@example.com", result.CurrentUser.DisplayName)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGrowthCheckinEligibilityUsesRecentActualSpendOnly(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	since := time.Date(2026, time.June, 20, 6, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta(growthHasRecentSpendSQL)).
		WithArgs(int64(42), since).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	eligible, err := hasGrowthRecentSpend(context.Background(), db, 42, since)
	require.NoError(t, err)
	require.True(t, eligible)
	require.NoError(t, mock.ExpectationsWereMet())

	require.Contains(t, growthHasRecentSpendSQL, "FROM usage_logs")
	require.Contains(t, growthHasRecentSpendSQL, "created_at >= $2")
	require.Contains(t, growthHasRecentSpendSQL, "actual_cost > 0")
	require.NotContains(t, growthHasRecentSpendSQL, "payment_orders")
	require.NotContains(t, growthHasRecentSpendSQL, "redeem_codes")
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

	result, err := NewGrowthRepository(db).GetLeaderboard(context.Background(), start, end, 75, 10)

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
		name        string
		periodSpend float64
		reward      float64
		want        bool
	}{
		{name: "reward within period spend", periodSpend: 20, reward: 10, want: true},
		{name: "reward equals period spend", periodSpend: 5, reward: 5, want: true},
		{name: "reward exceeds period spend", periodSpend: 4, reward: 5, want: false},
		{name: "zero reward", periodSpend: 20, reward: 0, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, growthLeaderboardRewardAllowed(test.periodSpend, test.reward))
		})
	}
}

func TestGrowthClaimDeniedErrorReturnsSpecificPublicReason(t *testing.T) {
	tests := map[string]string{
		"account_inactive":        "GROWTH_ACCOUNT_INACTIVE",
		"account_too_new":         "GROWTH_ACCOUNT_TOO_NEW",
		"recent_spend_too_low":    "GROWTH_RECENT_SPEND_TOO_LOW",
		"ip_account_limit":        "GROWTH_IDENTITY_RISK",
		"device_account_limit":    "GROWTH_IDENTITY_RISK",
		"missing_identity_signal": "GROWTH_IDENTITY_RISK",
		"unknown":                 "GROWTH_REWARD_INELIGIBLE",
	}
	for reason, expected := range tests {
		t.Run(reason, func(t *testing.T) {
			require.Equal(t, expected, infraerrors.Reason(growthClaimDeniedError(reason)))
		})
	}
}
