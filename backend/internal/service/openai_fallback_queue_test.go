package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fallbackQueueLoadCache struct {
	stubConcurrencyCache
	loads []map[int64]*AccountLoadInfo
	reads int
}

func (c *fallbackQueueLoadCache) GetAccountsLoadBatch(ctx context.Context, accounts []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error) {
	i := c.reads
	c.reads++
	if i >= len(c.loads) {
		i = len(c.loads) - 1
	}
	return c.loads[i], nil
}

func fallbackQueueService(cache ConcurrencyCache) *OpenAIGatewayService {
	older, newer := time.Now().Add(-time.Hour), time.Now()
	return &OpenAIGatewayService{
		accountRepo: stubOpenAIAccountRepo{accounts: []Account{
			{ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1, LastUsedAt: &older, GroupIDs: []int64{1}},
			{ID: 2, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1, LastUsedAt: &newer, GroupIDs: []int64{1}},
		}},
		concurrencyService: NewConcurrencyService(cache),
	}
}

func TestOpenAIFallbackQueueSkipsSaturatedQueue(t *testing.T) {
	cache := stubConcurrencyCache{
		acquireResults: map[int64]bool{1: false, 2: false},
		waitCounts:     map[int64]int{1: 100, 2: 0},
		loadMap: map[int64]*AccountLoadInfo{
			1: {AccountID: 1, CurrentConcurrency: 1, WaitingCount: 100, LoadRate: 10100},
			2: {AccountID: 2, CurrentConcurrency: 1, LoadRate: 100},
		},
	}
	svc := fallbackQueueService(cache)
	groupID := int64(1)
	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", "gpt-4", nil)
	require.NoError(t, err)
	require.NotNil(t, selection.WaitPlan)
	require.Equal(t, int64(2), selection.Account.ID, "a saturated queue must not hide another account's waiting capacity")
}

func TestOpenAIFallbackQueuePreservesAllFullWaitPlan(t *testing.T) {
	cache := stubConcurrencyCache{
		acquireResults: map[int64]bool{1: false, 2: false},
		waitCounts:     map[int64]int{1: 100, 2: 100},
		loadMap: map[int64]*AccountLoadInfo{
			1: {AccountID: 1, CurrentConcurrency: 1, WaitingCount: 100, LoadRate: 10100},
			2: {AccountID: 2, CurrentConcurrency: 1, WaitingCount: 100, LoadRate: 10100},
		},
	}
	svc := fallbackQueueService(cache)
	groupID := int64(1)
	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", "gpt-4", nil)
	require.NoError(t, err, "preserve the handler's queue-full response when every queue is full")
	require.NotNil(t, selection.WaitPlan)
	require.Equal(t, 100, selection.WaitPlan.MaxWaiting)
}

func TestOpenAIFallbackQueueRefreshesAllFullSnapshot(t *testing.T) {
	cache := &fallbackQueueLoadCache{
		stubConcurrencyCache: stubConcurrencyCache{
			acquireResults: map[int64]bool{1: false, 2: true},
		},
		loads: []map[int64]*AccountLoadInfo{
			{1: {AccountID: 1, CurrentConcurrency: 1, LoadRate: 100}, 2: {AccountID: 2, CurrentConcurrency: 1, LoadRate: 100}},
			{1: {AccountID: 1, CurrentConcurrency: 1, LoadRate: 100}, 2: {AccountID: 2, LoadRate: 0}},
		},
	}
	svc := fallbackQueueService(cache)
	groupID := int64(1)
	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", "gpt-4", nil)
	require.NoError(t, err)
	require.True(t, selection.Acquired, "refresh an all-full snapshot before queueing on another account")
	require.Equal(t, int64(2), selection.Account.ID)
	require.Equal(t, 2, cache.reads)
	selection.ReleaseFunc()
}

func TestOpenAIFallbackQueueSpilloverKeepsSessionAfterAdmission(t *testing.T) {
	cache := stubConcurrencyCache{
		acquireResults: map[int64]bool{1: false, 2: false},
		waitCounts:     map[int64]int{1: 100, 2: 0},
		loadMap: map[int64]*AccountLoadInfo{
			1: {AccountID: 1, CurrentConcurrency: 1, WaitingCount: 100, LoadRate: 10100},
			2: {AccountID: 2, CurrentConcurrency: 1, LoadRate: 100},
		},
	}
	svc := fallbackQueueService(cache)
	bindings := &stubGatewayCache{sessionBindings: map[string]int64{"openai:conversation": 1}}
	svc.cache = bindings
	groupID := int64(1)
	ctx := context.Background()
	selection, err := svc.SelectAccountWithLoadAwareness(ctx, &groupID, "conversation", "gpt-4", nil)
	require.NoError(t, err)
	require.NotNil(t, selection.WaitPlan)
	require.Equal(t, int64(2), selection.Account.ID)
	require.NoError(t, svc.BindStickySessionAfterSelection(ctx, &groupID, "conversation", selection))
	require.Equal(t, int64(1), bindings.sessionBindings["openai:conversation"], "a one-request spillover must not migrate the conversation after its wait completes")
}

func TestOpenAIFallbackQueueDoesNotUseExcludedCapacity(t *testing.T) {
	cache := stubConcurrencyCache{
		acquireResults: map[int64]bool{1: false, 2: true},
		waitCounts:     map[int64]int{1: 100, 2: 0},
		loadMap: map[int64]*AccountLoadInfo{
			1: {AccountID: 1, CurrentConcurrency: 1, WaitingCount: 100, LoadRate: 10100},
			2: {AccountID: 2, LoadRate: 0},
		},
	}
	svc := fallbackQueueService(cache)
	groupID := int64(1)
	selection, err := svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", "gpt-4", map[int64]struct{}{2: {}})
	require.NoError(t, err)
	require.Equal(t, int64(1), selection.Account.ID)
	require.NotNil(t, selection.WaitPlan)
}

func TestOpenAISelectionAdmissionBindsNewSession(t *testing.T) {
	svc := fallbackQueueService(stubConcurrencyCache{})
	bindings := &stubGatewayCache{sessionBindings: map[string]int64{}}
	svc.cache = bindings
	groupID := int64(1)
	selection := &AccountSelectionResult{Account: &Account{ID: 2}}
	require.NoError(t, svc.BindStickySessionAfterSelection(context.Background(), &groupID, "new-session", selection))
	require.Equal(t, int64(2), bindings.sessionBindings["openai:new-session"])
}
