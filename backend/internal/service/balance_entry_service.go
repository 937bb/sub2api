package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
)

// BalanceEntryService 余额明细服务
type BalanceEntryService struct {
	entClient        *dbent.Client
	balanceEntryRepo BalanceEntryRepository
	userRepo         UserRepository
	settingService   *SettingService
}

// NewBalanceEntryService 创建余额明细服务
func NewBalanceEntryService(
	entClient *dbent.Client,
	balanceEntryRepo BalanceEntryRepository,
	userRepo UserRepository,
	settingService *SettingService,
) *BalanceEntryService {
	return &BalanceEntryService{
		entClient:        entClient,
		balanceEntryRepo: balanceEntryRepo,
		userRepo:         userRepo,
		settingService:   settingService,
	}
}

// AddBalance 入账（充值/奖励/赠送等），创建 balance_entry 并同步更新 user.balance
func (s *BalanceEntryService) AddBalance(ctx context.Context, input *AddBalanceInput) (*BalanceEntry, error) {
	if input.Amount <= 0 {
		return nil, fmt.Errorf("add balance amount must be positive, got %.8f", input.Amount)
	}
	if input.BalanceType == BalanceTypeExpirable && input.ExpiresAt == nil {
		return nil, fmt.Errorf("expirable balance must have expires_at")
	}
	if input.BalanceType == "" {
		input.BalanceType = BalanceTypePermanent
	}

	entry := &BalanceEntry{
		UserID:      input.UserID,
		Amount:      input.Amount,
		Remaining:   input.Amount, // 入账时 remaining = amount
		BalanceType: input.BalanceType,
		Source:      input.Source,
		Note:        input.Note,
		ExpiresAt:   input.ExpiresAt,
	}

	if err := s.balanceEntryRepo.Create(ctx, entry); err != nil {
		return nil, fmt.Errorf("create balance entry: %w", err)
	}

	// 同步更新 user.balance 缓存字段
	if err := s.userRepo.UpdateBalance(ctx, input.UserID, input.Amount); err != nil {
		return nil, fmt.Errorf("sync user balance: %w", err)
	}

	return entry, nil
}

// DeductBalance 扣减余额，按先过期后永久的顺序扣减 balance_entries，并同步 user.balance
// 返回实际扣减金额。允许透支（当可用余额不足时仍继续扣减 user.balance）。
func (s *BalanceEntryService) DeductBalance(ctx context.Context, userID int64, amount float64, source, note string) (float64, error) {
	if amount <= 0 {
		return 0, nil
	}

	// 获取可用余额条目（已按 expires_at ASC NULLS LAST 排序）
	entries, err := s.balanceEntryRepo.GetAvailableEntries(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("get available entries: %w", err)
	}

	// 从条目中扣减
	deducted, err := s.balanceEntryRepo.DeductFromEntries(ctx, entries, amount)
	if err != nil {
		return 0, fmt.Errorf("deduct from entries: %w", err)
	}

	// 创建扣减记录（负数金额，remaining=0）
	deductEntry := &BalanceEntry{
		UserID:      userID,
		Amount:      -amount,
		Remaining:   0,
		BalanceType: BalanceTypePermanent,
		Source:      source,
		Note:        note,
	}
	if err := s.balanceEntryRepo.Create(ctx, deductEntry); err != nil {
		slog.Error("create deduct balance entry failed", "user_id", userID, "amount", amount, "error", err)
	}

	// 同步更新 user.balance（扣减 amount，允许透支）
	if err := s.userRepo.UpdateBalance(ctx, userID, -amount); err != nil {
		return deducted, fmt.Errorf("sync user balance: %w", err)
	}

	return deducted, nil
}

// RecordDeduction 仅记录扣减明细（不操作 user.balance，由调用方自行处理）
// 用于兼容现有已经直接操作 user.balance 的扣减流程
func (s *BalanceEntryService) RecordDeduction(ctx context.Context, userID int64, amount float64, source, note string) {
	if amount <= 0 {
		return
	}

	// 从可用条目中扣减 remaining
	entries, err := s.balanceEntryRepo.GetAvailableEntries(ctx, userID)
	if err != nil {
		slog.Error("record deduction: get available entries failed", "user_id", userID, "error", err)
	} else {
		if _, err := s.balanceEntryRepo.DeductFromEntries(ctx, entries, amount); err != nil {
			slog.Error("record deduction: deduct from entries failed", "user_id", userID, "error", err)
		}
	}

	// 创建扣减记录
	deductEntry := &BalanceEntry{
		UserID:      userID,
		Amount:      -amount,
		Remaining:   0,
		BalanceType: BalanceTypePermanent,
		Source:      source,
		Note:        note,
	}
	if err := s.balanceEntryRepo.Create(ctx, deductEntry); err != nil {
		slog.Error("record deduction: create entry failed", "user_id", userID, "error", err)
	}
}

