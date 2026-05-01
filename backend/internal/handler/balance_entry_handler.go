package handler

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// BalanceEntryHandler 余额明细用户端 handler
type BalanceEntryHandler struct {
	balanceEntryService *service.BalanceEntryService
	settingService      *service.SettingService
}

// NewBalanceEntryHandler creates a new BalanceEntryHandler
func NewBalanceEntryHandler(
	balanceEntryService *service.BalanceEntryService,
	settingService *service.SettingService,
) *BalanceEntryHandler {
	return &BalanceEntryHandler{
		balanceEntryService: balanceEntryService,
		settingService:      settingService,
	}
}

// balanceEntryResponse 单条余额明细响应
type balanceEntryResponse struct {
	ID          int64      `json:"id"`
	Amount      float64    `json:"amount"`
	Remaining   float64    `json:"remaining"`
	BalanceType string     `json:"balance_type"`
	Source      string     `json:"source"`
	Note        string     `json:"note"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	Expired     bool       `json:"expired"`
	CreatedAt   time.Time  `json:"created_at"`
}

// balanceSummaryResponse 余额汇总响应
type balanceSummaryResponse struct {
	TotalBalance     float64 `json:"total_balance"`
	PermanentBalance float64 `json:"permanent_balance"`
	ExpirableBalance float64 `json:"expirable_balance"`
	ExpiringSoon     float64 `json:"expiring_soon"`
	WarningDays      int     `json:"warning_days"`
}

// List 获取用户余额明细列表（分页）
// GET /api/v1/user/balance-entries
func (h *BalanceEntryHandler) List(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	page, pageSize := response.ParsePagination(c)
	offset := (page - 1) * pageSize

	entries, total, err := h.balanceEntryService.ListByUser(c.Request.Context(), subject.UserID, offset, pageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	items := make([]balanceEntryResponse, 0, len(entries))
	for _, e := range entries {
		items = append(items, balanceEntryResponse{
			ID:          e.ID,
			Amount:      e.Amount,
			Remaining:   e.Remaining,
			BalanceType: e.BalanceType,
			Source:      e.Source,
			Note:        e.Note,
			ExpiresAt:   e.ExpiresAt,
			Expired:     e.Expired,
			CreatedAt:   e.CreatedAt,
		})
	}

	response.Paginated(c, items, total, page, pageSize)
}

// Summary 获取用户余额汇总
// GET /api/v1/user/balance-entries/summary
func (h *BalanceEntryHandler) Summary(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	summary, err := h.balanceEntryService.GetUserBalanceSummary(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	warningDays := 3
	if h.settingService != nil {
		warningDays = h.settingService.GetBalanceExpiryWarningDays(c.Request.Context())
	}

	response.Success(c, balanceSummaryResponse{
		TotalBalance:     summary.TotalBalance,
		PermanentBalance: summary.PermanentBalance,
		ExpirableBalance: summary.ExpirableBalance,
		ExpiringSoon:     summary.ExpiringSoon,
		WarningDays:      warningDays,
	})
}
