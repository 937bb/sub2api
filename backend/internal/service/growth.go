package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

const (
	GrowthRewardModeFixed  = "fixed"
	GrowthRewardModeRandom = "random"
)

var (
	ErrGrowthCheckinDisabled     = infraerrors.Forbidden("GROWTH_CHECKIN_DISABLED", "daily check-in is disabled")
	ErrGrowthAlreadyChecked      = infraerrors.Conflict("GROWTH_ALREADY_CHECKED_IN", "already checked in today")
	ErrGrowthRewardIneligible    = infraerrors.Forbidden("GROWTH_REWARD_INELIGIBLE", "account is not eligible for a check-in reward")
	ErrGrowthAccountInactive     = infraerrors.Forbidden("GROWTH_ACCOUNT_INACTIVE", "account is inactive")
	ErrGrowthAccountTooNew       = infraerrors.Forbidden("GROWTH_ACCOUNT_TOO_NEW", "account has not reached the minimum age")
	ErrGrowthRechargeTooLow      = infraerrors.Forbidden("GROWTH_RECHARGE_TOO_LOW", "lifetime real recharge is below the configured minimum")
	ErrGrowthRecentSpendTooLow   = infraerrors.Forbidden("GROWTH_RECENT_SPEND_TOO_LOW", "recent actual spend is below the configured minimum")
	ErrGrowthIdentityRisk        = infraerrors.Forbidden("GROWTH_IDENTITY_RISK", "network or device risk control rejected this check-in")
	ErrGrowthCheckinRewardCap    = infraerrors.Forbidden("GROWTH_CHECKIN_REWARD_CAP", "lifetime check-in reward cap has been reached")
	ErrGrowthTotalRewardCap      = infraerrors.Forbidden("GROWTH_TOTAL_REWARD_CAP", "lifetime growth reward cap has been reached")
	ErrGrowthLeaderboardDisabled = infraerrors.Forbidden("GROWTH_LEADERBOARD_DISABLED", "leaderboard is disabled")
)

type GrowthStreakReward struct {
	Days   int     `json:"days"`
	Amount float64 `json:"amount"`
}

type GrowthLeaderboardRewardRule struct {
	ID           string  `json:"id"`
	Enabled      bool    `json:"enabled"`
	Period       string  `json:"period"`
	RankStart    int     `json:"rank_start"`
	RankEnd      int     `json:"rank_end"`
	RewardAmount float64 `json:"reward_amount"`
}

type GrowthConfig struct {
	CheckinEnabled              bool                          `json:"checkin_enabled"`
	CheckinRewardMode           string                        `json:"checkin_reward_mode"`
	CheckinFixedReward          float64                       `json:"checkin_fixed_reward"`
	CheckinMinReward            float64                       `json:"checkin_min_reward"`
	CheckinMaxReward            float64                       `json:"checkin_max_reward"`
	CheckinStreakRewards        []GrowthStreakReward          `json:"checkin_streak_rewards"`
	CheckinMinAccountAgeDays    int                           `json:"checkin_min_account_age_days"`
	CheckinMinTotalRecharged    float64                       `json:"checkin_min_total_recharged"`
	CheckinMaxRewardPaidRatio   float64                       `json:"checkin_max_reward_paid_ratio"`
	MaxTotalRewardPaidRatio     float64                       `json:"max_total_reward_paid_ratio"`
	CheckinMinRecentSpend       float64                       `json:"checkin_min_recent_spend"`
	CheckinRecentSpendDays      int                           `json:"checkin_recent_spend_days"`
	CheckinMaxAccountsPerIP     int                           `json:"checkin_max_accounts_per_ip"`
	CheckinMaxAccountsPerDevice int                           `json:"checkin_max_accounts_per_device"`
	LeaderboardEnabled          bool                          `json:"leaderboard_enabled"`
	LeaderboardAnonymous        bool                          `json:"leaderboard_anonymous"`
	LeaderboardDisplayLimit     int                           `json:"leaderboard_display_limit"`
	LeaderboardRewardRules      []GrowthLeaderboardRewardRule `json:"leaderboard_reward_rules"`
	UpdatedAt                   time.Time                     `json:"updated_at"`
}

