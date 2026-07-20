package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type GrowthHandler struct {
	service    *service.GrowthService
	hashSecret []byte
}

func NewGrowthHandler(growthService *service.GrowthService, cfg *config.Config) *GrowthHandler {
	secret := []byte("sub2api-growth-signal")
	if cfg != nil && strings.TrimSpace(cfg.JWT.Secret) != "" {
		secret = []byte(cfg.JWT.Secret)
	}
	return &GrowthHandler{service: growthService, hashSecret: secret}
}

func (h *GrowthHandler) GetCheckinStatus(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	var month time.Time
	if raw := strings.TrimSpace(c.Query("month")); raw != "" {
		parsed, err := time.Parse("2006-01", raw)
		if err != nil {
			response.BadRequest(c, "month must use YYYY-MM format")
			return
		}
		month = parsed
	}
	status, err := h.service.GetCheckinStatus(c.Request.Context(), subject.UserID, month)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, status)
}

type growthCheckinRequest struct {
	DeviceID string `json:"device_id" binding:"required,min=16,max=128"`
}

func (h *GrowthHandler) Checkin(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	var req growthCheckinRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	clientIP := ip.GetClientIP(c)
	abuseIP := ip.AbuseIdentity(clientIP)
	result, err := h.service.Checkin(
		c.Request.Context(), subject.UserID,
		h.hashSignal("ip", abuseIP),
		h.hashSignal("device", strings.TrimSpace(req.DeviceID)+"|"+c.GetHeader("User-Agent")),
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *GrowthHandler) GetLeaderboard(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	period := c.DefaultQuery("period", "daily")
	page, pageSize := response.ParsePagination(c)
	result, err := h.service.GetLeaderboard(c.Request.Context(), period, subject.UserID, page, pageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *GrowthHandler) hashSignal(kind, value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	mac := hmac.New(sha256.New, h.hashSecret)
	_, _ = mac.Write([]byte(kind + ":" + value))
	return hex.EncodeToString(mac.Sum(nil))
}
