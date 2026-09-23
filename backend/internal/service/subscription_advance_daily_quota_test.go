package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
)

func advanceDailyQuotaTestService(
	t *testing.T,
	now time.Time,
	sub *UserSubscription,
	group *Group,
) (*SubscriptionService, *subscriptionUserSubRepoStub) {
	t.Helper()
	repo := newSubscriptionUserSubRepoStub()
	repo.seed(sub)
	svc := NewSubscriptionService(&subscriptionGroupRepoStub{group: group}, repo, nil, nil, nil)
	svc.now = func() time.Time { return now }
	return svc, repo
}

func TestAdvanceDailyQuota_DeductsOneDayAndStartsRollingWindow(t *testing.T) {
	now := time.Date(2026, 9, 23, 22, 30, 0, 0, time.UTC)
	dailyWindow := now.Add(-12 * time.Hour)
	dailyLimit := 10.0
	weeklyLimit := 100.0
	monthlyLimit := 300.0
	originalExpiry := now.Add(10 * 24 * time.Hour)
	sub := &UserSubscription{
		ID:                 71,
		UserID:             7,
		GroupID:            9,
		StartsAt:           now.Add(-5 * 24 * time.Hour),
		ExpiresAt:          originalExpiry,
		Status:             SubscriptionStatusActive,
		DailyWindowStart:   &dailyWindow,
		DailyUsageUSD:      dailyLimit,
		WeeklyUsageUSD:     25,
		MonthlyUsageUSD:    50,
		WeeklyWindowStart:  &dailyWindow,
		MonthlyWindowStart: &dailyWindow,
	}
	group := &Group{
		ID:               9,
		SubscriptionType: SubscriptionTypeSubscription,
		DailyLimitUSD:    &dailyLimit,
		WeeklyLimitUSD:   &weeklyLimit,
		MonthlyLimitUSD:  &monthlyLimit,
	}
	svc, _ := advanceDailyQuotaTestService(t, now, sub, group)

	updated, err := svc.AdvanceDailyQuota(context.Background(), 7, 71, "advance-71")

	require.NoError(t, err)
	require.Equal(t, originalExpiry.Add(-24*time.Hour), updated.ExpiresAt)
	require.Zero(t, updated.DailyUsageUSD)
	require.Equal(t, 25.0, updated.WeeklyUsageUSD)
	require.Equal(t, 50.0, updated.MonthlyUsageUSD)
	require.NotNil(t, updated.DailyWindowStart)
	require.Equal(t, now, *updated.DailyWindowStart)
	require.False(t, updated.NeedsDailyResetAt(now.Add(23*time.Hour+59*time.Minute)))
	require.True(t, updated.NeedsDailyResetAt(now.Add(24*time.Hour)))
}

func TestAdvanceDailyQuota_RepeatedAdvanceMovesPreconsumedWindowForward(t *testing.T) {
	now := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	futureWindow := now.Add(12 * time.Hour)
	dailyLimit := 5.0
	originalExpiry := now.Add(10 * 24 * time.Hour)
	sub := &UserSubscription{
		ID:               72,
		UserID:           7,
		GroupID:          9,
		StartsAt:         now.Add(-5 * 24 * time.Hour),
		ExpiresAt:        originalExpiry,
		Status:           SubscriptionStatusActive,
		DailyWindowStart: &futureWindow,
		DailyUsageUSD:    dailyLimit,
	}
	group := &Group{ID: 9, SubscriptionType: SubscriptionTypeSubscription, DailyLimitUSD: &dailyLimit}
	svc, _ := advanceDailyQuotaTestService(t, now, sub, group)

	updated, err := svc.AdvanceDailyQuota(context.Background(), 7, 72, "advance-72")

	require.NoError(t, err)
	require.Equal(t, originalExpiry.Add(-24*time.Hour), updated.ExpiresAt)
	require.Equal(t, now, *updated.DailyWindowStart)
}