type GrowthPublicConfig struct {
	CheckinEnabled       bool                 `json:"checkin_enabled"`
	CheckinRewardMode    string               `json:"checkin_reward_mode"`
	CheckinFixedReward   float64              `json:"checkin_fixed_reward"`
	CheckinMinReward     float64              `json:"checkin_min_reward"`
	CheckinMaxReward     float64              `json:"checkin_max_reward"`
	CheckinStreakRewards []GrowthStreakReward `json:"checkin_streak_rewards"`
	LeaderboardEnabled   bool                 `json:"leaderboard_enabled"`
}

type GrowthCheckin struct {
	Date         string  `json:"date"`
	StreakDays   int     `json:"streak_days"`
	BaseReward   float64 `json:"base_reward"`
	StreakReward float64 `json:"streak_reward"`
	TotalReward  float64 `json:"total_reward"`
	BalanceAfter float64 `json:"balance_after"`
}

type GrowthCheckinStatus struct {
	Config          GrowthPublicConfig `json:"config"`
	Today           string             `json:"today"`
	CheckedInToday  bool               `json:"checked_in_today"`
	CurrentStreak   int                `json:"current_streak"`
	TotalCheckins   int                `json:"total_checkins"`
	MonthCheckins   []GrowthCheckin    `json:"month_checkins"`
	EligibilityHint string             `json:"eligibility_hint,omitempty"`
}

type GrowthCheckinClaim struct {
	UserID     int64
	Date       time.Time
	Now        time.Time
	BaseReward float64
	IPHash     string
	DeviceHash string
	Config     GrowthConfig
}

type GrowthLeaderboardItem struct {
	Rank          int     `json:"rank"`
	UserID        int64   `json:"user_id,omitempty"`
	DisplayName   string  `json:"display_name"`
	ActualCost    float64 `json:"actual_cost"`
	Requests      int64   `json:"requests"`
	IsCurrentUser bool    `json:"is_current_user"`
}

type GrowthLeaderboard struct {
	Period      string                  `json:"period"`
	PeriodStart time.Time               `json:"period_start"`
	PeriodEnd   time.Time               `json:"period_end"`
	Items       []GrowthLeaderboardItem `json:"items"`
	CurrentUser *GrowthLeaderboardItem  `json:"current_user,omitempty"`
	Total       int64                   `json:"total"`
	Page        int                     `json:"page"`
	PageSize    int                     `json:"page_size"`
	Pages       int                     `json:"pages"`
}

type GrowthRewardLedgerItem struct {
	ID           int64          `json:"id"`
	UserID       int64          `json:"user_id"`
	Email        string         `json:"email"`
	SourceType   string         `json:"source_type"`
	SourceKey    string         `json:"source_key"`
	Amount       float64        `json:"amount"`
	BalanceAfter float64        `json:"balance_after"`
	Metadata     map[string]any `json:"metadata"`
	CreatedAt    time.Time      `json:"created_at"`
}

type GrowthRiskEvent struct {
	ID         int64          `json:"id"`
	UserID     *int64         `json:"user_id,omitempty"`
	Email      string         `json:"email"`
	Decision   string         `json:"decision"`
	ReasonCode string         `json:"reason_code"`
	Evidence   map[string]any `json:"evidence"`
	CreatedAt  time.Time      `json:"created_at"`
}

type GrowthRepository interface {
	GetConfig(ctx context.Context) (*GrowthConfig, error)
	UpdateConfig(ctx context.Context, config GrowthConfig, updatedBy int64) (*GrowthConfig, error)
	GetCheckinStatus(ctx context.Context, userID int64, monthStart, monthEnd, today time.Time) (*GrowthCheckinStatus, error)
	ClaimCheckin(ctx context.Context, claim GrowthCheckinClaim) (*GrowthCheckin, error)
	GetLeaderboard(ctx context.Context, start, end time.Time, currentUserID int64, limit int, anonymous bool) (*GrowthLeaderboard, error)
	SettleLeaderboard(ctx context.Context, period string, start, end time.Time, rules []GrowthLeaderboardRewardRule, maxTotalRewardPaidRatio float64) ([]int64, float64, error)
	ListRewardLedger(ctx context.Context, page, pageSize int) ([]GrowthRewardLedgerItem, int64, error)
	ListRiskEvents(ctx context.Context, page, pageSize int) ([]GrowthRiskEvent, int64, error)
}

