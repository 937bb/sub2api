package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"time"
)

// CheckinService 签到服务
type CheckinService struct {
	checkinRepo      CheckinRepository
	balanceEntrySvc  *BalanceEntryService
	settingService   *SettingService
}

// NewCheckinService 创建签到服务
func NewCheckinService(
	checkinRepo CheckinRepository,
	balanceEntrySvc *BalanceEntryService,
	settingService *SettingService,
) *CheckinService {
	return &CheckinService{
		checkinRepo:     checkinRepo,
		balanceEntrySvc: balanceEntrySvc,
		settingService:  settingService,
	}
}

// Checkin 执行签到
func (s *CheckinService) Checkin(ctx context.Context, userID int64) (*CheckinRecord, error) {
	config := s.GetCheckinConfig(ctx)
	if !config.Enabled {
		return nil, fmt.Errorf("checkin feature is disabled")
	}

	today := time.Now().Truncate(24 * time.Hour)

	// 防刷：检查今日是否已签到
	existing, err := s.checkinRepo.GetByUserAndDate(ctx, userID, today)
	if err != nil {
		return nil, fmt.Errorf("check existing checkin: %w", err)
	}
	if existing != nil {
		return nil, fmt.Errorf("already checked in today")
	}

	// 计算连续签到天数
	streak := 1
	latest, err := s.checkinRepo.GetLatestByUser(ctx, userID)
	if err != nil {
		slog.Warn("failed to get latest checkin", "error", err, "user_id", userID)
	} else if latest != nil {
		yesterday := today.AddDate(0, 0, -1)
		latestDate := latest.CheckinDate.Truncate(24 * time.Hour)
		if latestDate.Equal(yesterday) {
			streak = latest.Streak + 1
		}
	}

	// 计算基础奖励
	var baseAmount float64
	switch config.Mode {
	case "random":
		if config.RandomMax > config.RandomMin && config.RandomMin >= 0 {
			baseAmount = config.RandomMin + rand.Float64()*(config.RandomMax-config.RandomMin)
			baseAmount = math.Round(baseAmount*100) / 100
		} else {
			baseAmount = config.RandomMin
		}
	default: // "fixed"
		baseAmount = config.FixedAmount
	}

	// 计算里程碑奖励
	var milestoneAmount float64
	var milestoneBalanceType string
	var milestoneExpiryDays int
	for _, ms := range config.Milestones {
		if ms.Days > 0 && streak == ms.Days {
			milestoneAmount = ms.Amount
			milestoneBalanceType = ms.BalanceType
			milestoneExpiryDays = ms.ExpiryDays
			break
		}
	}

	totalAmount := baseAmount + milestoneAmount

	// 保存签到记录
	record := &CheckinRecord{
		UserID:          userID,
		CheckinDate:     today,
		Streak:          streak,
		BaseAmount:      baseAmount,
		MilestoneAmount: milestoneAmount,
		TotalAmount:     totalAmount,
		BalanceType:     config.BalanceType,
	}
	if err := s.checkinRepo.Create(ctx, record); err != nil {
		return nil, fmt.Errorf("create checkin record: %w", err)
	}

	// 发放基础奖励
	if baseAmount > 0 {
		input := &AddBalanceInput{
			UserID:      userID,
			Amount:      baseAmount,
			BalanceType: config.BalanceType,
			Source:      BalanceSourceCheckin,
			Note:        fmt.Sprintf("签到基础奖励 (连续%d天)", streak),
		}
		if config.BalanceType == BalanceTypeExpirable && config.ExpiryDays > 0 {
			exp := time.Now().AddDate(0, 0, config.ExpiryDays)
			input.ExpiresAt = &exp
		}
		if _, err := s.balanceEntrySvc.AddBalance(ctx, input); err != nil {
			slog.Error("failed to add checkin base balance", "error", err, "user_id", userID)
		}
	}

	// 发放里程碑奖励
	if milestoneAmount > 0 {
		bt := milestoneBalanceType
		if bt == "" {
			bt = BalanceTypePermanent
		}
		input := &AddBalanceInput{
			UserID:      userID,
			Amount:      milestoneAmount,
			BalanceType: bt,
			Source:      BalanceSourceCheckin,
			Note:        fmt.Sprintf("签到里程碑奖励 (连续%d天)", streak),
		}
		if bt == BalanceTypeExpirable && milestoneExpiryDays > 0 {
			exp := time.Now().AddDate(0, 0, milestoneExpiryDays)
			input.ExpiresAt = &exp
		}
		if _, err := s.balanceEntrySvc.AddBalance(ctx, input); err != nil {
			slog.Error("failed to add checkin milestone balance", "error", err, "user_id", userID)
		}
	}

	return record, nil
}