func TestAdvanceDailyQuota_ReplaysLedgerWithoutSecondDeduction(t *testing.T) {
	now := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	dailyWindow := now.Add(-12 * time.Hour)
	dailyLimit := 10.0
	originalExpiry := now.Add(10 * 24 * time.Hour)
	sub := &UserSubscription{
		ID:               73,
		UserID:           7,
		GroupID:          9,
		StartsAt:         now.Add(-5 * 24 * time.Hour),
		ExpiresAt:        originalExpiry,
		Status:           SubscriptionStatusActive,
		DailyWindowStart: &dailyWindow,
		DailyUsageUSD:    dailyLimit,
	}
	group := &Group{ID: 9, SubscriptionType: SubscriptionTypeSubscription, DailyLimitUSD: &dailyLimit}
	svc, repo := advanceDailyQuotaTestService(t, now, sub, group)

	first, err := svc.AdvanceDailyQuota(context.Background(), 7, sub.ID, "same-operation")
	require.NoError(t, err)
	require.Equal(t, originalExpiry.Add(-24*time.Hour), first.ExpiresAt)
	require.Equal(t, 1, repo.updateCalls)

	// A later usage settlement must not alter the immutable replay response.
	repo.byID[sub.ID].DailyUsageUSD = 3
	second, err := svc.AdvanceDailyQuota(context.Background(), 7, sub.ID, "same-operation")
	require.NoError(t, err)
	require.Equal(t, first.ExpiresAt, second.ExpiresAt)
	require.Equal(t, first.DailyWindowStart, second.DailyWindowStart)
	require.Zero(t, second.DailyUsageUSD)
	require.Equal(t, 3.0, repo.byID[sub.ID].DailyUsageUSD)
	require.Equal(t, 1, repo.updateCalls, "replay must not mutate the subscription")
}

func TestAdvanceDailyQuota_RecoversAfterCoordinatorMarkSucceededFailure(t *testing.T) {
	now := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	dailyWindow := now.Add(-12 * time.Hour)
	dailyLimit := 10.0
	originalExpiry := now.Add(10 * 24 * time.Hour)
	sub := &UserSubscription{
		ID:               74,
		UserID:           7,
		GroupID:          9,
		StartsAt:         now.Add(-5 * 24 * time.Hour),
		ExpiresAt:        originalExpiry,
		Status:           SubscriptionStatusActive,
		DailyWindowStart: &dailyWindow,
		DailyUsageUSD:    dailyLimit,
	}
	group := &Group{ID: 9, SubscriptionType: SubscriptionTypeSubscription, DailyLimitUSD: &dailyLimit}
	svc, subRepo := advanceDailyQuotaTestService(t, now, sub, group)
	idempotencyRepo := &markBehaviorRepo{
		inMemoryIdempotencyRepo: *newInMemoryIdempotencyRepo(),
		failMarkSucceeded:       true,
	}
	cfg := DefaultIdempotencyConfig()
	cfg.ObserveOnly = false
	cfg.ProcessingTimeout = -time.Second
	coordinator := NewIdempotencyCoordinator(idempotencyRepo, cfg)
	opts := IdempotencyExecuteOptions{
		Scope:          "user.subscriptions.advance_daily_quota",
		ActorScope:     "user:7",
		Method:         "POST",
		Route:          "/api/v1/subscriptions/:id/advance-daily-quota",
		IdempotencyKey: "mark-failure-recovery",
		Payload:        map[string]any{"subscription_id": sub.ID},
		RequireKey:     true,
	}
	execute := func(ctx context.Context) (any, error) {
		return svc.AdvanceDailyQuota(ctx, 7, sub.ID, opts.IdempotencyKey)
	}

	_, err := coordinator.Execute(context.Background(), opts, execute)
	require.ErrorIs(t, err, ErrIdempotencyStoreUnavail)
	require.Equal(t, 1, subRepo.updateCalls)
	require.Equal(t, originalExpiry.Add(-24*time.Hour), subRepo.byID[sub.ID].ExpiresAt)

	recovered, err := svc.RecoverDailyQuotaAdvance(context.Background(), 7, sub.ID, opts.IdempotencyKey)
	require.NoError(t, err)
	require.NotNil(t, recovered)
	require.Equal(t, originalExpiry.Add(-24*time.Hour), recovered.ExpiresAt)

	require.Equal(t, 1, subRepo.updateCalls, "ledger recovery must not repeat the subscription mutation")
	require.Equal(t, originalExpiry.Add(-24*time.Hour), subRepo.byID[sub.ID].ExpiresAt)
}

func TestAdvanceDailyQuota_RequiresIdempotencyKey(t *testing.T) {
	now := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	dailyWindow := now.Add(-12 * time.Hour)
	dailyLimit := 10.0
	sub := &UserSubscription{
		ID: 75, UserID: 7, GroupID: 9,
		StartsAt: now.Add(-5 * 24 * time.Hour), ExpiresAt: now.Add(10 * 24 * time.Hour),
		Status: SubscriptionStatusActive, DailyWindowStart: &dailyWindow, DailyUsageUSD: dailyLimit,
	}
	svc, repo := advanceDailyQuotaTestService(t, now, sub, &Group{ID: 9, DailyLimitUSD: &dailyLimit})

	_, err := svc.AdvanceDailyQuota(context.Background(), 7, sub.ID, "")

	require.ErrorIs(t, err, ErrIdempotencyKeyRequired)
	require.Zero(t, repo.updateCalls)
}