type GrowthService struct {
	repo                GrowthRepository
	billingCacheService *BillingCacheService
}

func NewGrowthService(repo GrowthRepository, billingCacheService *BillingCacheService) *GrowthService {
	return &GrowthService{repo: repo, billingCacheService: billingCacheService}
}

func (s *GrowthService) GetConfig(ctx context.Context) (*GrowthConfig, error) {
	return s.repo.GetConfig(ctx)
}

func (s *GrowthService) UpdateConfig(ctx context.Context, config GrowthConfig, updatedBy int64) (*GrowthConfig, error) {
	if err := ValidateGrowthConfig(&config); err != nil {
		return nil, infraerrors.BadRequest("GROWTH_CONFIG_INVALID", err.Error())
	}
	return s.repo.UpdateConfig(ctx, config, updatedBy)
}

func (s *GrowthService) GetCheckinStatus(ctx context.Context, userID int64, month time.Time) (*GrowthCheckinStatus, error) {
	config, err := s.repo.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	now := timezone.Now()
	local := now.In(timezone.Location())
	if !month.IsZero() {
		local = time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, timezone.Location())
	}
	monthStartLocal := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, timezone.Location())
	monthEndLocal := monthStartLocal.AddDate(0, 1, 0)
	status, err := s.repo.GetCheckinStatus(ctx, userID, monthStartLocal, monthEndLocal, dateOnly(now))
	if err != nil {
		return nil, err
	}
	status.Config = publicGrowthConfig(*config)
	status.Today = dateOnly(now).Format("2006-01-02")
	return status, nil
}

func (s *GrowthService) Checkin(ctx context.Context, userID int64, ipHash, deviceHash string) (*GrowthCheckin, error) {
	config, err := s.repo.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	if !config.CheckinEnabled {
		return nil, ErrGrowthCheckinDisabled
	}
	baseReward, err := growthRewardAmount(*config)
	if err != nil {
		return nil, err
	}
	now := timezone.Now()
	result, err := s.repo.ClaimCheckin(ctx, GrowthCheckinClaim{
		UserID: userID, Date: dateOnly(now), Now: now.UTC(), BaseReward: baseReward,
		IPHash: ipHash, DeviceHash: deviceHash, Config: *config,
	})
	if err != nil {
		return nil, err
	}
	if s.billingCacheService != nil {
		_ = s.billingCacheService.InvalidateUserBalance(ctx, userID)
	}
	return result, nil
}

func (s *GrowthService) GetLeaderboard(ctx context.Context, period string, currentUserID int64) (*GrowthLeaderboard, error) {
	config, err := s.repo.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	if !config.LeaderboardEnabled {
		return nil, ErrGrowthLeaderboardDisabled
	}
	start, end, normalized, err := growthPeriodBounds(period, timezone.Now(), false)
	if err != nil {
		return nil, infraerrors.BadRequest("GROWTH_PERIOD_INVALID", err.Error())
	}
	result, err := s.repo.GetLeaderboard(ctx, start, end, currentUserID, config.LeaderboardDisplayLimit, config.LeaderboardAnonymous)
	if result != nil {
		result.Period = normalized
		result.PeriodStart = start
		result.PeriodEnd = end
	}
	return result, err
}

func (s *GrowthService) SettlePreviousPeriod(ctx context.Context, period string) (int, float64, error) {
	config, err := s.repo.GetConfig(ctx)
	if err != nil {
		return 0, 0, err
	}
	start, end, normalized, err := growthPeriodBounds(period, timezone.Now(), true)
	if err != nil {
		return 0, 0, err
	}
	if !config.LeaderboardEnabled {
		return 0, 0, nil
	}
	rules := make([]GrowthLeaderboardRewardRule, 0)
	for _, rule := range config.LeaderboardRewardRules {
		if rule.Enabled && rule.Period == normalized {
			rules = append(rules, rule)
		}
	}
	if len(rules) == 0 {
		return 0, 0, nil
	}
	userIDs, total, err := s.repo.SettleLeaderboard(ctx, normalized, start, end, rules, config.MaxTotalRewardPaidRatio)
	if err == nil && s.billingCacheService != nil {
		for _, userID := range userIDs {
			_ = s.billingCacheService.InvalidateUserBalance(ctx, userID)
		}
	}
	return len(userIDs), total, err
}

