package service

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// DailyRewardScheduler 每日奖励定时器，每日凌晨执行排行榜奖励 + 消费返现
type DailyRewardScheduler struct {
	leaderboardService *LeaderboardService
	cashbackService    *CashbackService
	stopCh             chan struct{}
	stopOnce           sync.Once
	wg                 sync.WaitGroup
}

func NewDailyRewardScheduler(
	leaderboardService *LeaderboardService,
	cashbackService *CashbackService,
) *DailyRewardScheduler {
	return &DailyRewardScheduler{
		leaderboardService: leaderboardService,
		cashbackService:    cashbackService,
		stopCh:             make(chan struct{}),
	}
}

func (s *DailyRewardScheduler) Start() {
	if s == nil {
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		// 计算距离下一个凌晨 00:05 的时间
		now := time.Now()
		next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 5, 0, 0, now.Location())
		initialDelay := time.Until(next)
		if initialDelay < 0 {
			initialDelay += 24 * time.Hour
		}

		slog.Info("[DailyRewardScheduler] started", "next_run", next.Format(time.RFC3339))

		timer := time.NewTimer(initialDelay)
		defer timer.Stop()

		for {
			select {
			case <-timer.C:
				s.runOnce()
				// 重置为24小时后
				timer.Reset(24 * time.Hour)
			case <-s.stopCh:
				return
			}
		}
	}()
}

func (s *DailyRewardScheduler) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
	s.wg.Wait()
}

func (s *DailyRewardScheduler) runOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	slog.Info("[DailyRewardScheduler] running daily reward tasks")

	// 排行榜奖励
	if s.leaderboardService != nil {
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("[DailyRewardScheduler] leaderboard reward panic", "recover", r)
				}
			}()
			s.leaderboardService.RunDailyReward(ctx)
		}()
	}

	// 消费返现（daily 周期）
	if s.cashbackService != nil {
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("[DailyRewardScheduler] cashback panic", "recover", r)
				}
			}()
			s.cashbackService.RunDailyCashback(ctx)
		}()
	}

	slog.Info("[DailyRewardScheduler] daily reward tasks completed")
}

// ProvideDailyRewardScheduler 创建并启动每日奖励调度器
func ProvideDailyRewardScheduler(
	leaderboardService *LeaderboardService,
	cashbackService *CashbackService,
) *DailyRewardScheduler {
	svc := NewDailyRewardScheduler(leaderboardService, cashbackService)
	svc.Start()
	return svc
}
