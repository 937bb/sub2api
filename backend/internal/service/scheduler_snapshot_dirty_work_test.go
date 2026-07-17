package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type dirtyWorkTestCache struct {
	outboxPollCache
	setAccounts    []*Account
	deletedAccount []int64
	setAccountErr  error
	lockAcquired   bool
}

func (c *dirtyWorkTestCache) SetAccount(_ context.Context, account *Account) error {
	c.setAccounts = append(c.setAccounts, account)
	return c.setAccountErr
}

func (c *dirtyWorkTestCache) DeleteAccount(_ context.Context, accountID int64) error {
	c.deletedAccount = append(c.deletedAccount, accountID)
	return nil
}

func (c *dirtyWorkTestCache) TryLockBucket(context.Context, SchedulerBucket, time.Duration) (bool, error) {
	return c.lockAcquired, nil
}

type dirtyWorkTestAccountRepo struct {
	AccountRepository
	account *Account
	err     error
}

func (r *dirtyWorkTestAccountRepo) GetByID(context.Context, int64) (*Account, error) {
	return r.account, r.err
}

type dirtyWorkTestRepo struct {
	promoteResults []int
	promoteCalls   int
	work           []SchedulerDirtyWork
	acknowledged   []SchedulerDirtyWork
	failures       []SchedulerDirtyWork
}

func (r *dirtyWorkTestRepo) Promote(context.Context, SchedulerOwnership, int) (int, error) {
	result := 0
	if r.promoteCalls < len(r.promoteResults) {
		result = r.promoteResults[r.promoteCalls]
	}
	r.promoteCalls++
	return result, nil
}

func (r *dirtyWorkTestRepo) RequestFullRebuild(context.Context) error { return nil }
func (r *dirtyWorkTestRepo) List(context.Context, int) ([]SchedulerDirtyWork, error) {
	return r.work, nil
}
func (r *dirtyWorkTestRepo) RecordFailure(_ context.Context, _ SchedulerOwnership, work SchedulerDirtyWork, _ error) (bool, error) {
	r.failures = append(r.failures, work)
	return true, nil
}
func (r *dirtyWorkTestRepo) Acknowledge(_ context.Context, _ SchedulerOwnership, work SchedulerDirtyWork) (bool, error) {
	r.acknowledged = append(r.acknowledged, work)
	return true, nil
}
func (r *dirtyWorkTestRepo) PendingStats(context.Context) (SchedulerDirtyWorkStats, error) {
	return SchedulerDirtyWorkStats{}, nil
}

type dirtyWorkTestOwnership struct {
	ctx    context.Context
	cancel context.CancelFunc
	lost   chan struct{}
}

func newDirtyWorkTestOwnership() *dirtyWorkTestOwnership {
	ctx, cancel := context.WithCancel(context.Background())
	return &dirtyWorkTestOwnership{ctx: ctx, cancel: cancel, lost: make(chan struct{})}
}

func (o *dirtyWorkTestOwnership) Epoch() int64             { return 1 }
func (o *dirtyWorkTestOwnership) Context() context.Context { return o.ctx }
func (o *dirtyWorkTestOwnership) Lost() <-chan struct{}    { return o.lost }
func (o *dirtyWorkTestOwnership) Err() error               { return o.ctx.Err() }
func (o *dirtyWorkTestOwnership) Close() error {
	o.cancel()
	return nil
}

type dirtyWorkTestOwnershipRepo struct{}

func (dirtyWorkTestOwnershipRepo) TryAcquire(context.Context) (SchedulerOwnership, bool, error) {
	return nil, false, nil
}

