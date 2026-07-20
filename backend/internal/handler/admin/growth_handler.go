package admin

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type GrowthHandler struct {
	service *service.GrowthService
}

func NewGrowthHandler(growthService *service.GrowthService) *GrowthHandler {
	return &GrowthHandler{service: growthService}
}

func (h *GrowthHandler) GetConfig(c *gin.Context) {
	config, err := h.service.GetConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, config)
}

func (h *GrowthHandler) UpdateConfig(c *gin.Context) {
	var req service.GrowthConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	updated, err := h.service.UpdateConfig(c.Request.Context(), req, subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, updated)
}

func (h *GrowthHandler) ListRewardLedger(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	items, total, err := h.service.ListRewardLedger(c.Request.Context(), page, pageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, total, page, pageSize)
}

func (h *GrowthHandler) ListRiskEvents(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	items, total, err := h.service.ListRiskEvents(c.Request.Context(), page, pageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, total, page, pageSize)
}

func (h *GrowthHandler) SettleLeaderboard(c *gin.Context) {
	period := strings.TrimSpace(c.Param("period"))
	count, total, err := h.service.SettlePreviousPeriod(c.Request.Context(), period)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"period": period, "rewarded_users": count, "total_reward": total})
}
