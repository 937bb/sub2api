package service

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// BalanceExpiryService 每日自动过期已到期的余额条目，并同步 user.balance 缓存字段。
// 开关关闭时跳过执行。
type BalanceExpiryService struct {
	balanceEntryService *BalanceEntryService
	settingService      *SettingService
	interval            time.Duration
	stopCh              chan struct{}
	stopOnce            sync.Once
	wg                  sync.WaitGroup
}

func NewBalanceExpiryService(
	balanceEntryService *BalanceEntryService,
	settingService *SettingService,
	interval time.Duration,
) *BalanceExpiryService {
	return &BalanceExpiryService{
		balanceEntryService: balanceEntryService,
		settingService:      settingService,
		interval:            interval,
		stopCh:              make(chan struct{}),
	}
}

func (s *BalanceExpiryService) Start() {
	if s == nil || s.balanceEntryService == nil || s.interval <= 0 {
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		// 启动时立即执行一次
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

func (s *BalanceExpiryService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
	s.wg.Wait()
}

func (s *BalanceExpiryService) runOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 检查开关
	if s.settingService != nil && !s.settingService.IsBalanceExpiryEnabled(ctx) {
		return
	}

	affectedUsers, err := s.balanceEntryService.ExpireBalances(ctx)
	if err != nil {
		slog.Error("[BalanceExpiry] expire balances failed", "error", err)
		return
	}
	if len(affectedUsers) > 0 {
		slog.Info("[BalanceExpiry] expired balance entries", "affected_users", len(affectedUsers))
	}
}

// ProvideBalanceExpiryService 创建并启动余额过期定时服务（每小时执行一次）
func ProvideBalanceExpiryService(
	balanceEntryService *BalanceEntryService,
	settingService *SettingService,
) *BalanceExpiryService {
	svc := NewBalanceExpiryService(balanceEntryService, settingService, time.Hour)
	svc.Start()
	return svc
}
