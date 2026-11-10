package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

func TestDashboardCacheCancellationDoesNotCancelSharedRefresh(t *testing.T) {
	var cache apiKeyDashboardCache
	var calls atomic.Int64
	started, release := make(chan struct{}), make(chan struct{})
	var cancelled atomic.Bool
	fetch := func(ctx context.Context, id int64) (*usagestats.UserDashboardStats, error) {
		calls.Add(1)
		close(started)
		<-release
		cancelled.Store(ctx.Err() != nil)
		return &usagestats.UserDashboardStats{TotalRequests: 42, ByPlatform: []usagestats.PlatformDashboardStats{{Platform: "openai"}}}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := cache.get(ctx, 7, fetch); done <- err }()
	<-started
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	second := make(chan *usagestats.UserDashboardStats, 1)
	go func() { stats, _ := cache.get(context.Background(), 7, fetch); second <- stats }()
	close(release)
	stats := <-second
	require.False(t, cancelled.Load())
	require.Equal(t, int64(42), stats.TotalRequests)
	stats.TotalRequests = 999
	stats.ByPlatform[0].Platform = "modified"
	again, err := cache.get(context.Background(), 7, fetch)
	require.NoError(t, err)
	require.Equal(t, int64(42), again.TotalRequests)
	require.Equal(t, "openai", again.ByPlatform[0].Platform)
	require.Equal(t, int64(1), calls.Load())
}

func TestDashboardCacheIsolationExpiryAndErrors(t *testing.T) {
	var cache apiKeyDashboardCache
	var calls int64
	fetch := func(_ context.Context, id int64) (*usagestats.UserDashboardStats, error) {
		calls++
		return &usagestats.UserDashboardStats{TotalRequests: id}, nil
	}
	for _, id := range []int64{1, 2, 1} {
		stats, err := cache.get(context.Background(), id, fetch)
		require.NoError(t, err)
		require.Equal(t, id, stats.TotalRequests)
	}
	require.Equal(t, int64(2), calls)
	cache.mu.Lock()
	entry := cache.entries[1]
	entry.expires = time.Now().Add(-time.Second)
	cache.entries[1] = entry
	cache.mu.Unlock()
	_, err := cache.get(context.Background(), 1, fetch)
	require.NoError(t, err)
	require.Equal(t, int64(3), calls)
	cache.mu.Lock()
	entry = cache.entries[1]
	entry.day--
	cache.entries[1] = entry
	cache.mu.Unlock()
	_, err = cache.get(context.Background(), 1, fetch)
	require.NoError(t, err)
	require.Equal(t, int64(4), calls)
	fail := errors.New("query failed")
	stats, err := cache.get(context.Background(), 3, func(context.Context, int64) (*usagestats.UserDashboardStats, error) { return nil, fail })
	require.ErrorIs(t, err, fail)
	require.Nil(t, stats)
	stats, err = cache.get(context.Background(), 3, fetch)
	require.NoError(t, err)
	require.Equal(t, int64(3), stats.TotalRequests)
}

func TestDashboardCacheBoundsRefreshesAndStorage(t *testing.T) {
	var cache apiKeyDashboardCache
	started, release, done := make(chan struct{}, 2), make(chan struct{}), make(chan struct{}, 2)
	fetch := func(context.Context, int64) (*usagestats.UserDashboardStats, error) {
		started <- struct{}{}
		<-release
		return &usagestats.UserDashboardStats{}, nil
	}
	for _, id := range []int64{1, 2} {
		go func() { _, _ = cache.get(context.Background(), id, fetch); done <- struct{}{} }()
	}
	<-started
	<-started
	_, err := cache.get(context.Background(), 3, fetch)
	require.ErrorIs(t, err, errDashboardBusy)
	close(release)
	<-done
	<-done
	cache.mu.Lock()
	for id := int64(1); id <= apiKeyDashboardMaxEntries; id++ {
		cache.entries[id] = dashboardEntry{stats: &usagestats.UserDashboardStats{}, expires: time.Now().Add(time.Hour), day: timezone.Today().Unix()}
	}
	cache.mu.Unlock()
	_, err = cache.get(context.Background(), 9999, func(context.Context, int64) (*usagestats.UserDashboardStats, error) {
		return &usagestats.UserDashboardStats{}, nil
	})
	require.NoError(t, err)
	cache.mu.Lock()
	defer cache.mu.Unlock()
	require.Len(t, cache.entries, apiKeyDashboardMaxEntries)
}
