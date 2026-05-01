package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

// LeaderboardRewardRule 排行榜奖励规则
type LeaderboardRewardRule struct {
	Rank        int     `json:"rank"`         // 名次（从 1 开始）
	Mode        string  `json:"mode"`         // fixed / percent（percent = 昨日消费百分比）
	Amount      float64 `json:"amount"`       // 金额或百分比
	BalanceType string  `json:"balance_type"` // permanent / expirable
	ExpiryDays  int     `json:"expiry_days"`  // 有效期天数
}

// LeaderboardEntry 排行榜条目（面向前端）
type LeaderboardEntry struct {
	Rank       int     `json:"rank"`
	Email      string  `json:"email"`
	ActualCost float64 `json:"actual_cost"`
	Requests   int64   `json:"requests"`
	Tokens     int64   `json:"tokens"`
}

// LeaderboardResponse 排行榜响应
type LeaderboardResponse struct {
	Yesterday []LeaderboardEntry `json:"yesterday"`
	Total     []LeaderboardEntry `json:"total"`
}

// LeaderboardService 排行榜 + 奖励服务
type LeaderboardService struct {
	settingService      *SettingService
	balanceEntryService *BalanceEntryService
	usageLogRepo        UsageLogRepository
}

// NewLeaderboardService creates a LeaderboardService
func NewLeaderboardService(
	settingService *SettingService,
	balanceEntryService *BalanceEntryService,
	usageLogRepo UsageLogRepository,
) *LeaderboardService {
	return &LeaderboardService{
		settingService:      settingService,
		balanceEntryService: balanceEntryService,
		usageLogRepo:        usageLogRepo,
	}
}

// GetLeaderboard 获取排行榜数据（昨日+总计）
func (s *LeaderboardService) GetLeaderboard(ctx context.Context) (*LeaderboardResponse, error) {
	if !s.settingService.GetBoolSetting(ctx, SettingKeyLeaderboardEnabled, false) {
		return &LeaderboardResponse{Yesterday: []LeaderboardEntry{}, Total: []LeaderboardEntry{}}, nil
	}

	topN := s.settingService.GetIntSetting(ctx, SettingKeyLeaderboardTopN, 10)
	maskEmail := s.settingService.GetBoolSetting(ctx, SettingKeyLeaderboardMaskEmail, true)

	now := time.Now()
	yesterdayStart := time.Date(now.Year(), now.Month(), now.Day()-1, 0, 0, 0, 0, now.Location())
	yesterdayEnd := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	// 昨日排行
	yesterdayRanking, err := s.usageLogRepo.GetUserSpendingRanking(ctx, yesterdayStart, yesterdayEnd, topN)
	if err != nil {
		return nil, fmt.Errorf("get yesterday ranking: %w", err)
	}

	// 总排行（最近30天）
	totalStart := now.AddDate(0, 0, -30)
	totalRanking, err := s.usageLogRepo.GetUserSpendingRanking(ctx, totalStart, now, topN)
	if err != nil {
		return nil, fmt.Errorf("get total ranking: %w", err)
	}

	resp := &LeaderboardResponse{
		Yesterday: s.rankingToEntries(yesterdayRanking, maskEmail),
		Total:     s.rankingToEntries(totalRanking, maskEmail),
	}
	return resp, nil
}

func (s *LeaderboardService) rankingToEntries(ranking *usagestats.UserSpendingRankingResponse, maskEmail bool) []LeaderboardEntry {
	if ranking == nil || len(ranking.Ranking) == 0 {
		return []LeaderboardEntry{}
	}
	entries := make([]LeaderboardEntry, 0, len(ranking.Ranking))
	for i, r := range ranking.Ranking {
		email := r.Email
		if maskEmail {
			email = maskEmailStr(email)
		}
		entries = append(entries, LeaderboardEntry{
			Rank:       i + 1,
			Email:      email,
			ActualCost: math.Round(r.ActualCost*10000) / 10000,
			Requests:   r.Requests,
			Tokens:     r.Tokens,
		})
	}
	return entries
}

// maskEmailStr 脱敏邮箱：a***@example.com
func maskEmailStr(email string) string {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		if len(email) <= 2 {
			return "***"
		}
		return email[:1] + "***" + email[len(email)-1:]
	}
	local := parts[0]
	if len(local) <= 1 {
		return local + "***@" + parts[1]
	}
	return local[:1] + "***@" + parts[1]
}

// RunDailyReward 每日排行榜奖励发放（由定时任务调用）
func (s *LeaderboardService) RunDailyReward(ctx context.Context) {
	if !s.settingService.GetBoolSetting(ctx, SettingKeyLeaderboardEnabled, false) {
		return
	}

	rulesJSON := s.settingService.GetStringSetting(ctx, SettingKeyLeaderboardRewardRules, "[]")
	var rules []LeaderboardRewardRule
	if err := json.Unmarshal([]byte(rulesJSON), &rules); err != nil {
		slog.Error("leaderboard_reward: parse rules failed", "error", err)
		return
	}
	if len(rules) == 0 {
		return
	}

	// 昨日排行
	now := time.Now()
	yesterdayStart := time.Date(now.Year(), now.Month(), now.Day()-1, 0, 0, 0, 0, now.Location())
	yesterdayEnd := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	maxRank := 0
	for _, r := range rules {
		if r.Rank > maxRank {
			maxRank = r.Rank
		}
	}

	ranking, err := s.usageLogRepo.GetUserSpendingRanking(ctx, yesterdayStart, yesterdayEnd, maxRank)
	if err != nil {
		slog.Error("leaderboard_reward: get ranking failed", "error", err)
		return
	}
	if ranking == nil || len(ranking.Ranking) == 0 {
		return
	}

	// 按名次发奖
	for _, rule := range rules {
		idx := rule.Rank - 1
		if idx < 0 || idx >= len(ranking.Ranking) {
			continue
		}
		item := ranking.Ranking[idx]

		var bonusAmount float64
		switch rule.Mode {
		case "fixed":
			bonusAmount = rule.Amount
		case "percent":
			bonusAmount = item.ActualCost * rule.Amount / 100
		default:
			continue
		}
		if bonusAmount <= 0 {
			continue
		}
		bonusAmount = math.Round(bonusAmount*10000) / 10000

		balanceType := rule.BalanceType
		if balanceType == "" {
			balanceType = BalanceTypePermanent
		}
		var expiresAt *time.Time
		if balanceType == BalanceTypeExpirable && rule.ExpiryDays > 0 {
			t := time.Now().AddDate(0, 0, rule.ExpiryDays)
			expiresAt = &t
		}

		if _, err := s.balanceEntryService.AddBalance(ctx, &AddBalanceInput{
			UserID:      item.UserID,
			Amount:      bonusAmount,
			BalanceType: balanceType,
			Source:      BalanceSourceLeaderboard,
			Note:        fmt.Sprintf("排行榜第%d名奖励 (昨日消费 $%.4f)", rule.Rank, item.ActualCost),
			ExpiresAt:   expiresAt,
		}); err != nil {
			slog.Error("leaderboard_reward: add balance failed", "user_id", item.UserID, "rank", rule.Rank, "bonus", bonusAmount, "error", err)
			continue
		}
		slog.Info("leaderboard_reward: applied", "user_id", item.UserID, "rank", rule.Rank, "bonus", bonusAmount)
	}
}