func TestAdvanceDailyQuota_RejectsInvalidExchange(t *testing.T) {
	now := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	today := timezone.StartOfDay(now)
	yesterday := today.AddDate(0, 0, -1)
	dailyLimit := 10.0
	weeklyLimit := 20.0
	monthlyLimit := 30.0

	tests := []struct {
		name   string
		userID int64
		sub    UserSubscription
		group  Group
		want   error
	}{
		{
			name:   "another user cannot mutate subscription",
			userID: 8,
			sub:    UserSubscription{UserID: 7, StartsAt: now.Add(-48 * time.Hour), ExpiresAt: now.Add(48 * time.Hour), Status: SubscriptionStatusActive, DailyWindowStart: &today, DailyUsageUSD: 10},
			group:  Group{DailyLimitUSD: &dailyLimit},
			want:   ErrSubscriptionNotFound,
		},
		{
			name:   "daily quota is not exhausted",
			userID: 7,
			sub:    UserSubscription{UserID: 7, StartsAt: now.Add(-48 * time.Hour), ExpiresAt: now.Add(48 * time.Hour), Status: SubscriptionStatusActive, DailyWindowStart: &today, DailyUsageUSD: 9.99},
			group:  Group{DailyLimitUSD: &dailyLimit},
			want:   ErrDailyQuotaNotExhausted,
		},
		{
			name:   "one-day card cannot exchange its only quota",
			userID: 7,
			sub:    UserSubscription{UserID: 7, StartsAt: now.Add(-time.Hour), ExpiresAt: now.Add(23 * time.Hour), Status: SubscriptionStatusActive, DailyWindowStart: &today, DailyUsageUSD: 10},
			group:  Group{DailyLimitUSD: &dailyLimit},
			want:   ErrDailyQuotaAdvanceOneTime,
		},
		{
			name:   "remaining term must exceed 24 hours",
			userID: 7,
			sub:    UserSubscription{UserID: 7, StartsAt: now.Add(-5 * 24 * time.Hour), ExpiresAt: now.Add(24 * time.Hour), Status: SubscriptionStatusActive, DailyWindowStart: &today, DailyUsageUSD: 10},
			group:  Group{DailyLimitUSD: &dailyLimit},
			want:   ErrDailyQuotaAdvanceTerm,
		},
		{
			name:   "stale daily window gets a free automatic reset",
			userID: 7,
			sub:    UserSubscription{UserID: 7, StartsAt: now.Add(-5 * 24 * time.Hour), ExpiresAt: now.Add(48 * time.Hour), Status: SubscriptionStatusActive, DailyWindowStart: &yesterday, DailyUsageUSD: 10},
			group:  Group{DailyLimitUSD: &dailyLimit},
			want:   ErrDailyQuotaAlreadyReset,
		},
		{
			name:   "weekly exhaustion blocks a useless exchange",
			userID: 7,
			sub:    UserSubscription{UserID: 7, StartsAt: now.Add(-5 * 24 * time.Hour), ExpiresAt: now.Add(48 * time.Hour), Status: SubscriptionStatusActive, DailyWindowStart: &today, DailyUsageUSD: 10, WeeklyUsageUSD: 20},
			group:  Group{DailyLimitUSD: &dailyLimit, WeeklyLimitUSD: &weeklyLimit},
			want:   ErrDailyQuotaAdvanceWeekly,
		},
		{
			name:   "weekly remainder must cover a complete daily quota",
			userID: 7,
			sub:    UserSubscription{UserID: 7, StartsAt: now.Add(-5 * 24 * time.Hour), ExpiresAt: now.Add(48 * time.Hour), Status: SubscriptionStatusActive, DailyWindowStart: &today, DailyUsageUSD: 10, WeeklyUsageUSD: 15},
			group:  Group{DailyLimitUSD: &dailyLimit, WeeklyLimitUSD: &weeklyLimit},
			want:   ErrDailyQuotaAdvanceWeekly,
		},
		{
			name:   "monthly remainder must cover a complete daily quota",
			userID: 7,
			sub:    UserSubscription{UserID: 7, StartsAt: now.Add(-5 * 24 * time.Hour), ExpiresAt: now.Add(48 * time.Hour), Status: SubscriptionStatusActive, DailyWindowStart: &today, DailyUsageUSD: 10, MonthlyUsageUSD: 25},
			group:  Group{DailyLimitUSD: &dailyLimit, MonthlyLimitUSD: &monthlyLimit},
			want:   ErrDailyQuotaAdvanceMonthly,
		},
	}

	for index := range tests {
		tt := tests[index]
		t.Run(tt.name, func(t *testing.T) {
			tt.sub.ID = 80 + int64(index)
			tt.sub.GroupID = 9
			tt.group.ID = 9
			tt.group.SubscriptionType = SubscriptionTypeSubscription
			svc, _ := advanceDailyQuotaTestService(t, now, &tt.sub, &tt.group)
			beforeExpiry := tt.sub.ExpiresAt

			_, err := svc.AdvanceDailyQuota(context.Background(), tt.userID, tt.sub.ID, "invalid-case")

			require.ErrorIs(t, err, tt.want)
			stored, getErr := svc.userSubRepo.GetByID(context.Background(), tt.sub.ID)
			require.NoError(t, getErr)
			require.Equal(t, beforeExpiry, stored.ExpiresAt)
			require.Equal(t, tt.sub.DailyUsageUSD, stored.DailyUsageUSD)
		})
	}
}

