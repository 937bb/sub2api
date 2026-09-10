package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *SettingHandler) GetOAuthRetrySettings(c *gin.Context) {
	value, err := h.settingService.GetOAuthRetrySettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, value)
}

func (h *SettingHandler) UpdateOAuthRetrySettings(c *gin.Context) {
	var value service.OAuthRetrySettings
	if err := c.ShouldBindJSON(&value); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if err := service.ValidateOAuthRetrySettings(value); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.settingService.SetOAuthRetrySettings(c.Request.Context(), value); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, value)
}