func (s *GrowthService) ListRewardLedger(ctx context.Context, page, pageSize int) ([]GrowthRewardLedgerItem, int64, error) {
	return s.repo.ListRewardLedger(ctx, page, pageSize)
}

func (s *GrowthService) ListRiskEvents(ctx context.Context, page, pageSize int) ([]GrowthRiskEvent, int64, error) {
	return s.repo.ListRiskEvents(ctx, page, pageSize)
}

func ValidateGrowthConfig(config *GrowthConfig) error {
	if config == nil {
		return errors.New("config is required")
	}
	if config.CheckinRewardMode != GrowthRewardModeFixed && config.CheckinRewardMode != GrowthRewardModeRandom {
		return errors.New("checkin_reward_mode must be fixed or random")
	}
	amounts := []float64{config.CheckinFixedReward, config.CheckinMinReward, config.CheckinMaxReward, config.CheckinMinTotalRecharged, config.CheckinMinRecentSpend}
	for _, amount := range amounts {
		if math.IsNaN(amount) || math.IsInf(amount, 0) || amount < 0 || amount > 1000000 {
			return errors.New("reward and spend amounts must be finite and non-negative")
		}
	}
	if config.CheckinFixedReward > 100 || config.CheckinMinReward > 100 || config.CheckinMaxReward > 100 {
		return errors.New("daily check-in rewards cannot exceed 100")
	}
	if !hasCentPrecision(config.CheckinFixedReward) || !hasCentPrecision(config.CheckinMinReward) || !hasCentPrecision(config.CheckinMaxReward) {
		return errors.New("check-in reward amounts must have at most two decimal places")
	}
	if config.CheckinMaxReward < config.CheckinMinReward {
		return errors.New("checkin_max_reward must be greater than or equal to checkin_min_reward")
	}
	if config.CheckinMinAccountAgeDays < 0 || config.CheckinRecentSpendDays < 1 || config.CheckinRecentSpendDays > 365 {
		return errors.New("invalid account age or recent spend window")
	}
	if config.CheckinMaxAccountsPerIP < 1 || config.CheckinMaxAccountsPerDevice < 1 {
		return errors.New("IP and device account limits must be at least 1")
	}
	if math.IsNaN(config.CheckinMaxRewardPaidRatio) || math.IsInf(config.CheckinMaxRewardPaidRatio, 0) || config.CheckinMaxRewardPaidRatio <= 0 || config.CheckinMaxRewardPaidRatio > 1 {
		return errors.New("check-in reward to paid amount ratio must be within 0-1")
	}
	if math.IsNaN(config.MaxTotalRewardPaidRatio) || math.IsInf(config.MaxTotalRewardPaidRatio, 0) || config.MaxTotalRewardPaidRatio <= 0 || config.MaxTotalRewardPaidRatio > 1 {
		return errors.New("total growth reward to paid amount ratio must be within 0-1")
	}
	if config.LeaderboardDisplayLimit < 1 || config.LeaderboardDisplayLimit > 100 {
		return errors.New("leaderboard_display_limit must be within 1-100")
	}
	seenDays := map[int]struct{}{}
	if len(config.CheckinStreakRewards) > 100 {
		return errors.New("too many streak rewards")
	}
	for _, reward := range config.CheckinStreakRewards {
		if reward.Days < 2 || reward.Days > 365 || reward.Amount <= 0 || reward.Amount > 100 || !hasCentPrecision(reward.Amount) {
			return errors.New("invalid streak reward")
		}
		if _, exists := seenDays[reward.Days]; exists {
			return fmt.Errorf("duplicate streak reward day %d", reward.Days)
		}
		seenDays[reward.Days] = struct{}{}
	}
	seenRuleIDs := map[string]struct{}{}
	if len(config.LeaderboardRewardRules) > 100 {
		return errors.New("too many leaderboard reward rules")
	}
	for i := range config.LeaderboardRewardRules {
		rule := &config.LeaderboardRewardRules[i]
		rule.ID = strings.TrimSpace(rule.ID)
		if rule.ID == "" || len(rule.ID) > 64 {
			return errors.New("leaderboard reward rule id is required and must be at most 64 characters")
		}
		if _, exists := seenRuleIDs[rule.ID]; exists {
			return fmt.Errorf("duplicate leaderboard reward rule id %q", rule.ID)
		}
		seenRuleIDs[rule.ID] = struct{}{}
		if rule.Period != "daily" && rule.Period != "weekly" && rule.Period != "monthly" {
			return errors.New("leaderboard reward period must be daily, weekly, or monthly")
		}
		if rule.RankStart < 1 || rule.RankEnd < rule.RankStart || rule.RankEnd > 100 {
			return errors.New("leaderboard reward rank range must be within 1-100")
		}
		if math.IsNaN(rule.RewardAmount) || math.IsInf(rule.RewardAmount, 0) || rule.RewardAmount <= 0 || rule.RewardAmount > 1000000 || !hasCentPrecision(rule.RewardAmount) {
			return errors.New("leaderboard reward amount must be positive")
		}
	}
	for i, left := range config.LeaderboardRewardRules {
		if !left.Enabled {
			continue
		}
		for _, right := range config.LeaderboardRewardRules[i+1:] {
			if right.Enabled && left.Period == right.Period && left.RankStart <= right.RankEnd && right.RankStart <= left.RankEnd {
				return fmt.Errorf("overlapping leaderboard rank ranges for %s", left.Period)
			}
		}
	}
	sort.Slice(config.CheckinStreakRewards, func(i, j int) bool { return config.CheckinStreakRewards[i].Days < config.CheckinStreakRewards[j].Days })
	return nil
}

