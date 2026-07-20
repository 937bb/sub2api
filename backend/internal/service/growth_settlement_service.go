package service

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/google/uuid"
)

const (
	growthSettlementInterval = time.Hour
	growthSettlementTimeout  = 2 * time.Minute
	growthSettlementGrace    = 15 * time.Minute
	growthSettlementLockKey  = "growth:leaderboard:settlement:leader"
	growthSettlementLockTTL  = 3 * time.Minute
)

// GrowthSettlementService settles completed leaderboard periods. Repository
// uniqueness is the final idempotency guard; the leader lock avoids redundant
// work across application instances.
type GrowthSettlementService struct {
	growth     *GrowthService
	lockCache  LeaderLockCache
	db         *sql.DB
	instanceID string
	stopCh     chan struct{}
	stopOnce   sync.Once
	wg         sync.WaitGroup
}

func NewGrowthSettlementService(growth *GrowthService, lockCache LeaderLockCache, db *sql.DB) *GrowthSettlementService {
	return &GrowthSettlementService{
		growth: growth, lockCache: lockCache, db: db,
		instanceID: uuid.NewString(), stopCh: make(chan struct{}),
	}
}

func (s *GrowthSettlementService) Start() {
	if s == nil || s.growth == nil {
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(growthSettlementInterval)
		defer ticker.Stop()
		s.runOnce()
		for {
			select {
			case <-ticker.C:
				s.runOnce()
			case <-s.stopCh:
				return
			}
		}
	}()
}

func (s *GrowthSettlementService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() { close(s.stopCh) })
	s.wg.Wait()
}

func (s *GrowthSettlementService) runOnce() {
	now := timezone.Now()
	if now.Sub(dateOnly(now)) < growthSettlementGrace {
		return
	}
	lockCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	release, ok := tryAcquireSingletonLeaderLock(lockCtx, s.lockCache, s.db, growthSettlementLockKey, s.instanceID, growthSettlementLockTTL)
	cancel()
	if !ok {
		return
	}
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), growthSettlementTimeout)
	defer cancel()
	for _, period := range []string{"daily", "weekly", "monthly"} {
		users, amount, err := s.growth.SettlePreviousPeriod(ctx, period)
		if err != nil {
			slog.Error("[GrowthSettlement] settlement failed", "period", period, "error", err)
			continue
		}
		if users > 0 {
			slog.Info("[GrowthSettlement] rewards settled", "period", period, "users", users, "amount", amount)
		}
	}
}
