package handler

import (
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// modelCatalogTimeBillingRule is the user-facing, non-sensitive portion of a
// group's recurring time billing configuration.
type modelCatalogTimeBillingRule struct {
	ID           string  `json:"id"`
	Enabled      bool    `json:"enabled"`
	RepeatType   string  `json:"repeat_type"`
	StartWeekday int     `json:"start_weekday,omitempty"`
	EndWeekday   int     `json:"end_weekday,omitempty"`
	Start        string  `json:"start"`
	End          string  `json:"end"`
	Rate         float64 `json:"rate_multiplier"`
}

type modelCatalogGroup struct {
	ID                      int64                         `json:"id"`
	Name                    string                        `json:"name"`
	Platform                string                        `json:"platform"`
	SubscriptionType        string                        `json:"subscription_type"`
	IsExclusive             bool                          `json:"is_exclusive"`
	DefaultRateMultiplier   float64                       `json:"default_rate_multiplier"`
	UserRateMultiplier      *float64                      `json:"user_rate_multiplier,omitempty"`
	ResolvedRateMultiplier  float64                       `json:"resolved_rate_multiplier"`
	TimeRateMultiplier      float64                       `json:"time_rate_multiplier"`
	EffectiveRateMultiplier float64                       `json:"effective_rate_multiplier"`
	ImageRateMultiplier     float64                       `json:"image_rate_multiplier"`
	VideoRateMultiplier     float64                       `json:"video_rate_multiplier"`
	TimeBillingRules        []modelCatalogTimeBillingRule `json:"time_billing_rules,omitempty"`
	Pricing                 *userSupportedModelPricing    `json:"pricing"`
}

type modelCatalogModel struct {
	Name         string                     `json:"name"`
	Platform     string                     `json:"platform"`
	BillingMode  service.BillingMode        `json:"billing_mode"`
	BasePricing  *userSupportedModelPricing `json:"base_pricing"`
	Groups       []modelCatalogGroup        `json:"groups"`
	ChannelCount int                        `json:"channel_count"`
}

type modelCatalogKey struct {
	Platform string
	Name     string
}

// ModelCatalog returns a model-centric view of the currently accessible
// catalog. This endpoint is intentionally independent from the legacy
// available-channels feature switch: the model marketplace is its own user
// page and must remain usable when that legacy page is disabled.
func (h *AvailableChannelHandler) ModelCatalog(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	groups, err := h.catalogGroups(c, subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	rates, err := h.apiKeyService.GetUserGroupRates(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	channels, err := h.channelService.ListAvailable(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	rows := buildModelCatalog(channels, groups, rates, timezone.Now())
	response.Success(c, gin.H{
		"models":        rows,
		"calculated_at": timezone.Now().UTC(),
		"timezone":      timezone.Name(),
	})
}

func (h *AvailableChannelHandler) catalogGroups(c *gin.Context, userID int64) ([]service.Group, error) {
	role, _ := middleware.GetUserRoleFromContext(c)
	if role == service.RoleAdmin {
		return h.apiKeyService.GetAllActiveGroups(c.Request.Context())
	}
	return h.apiKeyService.GetAvailableGroups(c.Request.Context(), userID)
}

func buildModelCatalog(channels []service.AvailableChannel, groups []service.Group, userRates map[int64]float64, now time.Time) []modelCatalogModel {
	groupByID := make(map[int64]service.Group, len(groups))
	for _, group := range groups {
		groupByID[group.ID] = group
	}

	rows := make(map[modelCatalogKey]*modelCatalogModel)
	for _, channel := range channels {
		if channel.Status != service.StatusActive {
			continue
		}
		for _, supported := range channel.SupportedModels {
			key := modelCatalogKey{Platform: supported.Platform, Name: strings.ToLower(supported.Name)}
			row := rows[key]
			if row == nil {
				mode := service.BillingModeToken
				if supported.Pricing != nil && supported.Pricing.BillingMode != "" {
					mode = supported.Pricing.BillingMode
				}
				row = &modelCatalogModel{
					Name: supported.Name, Platform: supported.Platform, BillingMode: mode,
					BasePricing: toUserPricing(supported.Pricing), Groups: make([]modelCatalogGroup, 0),
				}
				rows[key] = row
			}

			seenGroups := make(map[int64]struct{}, len(row.Groups))
			channelHasAccess := false
			for _, existing := range row.Groups {
				seenGroups[existing.ID] = struct{}{}
			}
			for _, ref := range channel.Groups {
				group, ok := groupByID[ref.ID]
				if !ok || group.Platform != supported.Platform {
					continue
				}
				if _, exists := seenGroups[group.ID]; exists {
					channelHasAccess = true
					continue
				}
				row.Groups = append(row.Groups, buildModelCatalogGroup(group, userRates, supported.Pricing, now))
				seenGroups[group.ID] = struct{}{}
				channelHasAccess = true
			}
			if channelHasAccess {
				row.ChannelCount++
			}
		}
	}

	result := make([]modelCatalogModel, 0, len(rows))
	for _, row := range rows {
		if len(row.Groups) == 0 {
			continue
		}
		sort.SliceStable(row.Groups, func(i, j int) bool {
			if row.Groups[i].IsExclusive != row.Groups[j].IsExclusive {
				return row.Groups[i].IsExclusive
			}
			return strings.ToLower(row.Groups[i].Name) < strings.ToLower(row.Groups[j].Name)
		})
		result = append(result, *row)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Platform != result[j].Platform {
			return result[i].Platform < result[j].Platform
		}
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})
	return result
}

func buildModelCatalogGroup(group service.Group, userRates map[int64]float64, basePricing *service.ChannelModelPricing, now time.Time) modelCatalogGroup {
	resolved := group.RateMultiplier
	var userRate *float64
	if value, ok := userRates[group.ID]; ok {
		copy := value
		userRate = &copy
		resolved = value
	}
	timeRate := group.PeakMultiplierAt(now)
	imageRate := resolved
	if group.ImageRateIndependent {
		imageRate = group.ImageRateMultiplier
	}
	videoRate := resolved
	if group.VideoRateIndependent {
		videoRate = group.VideoRateMultiplier
	}
	effective := resolved * timeRate
	mode := service.BillingModeToken
	if basePricing != nil && basePricing.BillingMode != "" {
		mode = basePricing.BillingMode
	}
	priceRate := effective
	switch mode {
	case service.BillingModeImage:
		priceRate = imageRate
	case service.BillingModeVideo:
		priceRate = videoRate
	}
	rules := make([]modelCatalogTimeBillingRule, 0, len(group.TimeBillingRules))
	for _, rule := range group.TimeBillingRules {
		rules = append(rules, modelCatalogTimeBillingRule{
			ID: rule.ID, Enabled: rule.Enabled, RepeatType: rule.RepeatType,
			StartWeekday: rule.StartWeekday, EndWeekday: rule.EndWeekday,
			Start: rule.Start, End: rule.End, Rate: rule.RateMultiplier,
		})
	}
	return modelCatalogGroup{
		ID: group.ID, Name: group.Name, Platform: group.Platform,
		SubscriptionType: group.SubscriptionType, IsExclusive: group.IsExclusive,
		DefaultRateMultiplier: group.RateMultiplier, UserRateMultiplier: userRate,
		ResolvedRateMultiplier: resolved, TimeRateMultiplier: timeRate,
		EffectiveRateMultiplier: effective, ImageRateMultiplier: imageRate,
		VideoRateMultiplier: videoRate, TimeBillingRules: rules,
		Pricing: scaleCatalogPricing(basePricing, priceRate),
	}
}

func scaleCatalogPricing(pricing *service.ChannelModelPricing, multiplier float64) *userSupportedModelPricing {
	if pricing == nil {
		return nil
	}
	clone := pricing.Clone()
	scale := func(value *float64) *float64 {
		if value == nil {
			return nil
		}
		result := *value * multiplier
		return &result
	}
	clone.InputPrice = scale(clone.InputPrice)
	clone.OutputPrice = scale(clone.OutputPrice)
	clone.CacheWritePrice = scale(clone.CacheWritePrice)
	clone.CacheReadPrice = scale(clone.CacheReadPrice)
	clone.ImageInputPrice = scale(clone.ImageInputPrice)
	clone.ImageOutputPrice = scale(clone.ImageOutputPrice)
	clone.PerRequestPrice = scale(clone.PerRequestPrice)
	for i := range clone.Intervals {
		clone.Intervals[i].InputPrice = scale(clone.Intervals[i].InputPrice)
		clone.Intervals[i].OutputPrice = scale(clone.Intervals[i].OutputPrice)
		clone.Intervals[i].CacheWritePrice = scale(clone.Intervals[i].CacheWritePrice)
		clone.Intervals[i].CacheReadPrice = scale(clone.Intervals[i].CacheReadPrice)
		clone.Intervals[i].PerRequestPrice = scale(clone.Intervals[i].PerRequestPrice)
	}
	return toUserPricing(&clone)
}