func TestSchedulerSnapshotDirtyWorkPromotesUntilDrainedAndAcknowledges(t *testing.T) {
	cache := &dirtyWorkTestCache{}
	account := &Account{ID: 42, Name: "fresh"}
	repo := &dirtyWorkTestRepo{
		promoteResults: []int{1, 1, 0},
		work:           []SchedulerDirtyWork{{Kind: SchedulerDirtyWorkAccount, EntityID: account.ID, Generation: 7}},
	}
	workerCtx, workerCancel := context.WithCancel(context.Background())
	svc := &SchedulerSnapshotService{
		cache:         cache,
		dirtyWorkRepo: repo,
		accountRepo:   &dirtyWorkTestAccountRepo{account: account},
		workerCtx:     workerCtx,
		workerCancel:  workerCancel,
	}
	owner := newDirtyWorkTestOwnership()

	workerCancel()
	svc.consumeDirtyWork(owner, time.Hour)

	require.Equal(t, 3, repo.promoteCalls)
	require.Equal(t, repo.work, repo.acknowledged)
	require.Empty(t, repo.failures)
	require.Equal(t, []*Account{account}, cache.setAccounts)
}

func TestSchedulerSnapshotDirtyWorkRecordsFailureWithoutAcknowledging(t *testing.T) {
	cache := &dirtyWorkTestCache{setAccountErr: errors.New("redis unavailable")}
	account := &Account{ID: 54, Name: "retry"}
	repo := &dirtyWorkTestRepo{
		promoteResults: []int{0},
		work:           []SchedulerDirtyWork{{Kind: SchedulerDirtyWorkAccount, EntityID: account.ID, Generation: 3}},
	}
	workerCtx, workerCancel := context.WithCancel(context.Background())
	svc := &SchedulerSnapshotService{
		cache:         cache,
		dirtyWorkRepo: repo,
		accountRepo:   &dirtyWorkTestAccountRepo{account: account},
		workerCtx:     workerCtx,
		workerCancel:  workerCancel,
	}
	owner := newDirtyWorkTestOwnership()

	workerCancel()
	svc.consumeDirtyWork(owner, time.Hour)

	require.Equal(t, repo.work, repo.failures)
	require.Empty(t, repo.acknowledged)
}

func TestSchedulerSnapshotDirtyWorkDeletesMissingAccount(t *testing.T) {
	cache := &dirtyWorkTestCache{}
	svc := &SchedulerSnapshotService{
		cache:       cache,
		accountRepo: &dirtyWorkTestAccountRepo{err: ErrAccountNotFound},
	}

	err := svc.handleDirtyWork(context.Background(), SchedulerDirtyWork{Kind: SchedulerDirtyWorkAccount, EntityID: 73})

	require.NoError(t, err)
	require.Equal(t, []int64{73}, cache.deletedAccount)
}

func TestSchedulerSnapshotDirtyWorkDrainsLegacyLifecycleEventWithoutDuplicateRebuild(t *testing.T) {
	cache := &dirtyWorkTestCache{lockAcquired: false}
	svc := &SchedulerSnapshotService{
		cache:         cache,
		dirtyWorkRepo: &dirtyWorkTestRepo{},
		ownershipRepo: dirtyWorkTestOwnershipRepo{},
	}
	accountID := int64(42)

	err := svc.handleOutboxEvent(context.Background(), SchedulerOutboxEvent{
		EventType: SchedulerOutboxEventAccountChanged,
		AccountID: &accountID,
	}, nil)

	require.NoError(t, err)
}

func TestSchedulerSnapshotDirtyWorkRetriesContendedBucket(t *testing.T) {
	cache := &dirtyWorkTestCache{lockAcquired: false}
	svc := &SchedulerSnapshotService{cache: cache}

	err := svc.handleDirtyWork(context.Background(), SchedulerDirtyWork{Kind: SchedulerDirtyWorkGroup, EntityID: 91})

	require.ErrorIs(t, err, errSchedulerBucketLockBusy)
}

func TestSchedulerSnapshotDirtyWorkRejectsUnknownKind(t *testing.T) {
	svc := &SchedulerSnapshotService{}
	err := svc.handleDirtyWork(context.Background(), SchedulerDirtyWork{Kind: 99})
	require.Error(t, err)
	require.False(t, errors.Is(err, context.Canceled))
}
