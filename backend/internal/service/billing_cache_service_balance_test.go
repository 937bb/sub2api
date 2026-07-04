//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type balanceTestCache struct {
	billingCacheWorkerStub

	mu            sync.Mutex
	balance       float64
	alwaysMiss    bool
	invalidated   atomic.Bool
	deductErr     error
	invalidateErr error
	subData       *SubscriptionCacheData

	getBalanceCalls        atomic.Int64
	setBalanceCalls        atomic.Int64
	deductBalanceCalls     atomic.Int64
	invalidateBalanceCalls atomic.Int64
}

func (c *balanceTestCache) GetUserBalance(context.Context, int64) (float64, error) {
	c.getBalanceCalls.Add(1)
	if c.alwaysMiss || c.invalidated.Load() {
		return 0, errors.New("cache miss")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.balance, nil
}

func (c *balanceTestCache) SetUserBalance(_ context.Context, _ int64, balance float64) error {
	c.setBalanceCalls.Add(1)
	c.mu.Lock()
	c.balance = balance
	c.mu.Unlock()
	c.invalidated.Store(false)
	return nil
}

func (c *balanceTestCache) DeductUserBalance(_ context.Context, _ int64, amount float64) error {
	c.deductBalanceCalls.Add(1)
	if c.deductErr != nil {
		return c.deductErr
	}
	c.mu.Lock()
	c.balance -= amount
	c.mu.Unlock()
	return nil
}

func (c *balanceTestCache) InvalidateUserBalance(context.Context, int64) error {
	c.invalidateBalanceCalls.Add(1)
	if c.invalidateErr != nil {
		return c.invalidateErr
	}
	c.invalidated.Store(true)
	return nil
}

func (c *balanceTestCache) GetSubscriptionCache(context.Context, int64, int64) (*SubscriptionCacheData, error) {
	if c.subData == nil {
		return nil, errors.New("cache miss")
	}
	return c.subData, nil
}

type interleavingBalanceCache struct {
	balanceTestCache

	setStarted        chan struct{}
	releaseSet        chan struct{}
	invalidateStarted chan struct{}
	releaseInvalidate chan struct{}

	setOnce        sync.Once
	invalidateOnce sync.Once
}

func (c *interleavingBalanceCache) SetUserBalance(ctx context.Context, userID int64, balance float64) error {
	if c.setStarted != nil {
		c.setOnce.Do(func() { close(c.setStarted) })
	}
	if c.releaseSet != nil {
		select {
		case <-c.releaseSet:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return c.balanceTestCache.SetUserBalance(ctx, userID, balance)
}

func (c *interleavingBalanceCache) InvalidateUserBalance(ctx context.Context, userID int64) error {
	if c.invalidateStarted != nil {
		c.invalidateOnce.Do(func() { close(c.invalidateStarted) })
	}
	if c.releaseInvalidate != nil {
		select {
		case <-c.releaseInvalidate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return c.balanceTestCache.InvalidateUserBalance(ctx, userID)
}

type generationBalanceTestCache struct {
	balanceTestCache

	generation            atomic.Int64
	generationErr         error
	getGenerationCalls    atomic.Int64
	setIfGenerationCalls  atomic.Int64
	setIfGenerationMisses atomic.Int64
}

func (c *generationBalanceTestCache) GetUserBalanceGeneration(context.Context, int64) (int64, error) {
	c.getGenerationCalls.Add(1)
	if c.generationErr != nil {
		return 0, c.generationErr
	}
	return c.generation.Load(), nil
}

func (c *generationBalanceTestCache) SetUserBalanceIfGeneration(ctx context.Context, userID int64, balance float64, generation int64) (bool, error) {
	c.setIfGenerationCalls.Add(1)
	if c.generation.Load() != generation {
		c.setIfGenerationMisses.Add(1)
		return false, nil
	}
	c.setBalanceCalls.Add(1)
	c.mu.Lock()
	c.balance = balance
	c.mu.Unlock()
	c.invalidated.Store(false)
	return true, nil
}

func (c *generationBalanceTestCache) DeductUserBalance(ctx context.Context, userID int64, amount float64) error {
	c.generation.Add(1)
	return c.balanceTestCache.DeductUserBalance(ctx, userID, amount)
}

func (c *generationBalanceTestCache) InvalidateUserBalance(ctx context.Context, userID int64) error {
	c.generation.Add(1)
	return c.balanceTestCache.InvalidateUserBalance(ctx, userID)
}

type blockingBalanceUserRepo struct {
	mockUserRepo

	mu          sync.Mutex
	balance     float64
	readStarted chan struct{}
	releaseRead chan struct{}
	readOnce    sync.Once
	calls       atomic.Int64
}

func (r *blockingBalanceUserRepo) GetByID(ctx context.Context, id int64) (*User, error) {
	r.calls.Add(1)
	r.mu.Lock()
	balance := r.balance
	r.mu.Unlock()

	if r.readStarted != nil {
		r.readOnce.Do(func() {
			close(r.readStarted)
			if r.releaseRead != nil {
				select {
				case <-r.releaseRead:
				case <-ctx.Done():
					return
				}
			}
		})
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}

	return &User{ID: id, Balance: balance}, nil
}

func (r *blockingBalanceUserRepo) SetBalance(balance float64) {
	r.mu.Lock()
	r.balance = balance
	r.mu.Unlock()
}

func newBalanceTestService(t *testing.T, cache BillingCache, userRepo UserRepository, cfg *config.Config) *BillingCacheService {
	t.Helper()
	if cfg == nil {
		cfg = &config.Config{}
	}
	svc := NewBillingCacheService(cache, userRepo, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(svc.Stop)
	return svc
}

func TestBalancePreflightDefaultRejectsZeroAndNegativeAllowsPositive(t *testing.T) {
	zero := newBalanceTestService(t, &balanceTestCache{balance: 0}, nil, &config.Config{})
	err := zero.CheckBillingEligibility(context.Background(), &User{ID: 1}, nil, nil, nil, "")
	require.ErrorIs(t, err, ErrInsufficientBalance)

	negative := newBalanceTestService(t, &balanceTestCache{balance: -0.0000001}, nil, &config.Config{})
	err = negative.CheckBillingEligibility(context.Background(), &User{ID: 1}, nil, nil, nil, "")
	require.ErrorIs(t, err, ErrInsufficientBalance)

	positive := newBalanceTestService(t, &balanceTestCache{balance: 0.0000001}, nil, &config.Config{})
	err = positive.CheckBillingEligibility(context.Background(), &User{ID: 1}, nil, nil, nil, "")
	require.NoError(t, err)
}

func TestBalancePreflightConfiguredReserveRejectsAtOrBelowThreshold(t *testing.T) {
	cfg := &config.Config{}
	cfg.Billing.MinimumBalanceReserve = 0.25

	below := newBalanceTestService(t, &balanceTestCache{balance: 0.24}, nil, cfg)
	err := below.CheckBillingEligibility(context.Background(), &User{ID: 1}, nil, nil, nil, "")
	require.ErrorIs(t, err, ErrInsufficientBalance)

	atReserve := newBalanceTestService(t, &balanceTestCache{balance: 0.25}, nil, cfg)
	err = atReserve.CheckBillingEligibility(context.Background(), &User{ID: 1}, nil, nil, nil, "")
	require.ErrorIs(t, err, ErrInsufficientBalance)

	above := newBalanceTestService(t, &balanceTestCache{balance: 0.250001}, nil, cfg)
	err = above.CheckBillingEligibility(context.Background(), &User{ID: 1}, nil, nil, nil, "")
	require.NoError(t, err)
}

func TestBalancePreflightBypassesSimpleMode(t *testing.T) {
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cache := &balanceTestCache{balance: -1}
	svc := newBalanceTestService(t, cache, nil, cfg)

	err := svc.CheckBillingEligibility(context.Background(), &User{ID: 1}, nil, nil, nil, "")
	require.NoError(t, err)
	require.Zero(t, cache.getBalanceCalls.Load())
}

func TestBalancePreflightBypassesSubscriptionMode(t *testing.T) {
	cache := &balanceTestCache{
		balance: -1,
		subData: &SubscriptionCacheData{
			Status:    SubscriptionStatusActive,
			ExpiresAt: time.Now().Add(time.Hour),
		},
	}
	svc := newBalanceTestService(t, cache, nil, &config.Config{})

	err := svc.CheckBillingEligibility(
		context.Background(),
		&User{ID: 1},
		nil,
		&Group{ID: 2, SubscriptionType: SubscriptionTypeSubscription},
		&UserSubscription{ID: 3},
		"",
	)
	require.NoError(t, err)
	require.Zero(t, cache.getBalanceCalls.Load())
}

func TestBillingCachePositiveHitDoesNotReadDB(t *testing.T) {
	cache := &balanceTestCache{balance: 5}
	userRepo := &balanceLoadUserRepoStub{balance: -1}
	svc := newBalanceTestService(t, cache, userRepo, &config.Config{})

	err := svc.CheckBillingEligibility(context.Background(), &User{ID: 1}, nil, nil, nil, "")
	require.NoError(t, err)
	require.Equal(t, int64(1), cache.getBalanceCalls.Load())
	require.Zero(t, userRepo.calls.Load())
}

func TestBillingCacheDelayedPositiveFillDoesNotUndoBalanceInvalidation(t *testing.T) {
	cache := &balanceTestCache{alwaysMiss: true}
	userRepo := &balanceLoadUserRepoStub{
		delay:   80 * time.Millisecond,
		balance: 5,
	}
	svc := newBalanceTestService(t, cache, userRepo, &config.Config{})

	done := make(chan error, 1)
	go func() {
		balance, err := svc.GetUserBalance(context.Background(), 42)
		if err != nil {
			done <- err
			return
		}
		if balance != 5 {
			done <- fmt.Errorf("balance = %v, want 5", balance)
			return
		}
		done <- nil
	}()

	require.Eventually(t, func() bool {
		return userRepo.calls.Load() == 1
	}, time.Second, 10*time.Millisecond)
	newBalance := -0.25
	require.NoError(t, svc.SyncBalanceCacheAfterDeduction(context.Background(), 42, 5.25, &newBalance))
	require.NoError(t, <-done)

	require.Never(t, func() bool {
		return cache.setBalanceCalls.Load() > 0
	}, 150*time.Millisecond, 10*time.Millisecond)
	require.Equal(t, int64(1), cache.invalidateBalanceCalls.Load())
}

func TestBillingCacheStalePositiveFillSkippedWhenInvalidationWinsInterleaving(t *testing.T) {
	cache := &interleavingBalanceCache{
		invalidateStarted: make(chan struct{}),
		releaseInvalidate: make(chan struct{}),
	}
	svc := newBalanceTestService(t, cache, nil, &config.Config{})

	version := svc.currentBalanceCacheVersion(42)
	newBalance := -0.25
	invalidateDone := make(chan error, 1)
	go func() {
		invalidateDone <- svc.SyncBalanceCacheAfterDeduction(context.Background(), 42, 5.25, &newBalance)
	}()

	require.Eventually(t, func() bool {
		select {
		case <-cache.invalidateStarted:
			return true
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)

	fillDone := make(chan struct{})
	go func() {
		svc.setBalanceCacheIfCurrent(context.Background(), cacheWriteTask{
			kind:           cacheWriteSetBalance,
			userID:         42,
			balance:        5,
			balanceVersion: version,
			createdAt:      time.Now(),
		})
		close(fillDone)
	}()

	require.Never(t, func() bool {
		select {
		case <-fillDone:
			return true
		default:
			return false
		}
	}, 50*time.Millisecond, 10*time.Millisecond)

	close(cache.releaseInvalidate)
	require.NoError(t, <-invalidateDone)
	require.Eventually(t, func() bool {
		select {
		case <-fillDone:
			return true
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)

	require.Zero(t, cache.setBalanceCalls.Load())
	require.Equal(t, int64(1), cache.invalidateBalanceCalls.Load())
}

func TestBillingCacheConcurrentInvalidationAfterVersionCheckClearsPositiveFill(t *testing.T) {
	cache := &interleavingBalanceCache{
		setStarted: make(chan struct{}),
		releaseSet: make(chan struct{}),
	}
	svc := newBalanceTestService(t, cache, nil, &config.Config{})

	version := svc.currentBalanceCacheVersion(42)
	fillDone := make(chan struct{})
	go func() {
		svc.setBalanceCacheIfCurrent(context.Background(), cacheWriteTask{
			kind:           cacheWriteSetBalance,
			userID:         42,
			balance:        5,
			balanceVersion: version,
			createdAt:      time.Now(),
		})
		close(fillDone)
	}()

	require.Eventually(t, func() bool {
		select {
		case <-cache.setStarted:
			return true
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)

	invalidateDone := make(chan error, 1)
	go func() {
		invalidateDone <- svc.InvalidateUserBalance(context.Background(), 42)
	}()

	require.Never(t, func() bool {
		select {
		case <-invalidateDone:
			return true
		default:
			return false
		}
	}, 50*time.Millisecond, 10*time.Millisecond)

	close(cache.releaseSet)
	require.Eventually(t, func() bool {
		select {
		case <-fillDone:
			return true
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)
	require.NoError(t, <-invalidateDone)

	require.Equal(t, int64(1), cache.setBalanceCalls.Load())
	require.Equal(t, int64(1), cache.invalidateBalanceCalls.Load())
	require.True(t, cache.invalidated.Load())
}

func TestBillingCacheCrossInstanceStalePositiveFillDoesNotRestoreEligibilityAfterInvalidation(t *testing.T) {
	cache := &generationBalanceTestCache{}
	cache.invalidated.Store(true)
	repo := &blockingBalanceUserRepo{
		balance:     5,
		readStarted: make(chan struct{}),
		releaseRead: make(chan struct{}),
	}
	instanceA := newBalanceTestService(t, cache, repo, &config.Config{})
	instanceB := newBalanceTestService(t, cache, nil, &config.Config{})

	done := make(chan error, 1)
	go func() {
		balance, err := instanceA.GetUserBalance(context.Background(), 42)
		if err != nil {
			done <- err
			return
		}
		if balance != 5 {
			done <- fmt.Errorf("balance = %v, want 5", balance)
			return
		}
		done <- nil
	}()

	require.Eventually(t, func() bool {
		return cache.getGenerationCalls.Load() == 1
	}, time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool {
		select {
		case <-repo.readStarted:
			return true
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)

	repo.SetBalance(-0.25)
	newBalance := -0.25
	require.NoError(t, instanceB.SyncBalanceCacheAfterDeduction(context.Background(), 42, 5.25, &newBalance))
	close(repo.releaseRead)
	require.NoError(t, <-done)

	require.Eventually(t, func() bool {
		return cache.setIfGenerationCalls.Load() == 1
	}, time.Second, 10*time.Millisecond)
	require.Equal(t, int64(1), cache.setIfGenerationMisses.Load())
	require.Zero(t, cache.setBalanceCalls.Load())
	_, err := cache.GetUserBalance(context.Background(), 42)
	require.Error(t, err)

	err = instanceA.CheckBillingEligibility(context.Background(), &User{ID: 42}, nil, nil, nil, "")
	require.ErrorIs(t, err, ErrInsufficientBalance)
}

func TestBillingCacheCrossInstanceStalePositiveFillDoesNotOverwriteAfterDeduct(t *testing.T) {
	cache := &generationBalanceTestCache{}
	cache.invalidated.Store(true)
	repo := &blockingBalanceUserRepo{
		balance:     5,
		readStarted: make(chan struct{}),
		releaseRead: make(chan struct{}),
	}
	instanceA := newBalanceTestService(t, cache, repo, &config.Config{})
	instanceB := newBalanceTestService(t, cache, nil, &config.Config{})

	done := make(chan error, 1)
	go func() {
		balance, err := instanceA.GetUserBalance(context.Background(), 42)
		if err != nil {
			done <- err
			return
		}
		if balance != 5 {
			done <- fmt.Errorf("balance = %v, want 5", balance)
			return
		}
		done <- nil
	}()

	require.Eventually(t, func() bool {
		return cache.getGenerationCalls.Load() == 1
	}, time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool {
		select {
		case <-repo.readStarted:
			return true
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)

	repo.SetBalance(4)
	require.NoError(t, instanceB.DeductBalanceCache(context.Background(), 42, 1))
	close(repo.releaseRead)
	require.NoError(t, <-done)

	require.Eventually(t, func() bool {
		return cache.setIfGenerationCalls.Load() == 1
	}, time.Second, 10*time.Millisecond)
	require.Equal(t, int64(1), cache.setIfGenerationMisses.Load())
	require.Zero(t, cache.setBalanceCalls.Load())
}

func TestBillingCacheGenerationReadFailureSkipsPositiveCacheWrite(t *testing.T) {
	cache := &generationBalanceTestCache{
		generationErr: errors.New("redis unavailable"),
	}
	cache.invalidated.Store(true)
	userRepo := &balanceLoadUserRepoStub{balance: 5}
	svc := newBalanceTestService(t, cache, userRepo, &config.Config{})

	balance, err := svc.GetUserBalance(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, 5.0, balance)
	require.Equal(t, int64(1), cache.getGenerationCalls.Load())

	require.Never(t, func() bool {
		return cache.setIfGenerationCalls.Load() > 0 || cache.setBalanceCalls.Load() > 0
	}, 150*time.Millisecond, 10*time.Millisecond)
}

func TestBillingCacheSyncBalanceAfterDeductionInvalidatesIneligibleBalance(t *testing.T) {
	cache := &balanceTestCache{balance: 0.50}
	userRepo := &balanceLoadUserRepoStub{balance: -0.25}
	svc := newBalanceTestService(t, cache, userRepo, &config.Config{})

	newBalance := -0.25
	require.NoError(t, svc.SyncBalanceCacheAfterDeduction(context.Background(), 1, 0.75, &newBalance))

	require.Equal(t, int64(1), cache.invalidateBalanceCalls.Load())
	require.Zero(t, cache.deductBalanceCalls.Load())

	err := svc.CheckBillingEligibility(context.Background(), &User{ID: 1}, nil, nil, nil, "")
	require.ErrorIs(t, err, ErrInsufficientBalance)
	require.Equal(t, int64(1), userRepo.calls.Load())
}

func TestBillingCacheFailedExhaustedInvalidationDoesNotUseStalePositiveCache(t *testing.T) {
	cache := &balanceTestCache{
		balance:       0.50,
		invalidateErr: errors.New("redis unavailable"),
	}
	userRepo := &balanceLoadUserRepoStub{balance: -0.25}
	svc := newBalanceTestService(t, cache, userRepo, &config.Config{})

	newBalance := -0.25
	err := svc.SyncBalanceCacheAfterDeduction(context.Background(), 1, 0.75, &newBalance)
	require.Error(t, err)
	require.Equal(t, int64(1), cache.invalidateBalanceCalls.Load())

	err = svc.CheckBillingEligibility(context.Background(), &User{ID: 1}, nil, nil, nil, "")
	require.ErrorIs(t, err, ErrInsufficientBalance)
	require.Equal(t, int64(1), userRepo.calls.Load())
}

func TestBillingCacheSyncBalanceAfterDeductionQueuesDeductWhenStillEligible(t *testing.T) {
	cache := &balanceTestCache{balance: 1}
	svc := newBalanceTestService(t, cache, nil, &config.Config{})

	newBalance := 0.75
	require.NoError(t, svc.SyncBalanceCacheAfterDeduction(context.Background(), 1, 0.25, &newBalance))

	require.Zero(t, cache.invalidateBalanceCalls.Load())
	require.Equal(t, int64(1), cache.deductBalanceCalls.Load())
}

func TestBillingCacheSyncDeductionBelowZeroIsIneligible(t *testing.T) {
	cache := &balanceTestCache{balance: 0.02}
	svc := newBalanceTestService(t, cache, nil, &config.Config{})

	require.NoError(t, svc.DeductBalanceCache(context.Background(), 1, 0.03))
	err := svc.CheckBillingEligibility(context.Background(), &User{ID: 1}, nil, nil, nil, "")
	require.ErrorIs(t, err, ErrInsufficientBalance)
}

func TestBillingCacheFailedDeductDoesNotUseStalePositiveCache(t *testing.T) {
	cache := &balanceTestCache{
		balance:   0.50,
		deductErr: errors.New("redis unavailable"),
	}
	userRepo := &balanceLoadUserRepoStub{balance: -0.25}
	svc := newBalanceTestService(t, cache, userRepo, &config.Config{})

	err := svc.DeductBalanceCache(context.Background(), 1, 0.75)
	require.Error(t, err)
	require.Equal(t, int64(1), cache.deductBalanceCalls.Load())

	err = svc.CheckBillingEligibility(context.Background(), &User{ID: 1}, nil, nil, nil, "")
	require.ErrorIs(t, err, ErrInsufficientBalance)
	require.Equal(t, int64(1), userRepo.calls.Load())
}

func TestBillingCacheAsyncDeductionBelowZeroIsIneligible(t *testing.T) {
	cache := &balanceTestCache{balance: 0.02}
	svc := newBalanceTestService(t, cache, nil, &config.Config{})

	svc.QueueDeductBalance(1, 0.03)
	require.Eventually(t, func() bool {
		return cache.deductBalanceCalls.Load() == 1
	}, time.Second, 10*time.Millisecond)
	err := svc.CheckBillingEligibility(context.Background(), &User{ID: 1}, nil, nil, nil, "")
	require.ErrorIs(t, err, ErrInsufficientBalance)
}

func TestGatewayBalanceFinalizationUsesAuthoritativeNewBalanceForCacheSync(t *testing.T) {
	cache := &balanceTestCache{balance: 10}
	userRepo := &balanceLoadUserRepoStub{balance: -0.25}
	svc := newBalanceTestService(t, cache, userRepo, &config.Config{})
	accountRepo := &mockAccountRepoForPlatform{}

	newBalance := -0.25
	finalizePostUsageBilling(context.Background(), &postUsageBillingParams{
		Cost:    &CostBreakdown{ActualCost: 0.75},
		User:    &User{ID: 1},
		APIKey:  &APIKey{ID: 2},
		Account: &Account{ID: 3},
	}, &billingDeps{
		billingCacheService: svc,
		deferredService:     NewDeferredService(accountRepo, nil, time.Hour),
		cfg:                 &config.Config{},
	}, &UsageBillingApplyResult{
		Applied:    true,
		NewBalance: &newBalance,
	})

	require.Equal(t, int64(1), cache.invalidateBalanceCalls.Load())
	require.Zero(t, cache.deductBalanceCalls.Load())

	err := svc.CheckBillingEligibility(context.Background(), &User{ID: 1}, nil, nil, nil, "")
	require.ErrorIs(t, err, ErrInsufficientBalance)
	require.Equal(t, int64(1), userRepo.calls.Load())
}
