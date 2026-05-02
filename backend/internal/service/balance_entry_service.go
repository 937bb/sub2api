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
// TotalBalance = user.balance (真实余额)
// PermanentBalance = user.balance - expirableBalance (永久部分)
// ExpirableBalance / ExpiringSoon 来自 balance_entries 表
func (s *BalanceEntryService) GetUserBalanceSummary(ctx context.Context, userID int64) (*BalanceSummary, error) {
	soonDays := 7
	if s.settingService != nil {
		_ = soonDays
	}

	// 从 balance_entries 获取有效期余额统计
	entrySummary, err := s.balanceEntryRepo.GetUserBalanceSummary(ctx, userID, soonDays)
	if err != nil {
		return nil, err
	}

	// 从 user 表获取真实余额
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user for summary: %w", err)
	}

	// 永久余额 = 用户余额 - 有效期余额（不低于0）
	permanentBalance := user.Balance - entrySummary.ExpirableBalance
	if permanentBalance < 0 {
		permanentBalance = 0
	}

	return &BalanceSummary{
		TotalBalance:     user.Balance,
		PermanentBalance: permanentBalance,
		ExpirableBalance: entrySummary.ExpirableBalance,
		ExpiringSoon:     entrySummary.ExpiringSoon,
	}, nil
}

// ListByUser 获取用户余额明细（分页）
// excludeSources: 排除指定来源的记录
func (s *BalanceEntryService) ListByUser(ctx context.Context, userID int64, page, pageSize int, excludeSources ...string) ([]*BalanceEntry, int64, error) {
	offset := (page - 1) * pageSize
	return s.balanceEntryRepo.ListByUser(ctx, userID, offset, pageSize, excludeSources...)
}

// ListByUserFiltered 获取用户余额明细（分页），按来源过滤
func (s *BalanceEntryService) ListByUserFiltered(ctx context.Context, userID int64, page, pageSize int, includeSources []string) ([]*BalanceEntry, int64, error) {
	offset := (page - 1) * pageSize
	return s.balanceEntryRepo.ListByUserFiltered(ctx, userID, offset, pageSize, includeSources)
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

// MigrateRedeemCodesToBalanceEntries 将 redeem_codes 中已使用的余额记录回填到 balance_entries
// 跳过已经存在对应 balance_entry 的记录（通过 note LIKE '%兑换码%' 或 source='redeem'/'admin' 判断）
// 返回迁移的条数
func (s *BalanceEntryService) MigrateRedeemCodesToBalanceEntries(ctx context.Context) (int, error) {
	client := s.entClient
	if client == nil {
		return 0, fmt.Errorf("ent client not available")
	}

	// 使用原生 SQL 批量插入：从 redeem_codes 中选出已使用的余额类型记录，
	// 排除已经被迁移过的（通过检查 balance_entries 中是否已有同 user_id + 相似 note 的记录）
	query := `
INSERT INTO balance_entries (user_id, amount, remaining, balance_type, source, note, expired, created_at)
SELECT
    rc.used_by,
    rc.value,
    CASE WHEN rc.value > 0 THEN rc.value ELSE 0 END,
    'permanent',
    CASE
        WHEN rc.type = 'admin_balance' THEN 'admin'
        ELSE 'redeem'
    END,
    CASE
        WHEN rc.type = 'admin_balance' THEN COALESCE(rc.notes, '管理员调整(历史迁移)')
        ELSE '兑换码充值(历史迁移) ' || LEFT(rc.code, 8)
    END,
    FALSE,
    COALESCE(rc.used_at, rc.created_at)
FROM redeem_codes rc
WHERE rc.status = 'used'
  AND rc.used_by IS NOT NULL
  AND rc.type IN ('balance', 'admin_balance')
  AND NOT EXISTS (
      SELECT 1 FROM balance_entries be
      WHERE be.user_id = rc.used_by
        AND be.amount = rc.value
        AND be.created_at = COALESCE(rc.used_at, rc.created_at)
  )`

	result, err := client.ExecContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("migrate redeem codes: %w", err)
	}
	affected, _ := result.RowsAffected()

	slog.Info("migrate_redeem_codes: completed", "migrated", affected)
	return int(affected), nil
}