// GetStatus 获取用户签到状态
func (s *CheckinService) GetStatus(ctx context.Context, userID int64) (*CheckinStatus, error) {
	today := time.Now().Truncate(24 * time.Hour)

	status := &CheckinStatus{}

	// 检查今日是否签到
	todayRecord, err := s.checkinRepo.GetByUserAndDate(ctx, userID, today)
	if err != nil {
		return nil, fmt.Errorf("check today checkin: %w", err)
	}
	status.CheckedInToday = todayRecord != nil

	// 获取最新记录算连续签到天数
	latest, err := s.checkinRepo.GetLatestByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get latest checkin: %w", err)
	}
	if latest != nil {
		latestDate := latest.CheckinDate.Truncate(24 * time.Hour)
		status.LastCheckinDate = latestDate.Format("2006-01-02")
		// 如果最近签到是今天或昨天，streak 有效
		if latestDate.Equal(today) || latestDate.Equal(today.AddDate(0, 0, -1)) {
			status.CurrentStreak = latest.Streak
		}
	}

	// 总签到次数
	total, err := s.checkinRepo.CountByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("count checkins: %w", err)
	}
	status.TotalCheckins = total

	return status, nil
}

// GetMonthlyRecords 获取月度签到记录
func (s *CheckinService) GetMonthlyRecords(ctx context.Context, userID int64, year, month int) ([]*CheckinRecord, error) {
	return s.checkinRepo.GetMonthlyRecords(ctx, userID, year, month)
}

// ListHistory 签到历史（分页）
func (s *CheckinService) ListHistory(ctx context.Context, userID int64, offset, limit int) ([]*CheckinRecord, int64, error) {
	return s.checkinRepo.ListByUser(ctx, userID, offset, limit)
}

// GetCheckinConfig 从设置读取签到配置
func (s *CheckinService) GetCheckinConfig(ctx context.Context) *CheckinConfig {
	config := &CheckinConfig{
		Enabled:     false,
		Mode:        "fixed",
		FixedAmount: 0.01,
		RandomMin:   0.01,
		RandomMax:   1.00,
		BalanceType: BalanceTypePermanent,
		ExpiryDays:  0,
		Milestones:  nil,
	}

	if s.settingService == nil {
		return config
	}

	config.Enabled = s.settingService.GetBoolSetting(ctx, SettingKeyCheckinEnabled, false)
	config.Mode = s.settingService.GetStringSetting(ctx, SettingKeyCheckinMode, "fixed")
	config.FixedAmount = s.settingService.GetFloatSetting(ctx, SettingKeyCheckinFixedAmount, 0.01)
	config.RandomMin = s.settingService.GetFloatSetting(ctx, SettingKeyCheckinRandomMin, 1)
	config.RandomMax = s.settingService.GetFloatSetting(ctx, SettingKeyCheckinRandomMax, 10)
	config.BalanceType = s.settingService.GetStringSetting(ctx, SettingKeyCheckinBalanceType, BalanceTypePermanent)
	config.ExpiryDays = s.settingService.GetIntSetting(ctx, SettingKeyCheckinExpiryDays, 0)

	milestonesJSON := s.settingService.GetStringSetting(ctx, SettingKeyCheckinMilestones, "[]")
	if err := json.Unmarshal([]byte(milestonesJSON), &config.Milestones); err != nil {
		slog.Warn("failed to parse checkin milestones", "error", err)
		config.Milestones = nil
	}

	return config
}
