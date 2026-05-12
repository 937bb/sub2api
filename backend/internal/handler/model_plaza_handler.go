package handler

import (
	"sort"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// ModelPlazaHandler 处理模型广场查询。
//
// 与 AvailableChannelHandler 不同，本 handler 不依赖「渠道」配置，
// 直接从 PricingService（全局定价数据）和用户可访问分组聚合出模型列表。
type ModelPlazaHandler struct {
	apiKeyService  *service.APIKeyService
	pricingService *service.PricingService
}

// NewModelPlazaHandler 创建模型广场 handler。
func NewModelPlazaHandler(
	apiKeyService *service.APIKeyService,
	pricingService *service.PricingService,
) *ModelPlazaHandler {
	return &ModelPlazaHandler{
		apiKeyService:  apiKeyService,
		pricingService: pricingService,
	}
}

// ── Response DTOs ──

type modelPlazaGroup struct {
	ID               int64   `json:"id"`
	Name             string  `json:"name"`
	Platform         string  `json:"platform"`
	RateMultiplier   float64 `json:"rate_multiplier"`
	IsExclusive      bool    `json:"is_exclusive"`
	SubscriptionType string  `json:"subscription_type"`
}

type modelPlazaPricing struct {
	InputPrice      *float64 `json:"input_price"`
	OutputPrice     *float64 `json:"output_price"`
	CacheWritePrice *float64 `json:"cache_write_price"`
	CacheReadPrice  *float64 `json:"cache_read_price"`
	ImageOutputPrice *float64 `json:"image_output_price"`
}

type modelPlazaModel struct {
	Name     string             `json:"name"`
	Platform string             `json:"platform"`
	Mode     string             `json:"mode"`
	Pricing  *modelPlazaPricing `json:"pricing"`
	Groups   []modelPlazaGroup  `json:"groups"`
}

type modelPlazaResponse struct {
	Models []modelPlazaModel `json:"models"`
	Groups []modelPlazaGroup `json:"groups"`
}

// providerToPlatform maps LiteLLM provider names to sub2api platform names.
var providerToPlatform = map[string]string{
	"openai":           "openai",
	"anthropic":        "anthropic",
	"vertex_ai":        "gemini",
	"vertex_ai-text":   "gemini",
	"vertex_ai-vision": "gemini",
	"gemini":           "gemini",
	"google":           "gemini",
}

// List 返回模型广场数据。
// GET /api/v1/model-plaza
func (h *ModelPlazaHandler) List(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	// 1. 获取用户可访问的分组
	userGroups, err := h.apiKeyService.GetAvailableGroups(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	// 2. 构建平台 → 分组映射
	platformGroups := make(map[string][]modelPlazaGroup)
	allGroups := make([]modelPlazaGroup, 0, len(userGroups))
	for _, g := range userGroups {
		dto := modelPlazaGroup{
			ID:               g.ID,
			Name:             g.Name,
			Platform:         g.Platform,
			RateMultiplier:   g.RateMultiplier,
			IsExclusive:      g.IsExclusive,
			SubscriptionType: g.SubscriptionType,
		}
		allGroups = append(allGroups, dto)
		platformGroups[g.Platform] = append(platformGroups[g.Platform], dto)
	}

	// 3. 获取所有模型定价数据
	allModels := h.pricingService.ListAll()

	// 4. 按用户平台过滤并聚合
	models := make([]modelPlazaModel, 0, len(allModels))
	for _, m := range allModels {
		// 将 LiteLLM provider 映射到 sub2api 平台
		platform := mapProviderToPlatform(m.Provider)

		// 只返回用户分组覆盖的平台的模型
		groups, ok := platformGroups[platform]
		if !ok {
			continue
		}

		pricing := toPlazaPricing(m.Pricing)

		models = append(models, modelPlazaModel{
			Name:     m.Name,
			Platform: platform,
			Mode:     m.Mode,
			Pricing:  pricing,
			Groups:   groups,
		})
	}

	// 排序：先按平台，再按名称
	sort.Slice(models, func(i, j int) bool {
		if models[i].Platform != models[j].Platform {
			return models[i].Platform < models[j].Platform
		}
		return models[i].Name < models[j].Name
	})

	response.Success(c, modelPlazaResponse{
		Models: models,
		Groups: allGroups,
	})
}

func mapProviderToPlatform(provider string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if p, ok := providerToPlatform[provider]; ok {
		return p
	}
	return provider
}

func toPlazaPricing(p *service.LiteLLMModelPricing) *modelPlazaPricing {
	if p == nil {
		return nil
	}
	pricing := &modelPlazaPricing{}
	if p.InputCostPerToken > 0 {
		v := p.InputCostPerToken
		pricing.InputPrice = &v
	}
	if p.OutputCostPerToken > 0 {
		v := p.OutputCostPerToken
		pricing.OutputPrice = &v
	}
	if p.CacheCreationInputTokenCost > 0 {
		v := p.CacheCreationInputTokenCost
		pricing.CacheWritePrice = &v
	}
	if p.CacheReadInputTokenCost > 0 {
		v := p.CacheReadInputTokenCost
		pricing.CacheReadPrice = &v
	}
	if p.OutputCostPerImage > 0 {
		v := p.OutputCostPerImage
		pricing.ImageOutputPrice = &v
	}
	return pricing
}
