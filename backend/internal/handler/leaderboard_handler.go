package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// LeaderboardHandler 排行榜 handler
type LeaderboardHandler struct {
	leaderboardService *service.LeaderboardService
	settingService     *service.SettingService
}

// NewLeaderboardHandler creates a LeaderboardHandler
func NewLeaderboardHandler(leaderboardService *service.LeaderboardService, settingService *service.SettingService) *LeaderboardHandler {
	return &LeaderboardHandler{
		leaderboardService: leaderboardService,
		settingService:     settingService,
	}
}

// GetLeaderboard 获取排行榜数据
// GET /api/v1/user/leaderboard
func (h *LeaderboardHandler) GetLeaderboard(c *gin.Context) {
	ctx := c.Request.Context()

	if !h.settingService.GetBoolSetting(ctx, service.SettingKeyLeaderboardEnabled, false) {
		response.Success(c, gin.H{
			"enabled":      false,
			"today":        []any{},
			"yesterday":    []any{},
			"total":        []any{},
			"reward_rules": []any{},
		})
		return
	}

	data, err := h.leaderboardService.GetLeaderboard(ctx)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	// 解析奖励规则供前端展示
	rewardRules := h.leaderboardService.GetRewardRulesForDisplay(ctx)

	response.Success(c, gin.H{
		"enabled":      true,
		"today":        data.Today,
		"yesterday":    data.Yesterday,
		"total":        data.Total,
		"reward_rules": rewardRules,
	})
}