func TestGetDailyQuotaAdvancePreview_ReportsPeriodCapacityAndResetTimes(t *testing.T) {
	now := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	dailyWindow := now.Add(-12 * time.Hour)
	weeklyWindow := now.Add(-2 * 24 * time.Hour)
	monthlyWindow := now.Add(-10 * 24 * time.Hour)
	dailyLimit := 10.0
	weeklyLimit := 20.0
	monthlyLimit := 100.0
	sub := &UserSubscription{
		ID:                 91,
		UserID:             7,
		GroupID:            9,
		StartsAt:           now.Add(-10 * 24 * time.Hour),
		ExpiresAt:          now.Add(40 * 24 * time.Hour),
		Status:             SubscriptionStatusActive,
		DailyWindowStart:   &dailyWindow,
		WeeklyWindowStart:  &weeklyWindow,
		MonthlyWindowStart: &monthlyWindow,
		DailyUsageUSD:      10,
		WeeklyUsageUSD:     15,
		MonthlyUsageUSD:    30,
	}
	group := &Group{ID: 9, DailyLimitUSD: &dailyLimit, WeeklyLimitUSD: &weeklyLimit, MonthlyLimitUSD: &monthlyLimit}
	svc, repo := advanceDailyQuotaTestService(t, now, sub, group)

	preview, err := svc.GetDailyQuotaAdvancePreview(context.Background(), 7, sub.ID)

	require.NoError(t, err)
	require.False(t, preview.CanAdvance)
	require.Equal(t, 5.0, *preview.WeeklyRemainingUSD)
	require.Equal(t, 70.0, *preview.MonthlyRemainingUSD)
	require.Equal(t, 5.0, preview.RecoverableUSD)
	require.Equal(t, []string{dailyQuotaAdvanceBlockWeeklyInsufficient}, preview.Blockers)
	require.Equal(t, weeklyWindow.Add(7*24*time.Hour), *preview.WeeklyResetsAt)
	require.Equal(t, monthlyWindow.Add(30*24*time.Hour), *preview.MonthlyResetsAt)
	require.Equal(t, sub.ExpiresAt.Add(-24*time.Hour), preview.ExpiresAtAfter)

	stored, getErr := repo.GetByID(context.Background(), sub.ID)
	require.NoError(t, getErr)
	require.Equal(t, 15.0, stored.WeeklyUsageUSD)
	require.Equal(t, sub.ExpiresAt, stored.ExpiresAt)
}

func TestAdvanceDailyQuota_UsesFreshPeriodAfterStoredWindowExpires(t *testing.T) {
	now := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	dailyWindow := now.Add(-12 * time.Hour)
	weeklyWindow := now.Add(-8 * 24 * time.Hour)
	dailyLimit := 10.0
	weeklyLimit := 20.0
	sub := &UserSubscription{
		ID:                92,
		UserID:            7,
		GroupID:           9,
		StartsAt:          now.Add(-15 * 24 * time.Hour),
		ExpiresAt:         now.Add(10 * 24 * time.Hour),
		Status:            SubscriptionStatusActive,
		DailyWindowStart:  &dailyWindow,
		WeeklyWindowStart: &weeklyWindow,
		DailyUsageUSD:     10,
		WeeklyUsageUSD:    20,
	}
	group := &Group{ID: 9, DailyLimitUSD: &dailyLimit, WeeklyLimitUSD: &weeklyLimit}
	svc, _ := advanceDailyQuotaTestService(t, now, sub, group)

	updated, err := svc.AdvanceDailyQuota(context.Background(), 7, sub.ID, "advance-fresh-period")

	require.NoError(t, err)
	require.Zero(t, updated.WeeklyUsageUSD)
	require.Equal(t, weeklyWindow.Add(7*24*time.Hour), *updated.WeeklyWindowStart)
	require.Zero(t, updated.DailyUsageUSD)
}
