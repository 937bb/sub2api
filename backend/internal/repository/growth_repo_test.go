package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
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
		name           string
		periodSpend    float64
		totalRecharged float64
		lifetimeReward float64
		reward         float64
		ratio          float64
		want           bool
		wantMaximum    float64
	}{
		{name: "valid paid spend", periodSpend: 20, totalRecharged: 100, lifetimeReward: 5, reward: 10, ratio: 0.5, want: true, wantMaximum: 50},
		{name: "free balance spend", periodSpend: 20, totalRecharged: 0, reward: 5, ratio: 1, want: false, wantMaximum: 0},
		{name: "reward exceeds spend", periodSpend: 4, totalRecharged: 100, reward: 5, ratio: 1, want: false, wantMaximum: 100},
		{name: "lifetime paid cap", periodSpend: 20, totalRecharged: 10, lifetimeReward: 8, reward: 3, ratio: 1, want: false, wantMaximum: 10},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			allowed, maximum := growthLeaderboardRewardAllowed(
				test.periodSpend, test.totalRecharged, test.lifetimeReward,
				test.reward, test.ratio,
			)
			require.Equal(t, test.want, allowed)
			require.Equal(t, test.wantMaximum, maximum)
		})
	}
}
