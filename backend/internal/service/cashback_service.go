package service

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"time"
)

// CashbackService 消费返现服务
type CashbackService struct {
	settingService      *SettingService
	balanceEntryService *BalanceEntryService
	usageLogRepo        UsageLogRepository
}

// NewCashbackService creates a CashbackService
func NewCashbackService(
	settingService *SettingService,
	balanceEntryService *BalanceEntryService,
	usageLogRepo UsageLogRepository,
) *CashbackService {
	return &CashbackService{
		settingService:      settingService,
		balanceEntryService: balanceEntryService,
		usageLogRepo:        usageLogRepo,
	}
}

// RunDailyCashback 每日消费返现（结算周期=daily 时由定时任务调用）
func (s *CashbackService) RunDailyCashback(ctx context.Context) {
	if !s.settingService.GetBoolSetting(ctx, SettingKeyCashbackEnabled, false) {
		return
	}
	cycle := s.settingService.GetStringSetting(ctx, SettingKeyCashbackCycle, "daily")
	if cycle != "daily" {
		return
	}

	threshold := s.settingService.GetFloatSetting(ctx, SettingKeyCashbackThreshold, 1)

	// 获取昨日所有用户消费
	now := time.Now()
	yesterdayStart := time.Date(now.Year(), now.Month(), now.Day()-1, 0, 0, 0, 0, now.Location())
	yesterdayEnd := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	// 用 GetUserSpendingRanking 来获取所有有消费的用户（limit 设大）
	ranking, err := s.usageLogRepo.GetUserSpendingRanking(ctx, yesterdayStart, yesterdayEnd, 100000)
	if err != nil {
		slog.Error("cashback_daily: get spending ranking failed", "error", err)
		return
	}
	if ranking == nil || len(ranking.Ranking) == 0 {
		return
	}

	for _, item := range ranking.Ranking {
		if item.ActualCost < threshold {
			continue
		}
		s.applyCashback(ctx, item.UserID, item.ActualCost, "daily")
	}
}

// ApplyRealtimeCashback 实时消费返现（每次扣费后调用）
func (s *CashbackService) ApplyRealtimeCashback(ctx context.Context, userID int64, consumeAmount float64) {
	if !s.settingService.GetBoolSetting(ctx, SettingKeyCashbackEnabled, false) {
		return
	}
	cycle := s.settingService.GetStringSetting(ctx, SettingKeyCashbackCycle, "daily")
	if cycle != "realtime" {
		return
	}

	threshold := s.settingService.GetFloatSetting(ctx, SettingKeyCashbackThreshold, 1)
	if consumeAmount < threshold {
		return
	}

	s.applyCashback(ctx, userID, consumeAmount, "realtime")
}

func (s *CashbackService) applyCashback(ctx context.Context, userID int64, consumeAmount float64, source string) {
	mode := s.settingService.GetStringSetting(ctx, SettingKeyCashbackMode, "percent")
	balanceType := s.settingService.GetStringSetting(ctx, SettingKeyCashbackBalanceType, BalanceTypePermanent)
	expiryDays := s.settingService.GetIntSetting(ctx, SettingKeyCashbackExpiryDays, 0)

	var cashbackAmount float64
	switch mode {
	case "fixed":
		cashbackAmount = s.settingService.GetFloatSetting(ctx, SettingKeyCashbackFixedAmount, 0.1)
	case "percent":
		percent := s.settingService.GetFloatSetting(ctx, SettingKeyCashbackPercent, 5)
		cashbackAmount = consumeAmount * percent / 100
	case "random":
		rMin := s.settingService.GetIntSetting(ctx, SettingKeyCashbackRandomMin, 1)
		rMax := s.settingService.GetIntSetting(ctx, SettingKeyCashbackRandomMax, 10)
		if rMin > rMax {
			rMin, rMax = rMax, rMin
		}
		cashbackAmount = float64(rMin + rand.Intn(rMax-rMin+1))
	default:
		return
	}

	if cashbackAmount <= 0 {
		return
	}
	cashbackAmount = math.Round(cashbackAmount*10000) / 10000

	var expiresAt *time.Time
	if balanceType == BalanceTypeExpirable && expiryDays > 0 {
		t := time.Now().AddDate(0, 0, expiryDays)
		expiresAt = &t
	}

	if _, err := s.balanceEntryService.AddBalance(ctx, &AddBalanceInput{
		UserID:      userID,
		Amount:      cashbackAmount,
		BalanceType: balanceType,
		Source:      BalanceSourceCashback,
		Note:        fmt.Sprintf("消费返现 (%s, 消费 $%.4f)", source, consumeAmount),
		ExpiresAt:   expiresAt,
	}); err != nil {
		slog.Error("cashback: add balance failed", "user_id", userID, "cashback", cashbackAmount, "error", err)
		return
	}

	slog.Info("cashback: applied", "user_id", userID, "consume", consumeAmount, "cashback", cashbackAmount, "mode", mode)
}
