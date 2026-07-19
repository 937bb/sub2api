package service

import (
	"context"
	"time"
)

const (
	SchedulerDirtyWorkAccount int16 = 1
	SchedulerDirtyWorkGroup   int16 = 2
	SchedulerDirtyWorkGlobal  int16 = 3
)

// SchedulerDirtyWork is an observed version of one bounded scheduler work key.
type SchedulerDirtyWork struct {
	Kind           int16
	EntityID       int64
	Generation     int64
	RebuildBuckets bool
	UpdatedAt      time.Time
	FailureCount   int
	LastFailureAt  *time.Time
	LastError      string
	RetryAt        time.Time
}

type SchedulerDirtyWorkStats struct {
	Count           int64
	OldestUpdatedAt *time.Time
	FailedCount     int64
	OldestFailureAt *time.Time
}

// SchedulerDirtyWorkRepository stores current dirty state, not an event log.
// Acknowledge only removes the exact generation returned by List.
type SchedulerDirtyWorkRepository interface {
	// Promote atomically derives canonical scopes and removes only the exact
	// source generations it observed. Only active ownership can authorize it.
	Promote(ctx context.Context, ownership SchedulerOwnership, limit int) (int, error)
	RequestFullRebuild(ctx context.Context) error
	List(ctx context.Context, limit int) ([]SchedulerDirtyWork, error)
	RecordFailure(ctx context.Context, ownership SchedulerOwnership, work SchedulerDirtyWork, failure error) (bool, error)
	Acknowledge(ctx context.Context, ownership SchedulerOwnership, work SchedulerDirtyWork) (bool, error)
	PendingStats(ctx context.Context) (SchedulerDirtyWorkStats, error)
}

// SchedulerOwnership represents exclusive scheduler ownership tied to one
// PostgreSQL session. Context is canceled when ownership is lost or released.
type SchedulerOwnership interface {
	Epoch() int64
	Context() context.Context
	Lost() <-chan struct{}
	Err() error
	Close() error
}

type SchedulerOwnershipRepository interface {
	TryAcquire(ctx context.Context) (SchedulerOwnership, bool, error)
}