func publicGrowthConfig(config GrowthConfig) GrowthPublicConfig {
	return GrowthPublicConfig{
		CheckinEnabled:     config.CheckinEnabled,
		CheckinRewardMode:  config.CheckinRewardMode,
		CheckinFixedReward: config.CheckinFixedReward, CheckinMinReward: config.CheckinMinReward,
		CheckinMaxReward: config.CheckinMaxReward, CheckinStreakRewards: config.CheckinStreakRewards,
		LeaderboardEnabled: config.LeaderboardEnabled,
	}
}

func growthRewardAmount(config GrowthConfig) (float64, error) {
	if config.CheckinRewardMode == GrowthRewardModeFixed {
		return roundCents(config.CheckinFixedReward), nil
	}
	minCents := int64(math.Round(config.CheckinMinReward * 100))
	maxCents := int64(math.Round(config.CheckinMaxReward * 100))
	if maxCents < minCents {
		return 0, errors.New("invalid random reward range")
	}
	span := maxCents - minCents + 1
	randomOffset, err := rand.Int(rand.Reader, big.NewInt(span))
	if err != nil {
		return 0, fmt.Errorf("generate reward: %w", err)
	}
	value := randomOffset.Int64() + minCents
	return float64(value) / 100, nil
}

func roundCents(value float64) float64 { return math.Round(value*100) / 100 }

func hasCentPrecision(value float64) bool {
	return math.Abs(value*100-math.Round(value*100)) < 1e-7
}

func dateOnly(value time.Time) time.Time {
	local := value.In(timezone.Location())
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, timezone.Location())
}

func growthPeriodBounds(period string, now time.Time, previous bool) (time.Time, time.Time, string, error) {
	local := now.In(timezone.Location())
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, timezone.Location())
	period = strings.ToLower(strings.TrimSpace(period))
	var start, end time.Time
	switch period {
	case "daily", "day":
		period = "daily"
		start, end = day, day.AddDate(0, 0, 1)
	case "weekly", "week":
		period = "weekly"
		offset := (int(day.Weekday()) + 6) % 7
		start = day.AddDate(0, 0, -offset)
		end = start.AddDate(0, 0, 7)
	case "monthly", "month":
		period = "monthly"
		start = time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, timezone.Location())
		end = start.AddDate(0, 1, 0)
	default:
		return time.Time{}, time.Time{}, "", errors.New("period must be daily, weekly, or monthly")
	}
	if previous {
		end = start
		switch period {
		case "daily":
			start = end.AddDate(0, 0, -1)
		case "weekly":
			start = end.AddDate(0, 0, -7)
		case "monthly":
			start = end.AddDate(0, -1, 0)
		}
	}
	return start.UTC(), end.UTC(), period, nil
}
