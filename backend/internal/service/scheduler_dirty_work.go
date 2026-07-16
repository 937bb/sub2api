package service

import (
	"context"
	"time"
)

// SchedulerDirtyWork is an observed version of one bounded scheduler work key.
type SchedulerDirtyWork struct {
	Kind           int16
	EntityID       int64
	Generation     int64
	RebuildBuckets bool
	UpdatedAt      time.Time
}

type SchedulerDirtyWorkStats struct {
	Count           int64
	OldestUpdatedAt *time.Time
}

// SchedulerDirtyWorkRepository stores current dirty state, not an event log.
// Acknowledge only removes the exact generation returned by List.
type SchedulerDirtyWorkRepository interface {
	// Promote atomically derives canonical scopes and removes only the exact
	// source generations it observed. Only active ownership can authorize it.
	Promote(ctx context.Context, ownership SchedulerOwnership, limit int) (int, error)
	List(ctx context.Context, limit int) ([]SchedulerDirtyWork, error)
	Acknowledge(ctx context.Context, work SchedulerDirtyWork) (bool, error)
	PendingStats(ctx context.Context) (SchedulerDirtyWorkStats, error)
}

// SchedulerOwnership represents exclusive scheduler ownership tied to one
// PostgreSQL session. Context is canceled when ownership is lost or released.
type SchedulerOwnership interface {
	Context() context.Context
	Lost() <-chan struct{}
	Err() error
	Close() error
}

type SchedulerOwnershipRepository interface {
	TryAcquire(ctx context.Context) (SchedulerOwnership, bool, error)
}
