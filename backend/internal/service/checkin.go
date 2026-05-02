package service

import (
	"context"
	"time"
)

// CheckinRecord 签到记录
type CheckinRecord struct {
	ID              int64
	UserID          int64
	CheckinDate     time.Time // 签到日期 (date only)
	Streak          int       // 连续签到天数（含当天）
	BaseAmount      float64   // 基础奖励
	MilestoneAmount float64   // 里程碑奖励
	TotalAmount     float64   // 总奖励
	BalanceType     string    // permanent / expirable
	CreatedAt       time.Time
}

// CheckinMilestone 里程碑规则
type CheckinMilestone struct {
	Days        int     `json:"days"`         // 连续签到满N天
	Amount      float64 `json:"amount"`       // 奖励金额
	BalanceType string  `json:"balance_type"` // permanent / expirable
	ExpiryDays  int     `json:"expiry_days"`  // 有效期天数（仅 expirable 生效）
}

// CheckinConfig 签到配置
type CheckinConfig struct {
	Enabled     bool                `json:"enabled"`
	Mode        string              `json:"mode"`         // fixed / random
	FixedAmount float64             `json:"fixed_amount"`
	RandomMin   float64             `json:"random_min"`
	RandomMax   float64             `json:"random_max"`
	BalanceType string              `json:"balance_type"` // permanent / expirable
	ExpiryDays  int                 `json:"expiry_days"`  // 有效期天数（仅 expirable 生效）
	Milestones  []CheckinMilestone  `json:"milestones"`
}

// CheckinStatus 用户签到状态
type CheckinStatus struct {
	CheckedInToday bool   `json:"checked_in_today"`
	CurrentStreak  int    `json:"current_streak"`
	TotalCheckins  int64  `json:"total_checkins"`
	LastCheckinDate string `json:"last_checkin_date,omitempty"`
}

// CheckinRepository 签到仓储接口
type CheckinRepository interface {
	// Create 创建签到记录
	Create(ctx context.Context, record *CheckinRecord) error
	// GetByUserAndDate 按用户和日期查询签到记录
	GetByUserAndDate(ctx context.Context, userID int64, date time.Time) (*CheckinRecord, error)
	// GetLatestByUser 获取用户最近一条签到记录
	GetLatestByUser(ctx context.Context, userID int64) (*CheckinRecord, error)
	// ListByUser 查询用户签到历史（分页）
	ListByUser(ctx context.Context, userID int64, offset, limit int) ([]*CheckinRecord, int64, error)
	// CountByUser 统计用户总签到次数
	CountByUser(ctx context.Context, userID int64) (int64, error)
	// GetMonthlyRecords 获取用户某月的签到记录（日历展示用）
	GetMonthlyRecords(ctx context.Context, userID int64, year int, month int) ([]*CheckinRecord, error)
}
