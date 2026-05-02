package service

import (
	"context"
	"time"
)

// BalanceType 余额类型
const (
	BalanceTypePermanent = "permanent" // 永久余额
	BalanceTypeExpirable = "expirable" // 有效期余额
)

// BalanceSource 余额来源
const (
	BalanceSourceRecharge        = "recharge"          // 在线充值
	BalanceSourceRedeem          = "redeem"            // 兑换码充值
	BalanceSourceCheckin         = "checkin"           // 每日签到
	BalanceSourceLeaderboard     = "leaderboard"       // 排行榜奖励
	BalanceSourceAdmin           = "admin"             // 管理员调整
	BalanceSourcePromo           = "promo"             // 优惠码赠送
	BalanceSourceAffiliate       = "affiliate"         // 邀请返利
	BalanceSourceFirstRedeemBonus = "first_redeem_bonus" // 首次兑换加成
	BalanceSourceRedeemBonus     = "redeem_bonus"      // 常规兑换加成
	BalanceSourceCashback        = "cashback"          // 消费返现
	BalanceSourceOAuthGrant      = "oauth_grant"       // OAuth 首绑赠送
	BalanceSourceConsumption     = "consumption"       // API 消费扣减
	BalanceSourceExpiryClear     = "expiry_clear"      // 过期清理
	BalanceSourceRefund          = "refund"            // 退款
)

// BalanceEntry 余额明细条目
type BalanceEntry struct {
	ID          int64
	UserID      int64
	Amount      float64   // 变动金额（正=入账，负=扣减记录）
	Remaining   float64   // 该笔入账余额剩余可用金额
	BalanceType string    // permanent | expirable
	Source      string    // 来源类型
	Note        string    // 描述/备注
	ExpiresAt   *time.Time // 过期时间（expirable 类型时有值）
	Expired     bool      // 是否已被过期清理
	CreatedAt   time.Time
}

// BalanceEntryRepository 余额明细仓储接口
type BalanceEntryRepository interface {
	// Create 创建一条余额明细
	Create(ctx context.Context, entry *BalanceEntry) error
	// ListByUser 查询用户余额明细（分页，按创建时间倒序）
	// excludeSources: 排除指定来源的记录（为空则不排除）
	ListByUser(ctx context.Context, userID int64, offset, limit int, excludeSources ...string) ([]*BalanceEntry, int64, error)
	// ListByUserFiltered 查询用户余额明细（分页），支持按来源包含过滤
	ListByUserFiltered(ctx context.Context, userID int64, offset, limit int, includeSources []string) ([]*BalanceEntry, int64, error)
	// GetAvailableEntries 获取用户所有可用余额条目（remaining > 0 且未过期），按过期时间升序（NULLS LAST）
	GetAvailableEntries(ctx context.Context, userID int64) ([]*BalanceEntry, error)
	// DeductFromEntries 从指定条目列表中扣减指定金额，返回实际扣减的总金额
	// 遵循先扣即将过期后扣永久的顺序（由调用方排好序传入）
	DeductFromEntries(ctx context.Context, entries []*BalanceEntry, amount float64) (float64, error)
	// ExpireEntries 将过期的条目标记为 expired 并清零 remaining，返回受影响的条目列表
	ExpireEntries(ctx context.Context, before time.Time) ([]*BalanceEntry, error)
	// SumRemainingByUser 计算用户所有未过期条目的 remaining 总和
	SumRemainingByUser(ctx context.Context, userID int64) (float64, error)
	// GetUserBalanceSummary 获取用户余额汇总（永久余额/有效期余额/即将过期余额）
	GetUserBalanceSummary(ctx context.Context, userID int64, soonDays int) (*BalanceSummary, error)
}

// BalanceSummary 用户余额汇总
type BalanceSummary struct {
	TotalBalance     float64 // 总可用余额
	PermanentBalance float64 // 永久余额
	ExpirableBalance float64 // 有效期余额
	ExpiringSoon     float64 // 即将过期余额（N天内）
}

// AddBalanceInput 入账请求参数
type AddBalanceInput struct {
	UserID      int64
	Amount      float64
	BalanceType string     // permanent | expirable
	Source      string     // 来源
	Note        string     // 备注
	ExpiresAt   *time.Time // 过期时间（expirable 必填）
}