// RecordAddition 仅记录入账明细（不操作 user.balance，由调用方自行处理）
// 用于兼容现有已经直接操作 user.balance 的入账流程
func (s *BalanceEntryService) RecordAddition(ctx context.Context, input *AddBalanceInput) {
	if input.Amount <= 0 {
		return
	}
	if input.BalanceType == "" {
		input.BalanceType = BalanceTypePermanent
	}

	entry := &BalanceEntry{
		UserID:      input.UserID,
		Amount:      input.Amount,
		Remaining:   input.Amount,
		BalanceType: input.BalanceType,
		Source:      input.Source,
		Note:        input.Note,
		ExpiresAt:   input.ExpiresAt,
	}
	if err := s.balanceEntryRepo.Create(ctx, entry); err != nil {
		slog.Error("record addition: create entry failed", "user_id", input.UserID, "error", err)
	}
}

// ExpireBalances 过期清理：将所有过期条目标记为 expired、清零 remaining，并同步 user.balance
// 返回受影响的用户 ID 列表
func (s *BalanceEntryService) ExpireBalances(ctx context.Context) ([]int64, error) {
	expired, err := s.balanceEntryRepo.ExpireEntries(ctx, time.Now())
	if err != nil {
		return nil, fmt.Errorf("expire entries: %w", err)
	}
	if len(expired) == 0 {
		return nil, nil
	}

	// 按用户聚合过期金额
	userExpired := make(map[int64]float64)
	for _, e := range expired {
		// ExpireEntries 返回的 remaining 是清零前的值（因为 UPDATE SET remaining=0 RETURNING...）
		// 实际上 RETURNING 返回的是更新后的值。我们需要用 amount 作为参考。
		// 但更准确的做法：expired entry 原来的 remaining 已被清零，
		// 我们在 ExpireEntries 中用 CTE 记录旧值。
		// 简化处理：重新从数据库计算用户正确余额。
		userExpired[e.UserID] = 0 // 标记需要重算
	}

	// 重算每个受影响用户的 balance 并同步
	affectedUsers := make([]int64, 0, len(userExpired))
	for uid := range userExpired {
		affectedUsers = append(affectedUsers, uid)
		newBalance, err := s.balanceEntryRepo.SumRemainingByUser(ctx, uid)
		if err != nil {
			slog.Error("expire: sum remaining failed", "user_id", uid, "error", err)
			continue
		}
		// 直接设置 user.balance（而不是增减），确保一致性
		user, err := s.userRepo.GetByID(ctx, uid)
		if err != nil {
			slog.Error("expire: get user failed", "user_id", uid, "error", err)
			continue
		}
		diff := newBalance - user.Balance
		if diff != 0 {
			if err := s.userRepo.UpdateBalance(ctx, uid, diff); err != nil {
				slog.Error("expire: sync user balance failed", "user_id", uid, "error", err)
			}
		}
	}

	return affectedUsers, nil
}

// GetUserBalanceSummary 获取用户余额汇总
func (s *BalanceEntryService) GetUserBalanceSummary(ctx context.Context, userID int64) (*BalanceSummary, error) {
	soonDays := 7 // 默认 7 天内即将过期
	if s.settingService != nil {
		// 从设置中读取预警天数（后续实现）
		_ = soonDays
	}
	return s.balanceEntryRepo.GetUserBalanceSummary(ctx, userID, soonDays)
}

// ListByUser 获取用户余额明细（分页）
func (s *BalanceEntryService) ListByUser(ctx context.Context, userID int64, page, pageSize int) ([]*BalanceEntry, int64, error) {
	offset := (page - 1) * pageSize
	return s.balanceEntryRepo.ListByUser(ctx, userID, offset, pageSize)
}

// SyncUserBalance 重算并同步用户余额（修复不一致场景）
func (s *BalanceEntryService) SyncUserBalance(ctx context.Context, userID int64) error {
	newBalance, err := s.balanceEntryRepo.SumRemainingByUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("sum remaining: %w", err)
	}
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}
	diff := newBalance - user.Balance
	if diff == 0 {
		return nil
	}
	return s.userRepo.UpdateBalance(ctx, userID, diff)
}
