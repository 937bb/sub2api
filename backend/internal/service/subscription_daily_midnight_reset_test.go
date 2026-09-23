package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
)

type dailyRollingResetRepo struct {
	userSubRepoNoop

	resetCalled    bool
	newWindowStart time.Time
}

func (r *dailyRollingResetRepo) ResetDailyUsage(_ context.Context, _ int64, _ *time.Time, newWindowStart time.Time) error {
	r.resetCalled = true
	r.newWindowStart = newWindowStart
	return nil
}

func dailyRollingTestBase() time.Time {
	return timezone.StartOfDay(time.Date(2026, 8, 6, 12, 0, 0, 0, timezone.Location()))
}

func newDailyRollingTestSub(dailyWindowStart time.Time, base time.Time) *UserSubscription {
	start := dailyWindowStart
	return &UserSubscription{
		ID:               1,
		UserID:           10,
		GroupID:          20,
		StartsAt:         base.AddDate(0, 0, -3),
		ExpiresAt:        base.AddDate(0, 0, 30),
		DailyUsageUSD:    43.34,
		DailyWindowStart: &start,
	}
}

func TestCheckAndResetWindows_DailyResetsAfterRolling24Hours(t *testing.T) {
	base := dailyRollingTestBase()
	windowStart := base.Add(16*time.Hour + 49*time.Minute)
	now := windowStart.Add(24 * time.Hour)

	repo := &dailyRollingResetRepo{}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	svc.now = func() time.Time { return now }
	sub := newDailyRollingTestSub(windowStart, base)

	require.NoError(t, svc.CheckAndResetWindows(context.Background(), sub))

	require.True(t, repo.resetCalled)
	require.Equal(t, now, repo.newWindowStart)
	require.Zero(t, sub.DailyUsageUSD)
	require.Equal(t, now, *sub.DailyWindowStart)
}

func TestCheckAndResetWindows_DailyNoResetBeforeRolling24Hours(t *testing.T) {
	base := dailyRollingTestBase()
	windowStart := base.Add(16*time.Hour + 49*time.Minute)
	now := windowStart.Add(24*time.Hour - time.Second)

	repo := &dailyRollingResetRepo{}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	svc.now = func() time.Time { return now }
	sub := newDailyRollingTestSub(windowStart, base)

	require.NoError(t, svc.CheckAndResetWindows(context.Background(), sub))

	require.False(t, repo.resetCalled)
	require.Equal(t, 43.34, sub.DailyUsageUSD)
}

func TestCheckAndResetWindows_AdvancesStaleRollingAnchorToLatestPeriod(t *testing.T) {
	base := dailyRollingTestBase()
	staleAnchor := base.AddDate(0, 0, -3).Add(17*time.Hour + 18*time.Minute)
	now := base.AddDate(0, 0, 3).Add(10 * time.Hour)

	repo := &dailyRollingResetRepo{}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	svc.now = func() time.Time { return now }
	sub := newDailyRollingTestSub(staleAnchor, base)

	require.NoError(t, svc.CheckAndResetWindows(context.Background(), sub))

	require.True(t, repo.resetCalled)
	require.Equal(t, staleAnchor.Add(5*24*time.Hour), repo.newWindowStart)
}

func TestNeedsDailyReset_UsesRolling24HourSchedule(t *testing.T) {
	base := dailyRollingTestBase()
	sub := newDailyRollingTestSub(base, base)

	require.False(t, sub.NeedsDailyResetAt(base.Add(24*time.Hour-time.Second)))
	require.True(t, sub.NeedsDailyResetAt(base.Add(24*time.Hour)))
}

func TestDailyResetTime_UsesRollingWindowStartFromMaintenanceSuite(t *testing.T) {
	base := dailyRollingTestBase()

	sub := newDailyRollingTestSub(base.Add(16*time.Hour+49*time.Minute), base)
	resetAt := sub.DailyResetTime()
	require.NotNil(t, resetAt)
	require.Equal(t, base.Add(16*time.Hour+49*time.Minute+24*time.Hour), *resetAt)
}

func TestNormalizeExpiredWindows_DailyUsageAdvancesAfterRollingWindow(t *testing.T) {
	base := dailyRollingTestBase()
	windowStart := base.Add(16*time.Hour + 49*time.Minute)
	now := windowStart.Add(24*time.Hour + time.Minute)

	subs := []UserSubscription{*newDailyRollingTestSub(windowStart, base)}
	normalizeExpiredWindowsAt(subs, now)

	require.Zero(t, subs[0].DailyUsageUSD)
	require.Equal(t, windowStart.Add(24*time.Hour), *subs[0].DailyWindowStart)
}

func TestCheckAndResetWindows_OneTimeDailyCardStillExemptFromAutomaticReset(t *testing.T) {
	base := dailyRollingTestBase()
	startsAt := base.Add(17 * time.Hour)
	anchor := base
	now := startsAt.Add(48 * time.Hour)

	repo := &dailyRollingResetRepo{}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	svc.now = func() time.Time { return now }
	sub := &UserSubscription{
		ID:               1,
		UserID:           10,
		GroupID:          20,
		StartsAt:         startsAt,
		ExpiresAt:        startsAt.AddDate(0, 0, 1),
		DailyUsageUSD:    10,
		DailyWindowStart: &anchor,
	}

	require.NoError(t, svc.CheckAndResetWindows(context.Background(), sub))

	require.False(t, repo.resetCalled, "日卡为一次性配额，不应自动重置")
	require.Equal(t, 10.0, sub.DailyUsageUSD)
}
