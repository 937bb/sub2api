package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestLiveLeaseReplacesRegularSlotsAndCountsTowardLimits(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	regular := NewConcurrencyCache(client, 15, 900)
	live, ok := regular.(service.LiveConcurrencyCache)
	require.True(t, ok)
	ctx := context.Background()

	accountAcquired, err := regular.AcquireAccountSlot(ctx, 10, 1, "regular-account")
	require.NoError(t, err)
	require.True(t, accountAcquired)
	userAcquired, err := regular.AcquireUserSlot(ctx, 20, 1, "regular-user")
	require.NoError(t, err)
	require.True(t, userAcquired)

	acquired, err := live.AcquireLiveLease(ctx, 10, 1, 20, 1, 30, "live-lease", true)
	require.NoError(t, err)
	require.True(t, acquired)
	require.NoError(t, regular.ReleaseAccountSlot(ctx, 10, "regular-account"))
	require.NoError(t, regular.ReleaseUserSlot(ctx, 20, "regular-user"))

	accountCount, err := regular.GetAccountConcurrency(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, 1, accountCount)
	userCount, err := regular.GetUserConcurrency(ctx, 20)
	require.NoError(t, err)
	require.Equal(t, 1, userCount)
	accountAcquired, err = regular.AcquireAccountSlot(ctx, 10, 1, "ordinary-blocked")
	require.NoError(t, err)
	require.False(t, accountAcquired)

	refreshed, err := live.RefreshLiveLease(ctx, 10, 20, 30, "live-lease")
	require.NoError(t, err)
	require.True(t, refreshed)
	require.NoError(t, live.ReleaseLiveLease(ctx, 10, 20, 30, "live-lease"))
	accountAcquired, err = regular.AcquireAccountSlot(ctx, 10, 1, "ordinary-allowed")
	require.NoError(t, err)
	require.True(t, accountAcquired)
}

func TestLiveLeaseExpiresWithoutRefresh(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	regular := NewConcurrencyCache(client, 15, 900)
	live, ok := regular.(service.LiveConcurrencyCache)
	require.True(t, ok)
	ctx := context.Background()

	acquired, err := live.AcquireLiveLease(ctx, 10, 1, 20, 1, 30, "expired-live", false)
	require.NoError(t, err)
	require.True(t, acquired)

	redisServer.FastForward(61 * time.Second)
	acquired, err = regular.AcquireAccountSlot(ctx, 10, 1, "ordinary-after-expiry")
	require.NoError(t, err)
	require.True(t, acquired)
	refreshed, err := live.RefreshLiveLease(ctx, 10, 20, 30, "expired-live")
	require.NoError(t, err)
	require.False(t, refreshed)
}

func TestQuotaBypassActiveAccountIsSharedAndOnlyAdvancesByCompareAndSet(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	routing, ok := NewConcurrencyCache(client, 15, 900).(service.QuotaBypassRoutingCache)
	require.True(t, ok)
	ctx := context.Background()
	const poolKey = "group:42:openai:0"

	require.NoError(t, routing.SetQuotaBypassActiveAccount(ctx, poolKey, 20, 15*time.Minute))
	accountID, err := routing.GetQuotaBypassActiveAccount(ctx, poolKey)
	require.NoError(t, err)
	require.Equal(t, int64(20), accountID)

	// A full target can explicitly advance to the next account.
	require.NoError(t, routing.SetQuotaBypassActiveAccount(ctx, poolKey, 30, 15*time.Minute))
	activeID, err := routing.AdvanceQuotaBypassActiveAccount(ctx, poolKey, 20, 40, 15*time.Minute)
	require.NoError(t, err)
	require.Equal(t, int64(30), activeID, "stale observer must follow the newer active account")
	activeID, err = routing.AdvanceQuotaBypassActiveAccount(ctx, poolKey, 30, 40, 15*time.Minute)
	require.NoError(t, err)
	require.Equal(t, int64(40), activeID)
	accountID, err = routing.GetQuotaBypassActiveAccount(ctx, poolKey)
	require.NoError(t, err)
	require.Equal(t, int64(40), accountID)

	redisServer.FastForward(16 * time.Minute)
	accountID, err = routing.GetQuotaBypassActiveAccount(ctx, poolKey)
	require.NoError(t, err)
	require.Zero(t, accountID)
}
