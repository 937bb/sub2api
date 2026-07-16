package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type opsMetricsProjectionRepo struct {
	AccountRepository
	loads           []AccountWithConcurrency
	fullListCalls   int
	projectionCalls int
}

func (r *opsMetricsProjectionRepo) ListSchedulable(context.Context) ([]Account, error) {
	r.fullListCalls++
	return nil, nil
}

func (r *opsMetricsProjectionRepo) ListSchedulableAccountLoads(context.Context) ([]AccountWithConcurrency, error) {
	r.projectionCalls++
	return r.loads, nil
}

type opsMetricsLoadCache struct {
	ConcurrencyCache
	got []AccountWithConcurrency
}

func (c *opsMetricsLoadCache) GetAccountsLoadBatch(_ context.Context, accounts []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error) {
	c.got = accounts
	return map[int64]*AccountLoadInfo{11: {AccountID: 11, WaitingCount: 2}}, nil
}

func TestCollectConcurrencyQueueDepthUsesProjection(t *testing.T) {
	want := []AccountWithConcurrency{{ID: 11, MaxConcurrency: 7}}
	repo := &opsMetricsProjectionRepo{loads: want}
	cache := &opsMetricsLoadCache{}
	concurrency := NewConcurrencyService(cache)
	concurrency.SetAccountLoadBatchCacheTTL(0)
	collector := NewOpsMetricsCollector(nil, nil, repo, concurrency, nil, nil, nil)

	depth := collector.collectConcurrencyQueueDepth(context.Background())

	require.NotNil(t, depth)
	require.Equal(t, 2, *depth)
	require.Equal(t, 1, repo.projectionCalls)
	require.Zero(t, repo.fullListCalls)
	require.Equal(t, want, cache.got)
}

type legacyOpsMetricsAccountRepo struct {
	AccountRepository
}

func (r *legacyOpsMetricsAccountRepo) ListSchedulable(context.Context) ([]Account, error) {
	return []Account{{ID: 11, Concurrency: 2, LoadFactor: intPtr(7)}}, nil
}

func TestCollectConcurrencyQueueDepthUsesProjectionForDirectConstruction(t *testing.T) {
	want := []AccountWithConcurrency{{ID: 11, MaxConcurrency: 7}}
	repo := &opsMetricsProjectionRepo{loads: want}
	cache := &opsMetricsLoadCache{}
	collector := &OpsMetricsCollector{
		accountRepo:        repo,
		concurrencyService: NewConcurrencyService(cache),
	}

	depth := collector.collectConcurrencyQueueDepth(context.Background())

	require.NotNil(t, depth)
	require.Equal(t, 2, *depth)
	require.Equal(t, 1, repo.projectionCalls)
	require.Zero(t, repo.fullListCalls)
	require.Equal(t, want, cache.got)
}

func TestCollectConcurrencyQueueDepthFallsBackForLegacyRepository(t *testing.T) {
	repo := &legacyOpsMetricsAccountRepo{}
	cache := &opsMetricsLoadCache{}
	collector := &OpsMetricsCollector{
		accountRepo:        repo,
		concurrencyService: NewConcurrencyService(cache),
	}

	depth := collector.collectConcurrencyQueueDepth(context.Background())

	require.NotNil(t, depth)
	require.Equal(t, 2, *depth)
	require.Equal(t, []AccountWithConcurrency{{ID: 11, MaxConcurrency: 7}}, cache.got)
}
