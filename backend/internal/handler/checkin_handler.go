package handler

import (
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// CheckinHandler 签到用户端 handler
type CheckinHandler struct {
	checkinService *service.CheckinService
	settingService *service.SettingService
}

// NewCheckinHandler creates a new CheckinHandler
func NewCheckinHandler(
	checkinService *service.CheckinService,
	settingService *service.SettingService,
) *CheckinHandler {
	return &CheckinHandler{
		checkinService: checkinService,
		settingService: settingService,
	}
}

// checkinRecordResponse 签到记录响应
type checkinRecordResponse struct {
	ID              int64   `json:"id"`
	CheckinDate     string  `json:"checkin_date"`
	Streak          int     `json:"streak"`
	BaseAmount      float64 `json:"base_amount"`
	MilestoneAmount float64 `json:"milestone_amount"`
	TotalAmount     float64 `json:"total_amount"`
	BalanceType     string  `json:"balance_type"`
	CreatedAt       time.Time `json:"created_at"`
}

// checkinStatusResponse 签到状态响应
type checkinStatusResponse struct {
	Enabled        bool                      `json:"enabled"`
	CheckedInToday bool                      `json:"checked_in_today"`
	CurrentStreak  int                       `json:"current_streak"`
	TotalCheckins  int64                     `json:"total_checkins"`
	LastCheckinDate string                   `json:"last_checkin_date,omitempty"`
	Config         *checkinConfigResponse    `json:"config,omitempty"`
}

// checkinConfigResponse 签到配置响应（仅公开必要信息给前端）
type checkinConfigResponse struct {
	Mode        string                       `json:"mode"`
	FixedAmount float64                      `json:"fixed_amount,omitempty"`
	RandomMin   float64                      `json:"random_min,omitempty"`
	RandomMax   float64                      `json:"random_max,omitempty"`
	BalanceType string                       `json:"balance_type"`
	ExpiryDays  int                          `json:"expiry_days,omitempty"`
	Milestones  []service.CheckinMilestone   `json:"milestones,omitempty"`
}

// Checkin 执行签到
// POST /api/v1/user/checkin
func (h *CheckinHandler) Checkin(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	record, err := h.checkinService.Checkin(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, checkinRecordResponse{
		ID:              record.ID,
		CheckinDate:     record.CheckinDate.Format("2006-01-02"),
		Streak:          record.Streak,
		BaseAmount:      record.BaseAmount,
		MilestoneAmount: record.MilestoneAmount,
		TotalAmount:     record.TotalAmount,
		BalanceType:     record.BalanceType,
		CreatedAt:       record.CreatedAt,
	})
}

// Status 获取签到状态
// GET /api/v1/user/checkin/status
func (h *CheckinHandler) Status(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	config := h.checkinService.GetCheckinConfig(c.Request.Context())
	if !config.Enabled {
		response.Success(c, checkinStatusResponse{Enabled: false})
		return
	}

	status, err := h.checkinService.GetStatus(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	resp := checkinStatusResponse{
		Enabled:         true,
		CheckedInToday:  status.CheckedInToday,
		CurrentStreak:   status.CurrentStreak,
		TotalCheckins:   status.TotalCheckins,
		LastCheckinDate: status.LastCheckinDate,
		Config: &checkinConfigResponse{
			Mode:        config.Mode,
			FixedAmount: config.FixedAmount,
			RandomMin:   config.RandomMin,
			RandomMax:   config.RandomMax,
			BalanceType: config.BalanceType,
			ExpiryDays:  config.ExpiryDays,
			Milestones:  config.Milestones,
		},
	}

	response.Success(c, resp)
}

// Calendar 获取月度签到日历
// GET /api/v1/user/checkin/calendar?year=2025&month=5
func (h *CheckinHandler) Calendar(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	now := time.Now()
	year := now.Year()
	month := int(now.Month())

	if y, err := strconv.Atoi(c.Query("year")); err == nil && y >= 2020 && y <= 2100 {
		year = y
	}
	if m, err := strconv.Atoi(c.Query("month")); err == nil && m >= 1 && m <= 12 {
		month = m
	}

	records, err := h.checkinService.GetMonthlyRecords(c.Request.Context(), subject.UserID, year, month)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	items := make([]checkinRecordResponse, 0, len(records))
	for _, r := range records {
		items = append(items, checkinRecordResponse{
			ID:              r.ID,
			CheckinDate:     r.CheckinDate.Format("2006-01-02"),
			Streak:          r.Streak,
			BaseAmount:      r.BaseAmount,
			MilestoneAmount: r.MilestoneAmount,
			TotalAmount:     r.TotalAmount,
			BalanceType:     r.BalanceType,
			CreatedAt:       r.CreatedAt,
		})
	}

	response.Success(c, gin.H{
		"year":    year,
		"month":   month,
		"records": items,
	})
}
