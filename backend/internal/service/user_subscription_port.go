package service

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

type UserSubscriptionRepository interface {
	Create(ctx context.Context, sub *UserSubscription) error
	GetByID(ctx context.Context, id int64) (*UserSubscription, error)
	GetByIDForUpdate(ctx context.Context, id int64) (*UserSubscription, error)
	GetByIDIncludeDeleted(ctx context.Context, id int64) (*UserSubscription, error)
	GetByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*UserSubscription, error)
	GetActiveByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*UserSubscription, error)
	Update(ctx context.Context, sub *UserSubscription) error
	Delete(ctx context.Context, id int64) error
	Restore(ctx context.Context, subscriptionID int64, restoredStatus string) (*UserSubscription, error)

	ListByUserID(ctx context.Context, userID int64) ([]UserSubscription, error)
	ListActiveByUserID(ctx context.Context, userID int64) ([]UserSubscription, error)
	ListByGroupID(ctx context.Context, groupID int64, params pagination.PaginationParams) ([]UserSubscription, *pagination.PaginationResult, error)
	List(ctx context.Context, params pagination.PaginationParams, userID, groupID *int64, status, platform, sortBy, sortOrder string) ([]UserSubscription, *pagination.PaginationResult, error)

	ExistsByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (bool, error)
	ExistsActiveByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (bool, error)
	ExtendExpiry(ctx context.Context, subscriptionID int64, newExpiresAt time.Time) error
	UpdateStatus(ctx context.Context, subscriptionID int64, status string) error
	UpdateNotes(ctx context.Context, subscriptionID int64, notes string) error

	// ActivateWindows activates usage windows on first use. All windows are anchored
	// at the exact activation instant supplied by the caller.
	// It only applies when all three windows are inactive.
	ActivateWindows(ctx context.Context, id int64, dailyStart, periodicStart time.Time) error
	// ResetUsageWindows manually resets selected usage windows. Daily and periodic
	// windows use the supplied reset instants as their new anchors.
	ResetUsageWindows(ctx context.Context, id int64, resetDaily, resetWeekly, resetMonthly bool, dailyStart, periodicStart time.Time) error
	ResetDailyUsage(ctx context.Context, id int64, expectedWindowStart *time.Time, newWindowStart time.Time) error
	ResetWeeklyUsage(ctx context.Context, id int64, expectedWindowStart *time.Time, newWindowStart time.Time) error
	ResetMonthlyUsage(ctx context.Context, id int64, expectedWindowStart *time.Time, newWindowStart time.Time) error
	IncrementUsage(ctx context.Context, id int64, costUSD float64) error

	BatchUpdateExpiredStatus(ctx context.Context) (int64, error)
}

// DailyQuotaAdvanceLedger stores the durable result of a daily quota advance.
// ResponseSnapshot is written before the subscription transaction commits.
type DailyQuotaAdvanceLedger struct {
	UserID             int64
	SubscriptionID     int64
	IdempotencyKeyHash string
	ResponseSnapshot   string
	ExpiresAt          time.Time
}

// DailyQuotaAdvanceLedgerRepository is implemented by the production
// subscription repository. Keeping it separate avoids coupling unrelated
// repository test doubles to this operation-specific ledger.
type DailyQuotaAdvanceLedgerRepository interface {
	GetDailyQuotaAdvance(ctx context.Context, userID int64, keyHash string) (*DailyQuotaAdvanceLedger, error)
	ClaimDailyQuotaAdvance(ctx context.Context, record *DailyQuotaAdvanceLedger) (bool, error)
	CompleteDailyQuotaAdvance(ctx context.Context, userID int64, keyHash, responseSnapshot string) error
}
