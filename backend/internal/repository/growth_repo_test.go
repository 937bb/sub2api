package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
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

func TestGrowthRiskOverrideStatus(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT status FROM growth_risk_account_states WHERE user_id = $1 FOR UPDATE`)).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("whitelisted"))

	status, err := getGrowthRiskStatus(context.Background(), db, 42)
	require.NoError(t, err)
	require.Equal(t, "whitelisted", status)
	require.True(t, growthRiskBypassesIdentity(status))
	require.True(t, growthRiskBypassesIdentity("cleared_once"))
	require.False(t, growthRiskBypassesIdentity("flagged"))
	require.True(t, growthRiskClearsAfterCheckin("flagged"))
	require.True(t, growthRiskClearsAfterCheckin("cleared_once"))
	require.False(t, growthRiskClearsAfterCheckin("whitelisted"))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGrowthRepositoryUpdatesRiskAccountAction(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectExec(regexp.QuoteMeta("UPDATE growth_risk_account_states")).
		WithArgs("cleared_once", "verified customer", int64(9), int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = NewGrowthRepository(db).UpdateRiskAccount(context.Background(), 42, "clear", "verified customer", 9)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGrowthRepositoryListsActiveRiskAccounts(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM growth_risk_account_states").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT s.user_id").
		WithArgs(20, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"user_id", "email", "status", "reason_code", "event_count", "first_flagged_at", "last_flagged_at", "action_note", "action_by_email", "action_at",
		}).AddRow(
			int64(42), "flagged@example.com", "flagged", "device_account_limit", 3,
			time.Date(2026, 7, 19, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC), "", "", nil,
		))

	items, total, err := NewGrowthRepository(db).ListRiskAccounts(context.Background(), 1, 20)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	require.Equal(t, "flagged@example.com", items[0].Email)
	require.Equal(t, "flagged", items[0].Status)
	require.Nil(t, items[0].ActionAt)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGrowthRepositoryFlagsIdentityRiskAccount(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	repo := &growthRepository{db: db}

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO growth_risk_events")).
		WithArgs(int64(42), "denied", "device_account_limit", "ip-hash", "device-hash", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO growth_risk_account_states")).
		WithArgs(int64(42), "device_account_limit").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	repo.recordRiskEvent(context.Background(), service.GrowthCheckinClaim{
		UserID: 42, IPHash: "ip-hash", DeviceHash: "device-hash",
	}, "denied", "device_account_limit", map[string]any{"limit": 1})

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
