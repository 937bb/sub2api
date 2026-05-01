package service

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"time"
)

// RedeemBonusService 兑换加成服务
// 处理首次兑换加成 (F4) 和常规兑换加成 (F5)
type RedeemBonusService struct {
	settingService      *SettingService
	balanceEntryService *BalanceEntryService
	userRepo            UserRepository
	redeemRepo          RedeemCodeRepository
}

// NewRedeemBonusService creates a RedeemBonusService
func NewRedeemBonusService(
	settingService *SettingService,
	balanceEntryService *BalanceEntryService,
	userRepo UserRepository,
	redeemRepo RedeemCodeRepository,
) *RedeemBonusService {
	return &RedeemBonusService{
		settingService:      settingService,
		balanceEntryService: balanceEntryService,
		userRepo:            userRepo,
		redeemRepo:          redeemRepo,
	}
}

// ApplyRedeemBonus 兑换码余额充值后调用，异步发放首次加成 + 常规加成
// redeemAmount: 本次兑换的余额金额（正数）
// isFirstRedeem: 是否为用户首次余额兑换
func (s *RedeemBonusService) ApplyRedeemBonus(ctx context.Context, userID int64, redeemAmount float64, isFirstRedeem bool) {
	// F4: 首次兑换加成
	if isFirstRedeem {
		s.applyFirstRedeemBonus(ctx, userID, redeemAmount)
	}

	// F5: 常规兑换加成（含首次，可与 F4 叠加）
	s.applyRegularRedeemBonus(ctx, userID, redeemAmount)
}

// applyFirstRedeemBonus 首次兑换加成
func (s *RedeemBonusService) applyFirstRedeemBonus(ctx context.Context, userID int64, redeemAmount float64) {
	if !s.settingService.GetBoolSetting(ctx, SettingKeyFirstRedeemBonusEnabled, false) {
		return
	}

	multiplier := s.settingService.GetFloatSetting(ctx, SettingKeyFirstRedeemBonusMultiplier, 2)
	cap := s.settingService.GetFloatSetting(ctx, SettingKeyFirstRedeemBonusCap, 0)
	balanceType := s.settingService.GetStringSetting(ctx, SettingKeyFirstRedeemBonusBalanceType, BalanceTypePermanent)
	expiryDays := s.settingService.GetIntSetting(ctx, SettingKeyFirstRedeemBonusExpiryDays, 0)

	// 加成金额 = 充值金额 * (倍率 - 1)
	// 例如倍率=2，充值10，加成=10*1=10
	bonusAmount := redeemAmount * (multiplier - 1)
	if bonusAmount <= 0 {
		return
	}
	if cap > 0 && bonusAmount > cap {
		bonusAmount = cap
	}

	// 计算过期时间
	var expiresAt *time.Time
	if balanceType == BalanceTypeExpirable && expiryDays > 0 {
		t := time.Now().AddDate(0, 0, expiryDays)
		expiresAt = &t
	}

	// 入账
	if _, err := s.balanceEntryService.AddBalance(ctx, &AddBalanceInput{
		UserID:      userID,
		Amount:      bonusAmount,
		BalanceType: balanceType,
		Source:      BalanceSourceFirstRedeemBonus,
		Note:        fmt.Sprintf("首次兑换加成 (%.2fx, 充值 $%.4f)", multiplier, redeemAmount),
		ExpiresAt:   expiresAt,
	}); err != nil {
		slog.Error("first_redeem_bonus: add balance failed", "user_id", userID, "bonus", bonusAmount, "error", err)
		return
	}

	slog.Info("first_redeem_bonus: applied", "user_id", userID, "redeem_amount", redeemAmount, "bonus", bonusAmount)
}

// applyRegularRedeemBonus 常规兑换加成
func (s *RedeemBonusService) applyRegularRedeemBonus(ctx context.Context, userID int64, redeemAmount float64) {
	if !s.settingService.GetBoolSetting(ctx, SettingKeyRedeemBonusEnabled, false) {
		return
	}

	// 检查最低触发金额
	minAmount := s.settingService.GetFloatSetting(ctx, SettingKeyRedeemBonusMinAmount, 0)
	if minAmount > 0 && redeemAmount < minAmount {
		return
	}

	mode := s.settingService.GetStringSetting(ctx, SettingKeyRedeemBonusMode, "percent")
	cap := s.settingService.GetFloatSetting(ctx, SettingKeyRedeemBonusCap, 0)
	balanceType := s.settingService.GetStringSetting(ctx, SettingKeyRedeemBonusBalanceType, BalanceTypePermanent)
	expiryDays := s.settingService.GetIntSetting(ctx, SettingKeyRedeemBonusExpiryDays, 0)

	var bonusAmount float64
	switch mode {
	case "fixed":
		bonusAmount = s.settingService.GetFloatSetting(ctx, SettingKeyRedeemBonusFixedAmount, 1)
	case "percent":
		percent := s.settingService.GetFloatSetting(ctx, SettingKeyRedeemBonusPercent, 10)
		bonusAmount = redeemAmount * percent / 100
	case "random":
		rMin := s.settingService.GetIntSetting(ctx, SettingKeyRedeemBonusRandomMin, 1)
		rMax := s.settingService.GetIntSetting(ctx, SettingKeyRedeemBonusRandomMax, 10)
		if rMin > rMax {
			rMin, rMax = rMax, rMin
		}
		bonusAmount = float64(rMin + rand.Intn(rMax-rMin+1))
	default:
		return
	}

	if bonusAmount <= 0 {
		return
	}
	if cap > 0 && bonusAmount > cap {
		bonusAmount = cap
	}
	// 四舍五入到4位小数
	bonusAmount = math.Round(bonusAmount*10000) / 10000

	var expiresAt *time.Time
	if balanceType == BalanceTypeExpirable && expiryDays > 0 {
		t := time.Now().AddDate(0, 0, expiryDays)
		expiresAt = &t
	}

	if _, err := s.balanceEntryService.AddBalance(ctx, &AddBalanceInput{
		UserID:      userID,
		Amount:      bonusAmount,
		BalanceType: balanceType,
		Source:      BalanceSourceRedeemBonus,
		Note:        fmt.Sprintf("兑换加成 (模式=%s, 充值 $%.4f)", mode, redeemAmount),
		ExpiresAt:   expiresAt,
	}); err != nil {
		slog.Error("redeem_bonus: add balance failed", "user_id", userID, "bonus", bonusAmount, "error", err)
		return
	}

	slog.Info("redeem_bonus: applied", "user_id", userID, "redeem_amount", redeemAmount, "bonus", bonusAmount, "mode", mode)
}

// IsFirstBalanceRedeem 检查用户是否为首次余额兑换
// 调用时点：刚刚完成兑换后，所以只有1条余额兑换记录说明是首次
func (s *RedeemBonusService) IsFirstBalanceRedeem(ctx context.Context, userID int64, currentRedeemAmount float64) bool {
	totalRecharged, err := s.redeemRepo.SumPositiveBalanceByUser(ctx, userID)
	if err != nil {
		slog.Error("check first redeem: sum query failed", "user_id", userID, "error", err)
		return false
	}
	// 如果总充值金额等于本次充值金额（浮点比较用容差），说明是首次
	return math.Abs(totalRecharged-currentRedeemAmount) < 0.001
}
