package admin

import (
	"errors"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type AntiDegradeHandler struct {
	service *service.AntiDegradeService
}

func NewAntiDegradeHandler(svc *service.AntiDegradeService) *AntiDegradeHandler {
	return &AntiDegradeHandler{service: svc}
}

func (h *AntiDegradeHandler) requireService(c *gin.Context) bool {
	role, ok := middleware.GetUserRoleFromContext(c)
	if !ok || role != service.RoleAdmin {
		response.Forbidden(c, "Admin access required")
		return false
	}
	if h == nil || h.service == nil {
		response.ErrorFrom(c, errors.New("anti-degrade service unavailable"))
		return false
	}
	return true
}

func antiDegradeID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return 0, false
	}
	return id, true
}

// Strategies returns the server-owned, data-driven strategy registry.  It is
// read-only and intentionally available only to administrators.
func (h *AntiDegradeHandler) Strategies(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	response.Success(c, gin.H{"strategies": service.ListAntiDegradeStrategyProfiles()})
}

// GET /api/v1/admin/accounts/:id/anti-degrade
func (h *AntiDegradeHandler) Preview(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	id, ok := antiDegradeID(c)
	if !ok {
		return
	}
	mode := service.AntiDegradeMode(c.Query("mode"))
	account, err := h.service.PreviewMode(c.Request.Context(), id, mode)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, account)
}

// POST /api/v1/admin/accounts/:id/anti-degrade/apply
func (h *AntiDegradeHandler) Apply(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	id, ok := antiDegradeID(c)
	if !ok {
		return
	}
	mode := service.AntiDegradeMode(c.Query("mode"))
	account, err := h.service.ApplyAntiDegradeMode(c.Request.Context(), id, mode)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.AccountFromService(account))
}

// POST /api/v1/admin/accounts/:id/anti-degrade/revert
func (h *AntiDegradeHandler) Revert(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	id, ok := antiDegradeID(c)
	if !ok {
		return
	}
	var req struct {
		ConfirmDisable bool `json:"confirm_disable"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || !req.ConfirmDisable {
		response.BadRequest(c, "关闭防降智模式需要管理员明确确认")
		return
	}
	middleware.SetAuditAction(c, "account.protection.disable")
	middleware.SetAuditExtra(c, map[string]any{"enabled": false, "confirm": true})
	account, err := h.service.Revert(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.AccountFromService(account))
}
