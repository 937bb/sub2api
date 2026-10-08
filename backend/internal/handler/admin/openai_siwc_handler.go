package admin

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *OpenAIOAuthHandler) RepairLegacySIWC(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	account, err := h.adminService.GetAccount(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	credentials, err := h.openaiOAuthService.RecoverLegacySIWC(c.Request.Context(), account)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	writer, ok := h.adminService.(interface {
		SaveLegacySIWCCredentials(context.Context, *service.Account, map[string]any) (*service.Account, error)
	})
	if !ok {
		response.BadRequest(c, "SIWC repair unavailable")
		return
	}
	account, err = writer.SaveLegacySIWCCredentials(c.Request.Context(), account, credentials)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.AccountForObserver(c.Request.Context(), dto.AccountFromService(account)))
}

func (h *OpenAIOAuthHandler) GenerateSIWCAuthURL(c *gin.Context) {
	// Reauthorization URLs may contain an ID-token hint for the official issuer.
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	var input struct {
		ProxyID         *int64 `json:"proxy_id"`
		HostID          string `json:"host_id"`
		AccountID       int64  `json:"account_id" binding:"min=0"`
		ResumeSessionID string `json:"resume_session_id" binding:"max=256"`
	}
	if c.ShouldBindJSON(&input) != nil {
		response.BadRequest(c, "Invalid SIWC authorization request")
		return
	}
	var result *service.OpenAIAuthURLResult
	var err error
	if input.AccountID > 0 {
		account, loadErr := h.adminService.GetAccount(c.Request.Context(), input.AccountID)
		if loadErr != nil {
			response.ErrorFrom(c, loadErr)
			return
		}
		result, err = h.openaiOAuthService.GenerateSIWCReauthURL(c.Request.Context(), account)
	} else {
		result, err = h.openaiOAuthService.GenerateSIWCAuthURL(c.Request.Context(), input.ProxyID, input.HostID, input.ResumeSessionID)
	}
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *OpenAIOAuthHandler) CreateAccountFromSIWC(c *gin.Context) {
	var input struct {
		SessionID   string  `json:"session_id" binding:"required"`
		CallbackURL string  `json:"callback_url" binding:"required"`
		Name        string  `json:"name"`
		Concurrency int     `json:"concurrency" binding:"min=1,max=100"`
		GroupIDs    []int64 `json:"group_ids"`
		AccountID   int64   `json:"account_id" binding:"min=0"`
	}
	if c.ShouldBindJSON(&input) != nil {
		response.BadRequest(c, "Invalid SIWC account request")
		return
	}
	account, err := h.openaiOAuthService.CompleteSIWC(c.Request.Context(), input.SessionID, input.CallbackURL, input.AccountID,
		func(credentials map[string]any, proxyID *int64) (*service.Account, error) {
			if input.AccountID > 0 {
				account, err := h.adminService.GetAccount(c.Request.Context(), input.AccountID)
				if err != nil {
					return nil, err
				}
				if (proxyID == nil) != (account.ProxyID == nil) || (proxyID != nil && *proxyID != *account.ProxyID) {
					return nil, errors.New("SIWC proxy changed; start authorization again")
				}
				writer, ok := h.adminService.(interface {
					SaveSIWCCredentials(context.Context, *service.Account, map[string]any) (*service.Account, error)
				})
				if !ok {
					return nil, errors.New("SIWC persistence unavailable")
				}
				delete(credentials, "model_mapping")
				return writer.SaveSIWCCredentials(c.Request.Context(), account, credentials)
			}
			name := strings.TrimSpace(input.Name)
			if name == "" {
				name, _ = credentials["email"].(string)
			}
			if name == "" {
				name = "ChatGPT SIWC"
			}
			return h.adminService.CreateAccount(c.Request.Context(), &service.CreateAccountInput{
				Name: name, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
				Credentials: credentials, ProxyID: proxyID, Concurrency: input.Concurrency,
				GroupIDs: input.GroupIDs, SkipDefaultGroupBind: true,
				Extra: map[string]any{"openai_ws_force_http": true},
			})
		})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.AccountForObserver(c.Request.Context(), dto.AccountFromService(account)))
}
