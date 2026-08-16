package handler

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *OpenAIGatewayHandler) resolveOpenAIRequestModelMapping(
	c *gin.Context,
	apiKey *service.APIKey,
	requestedModel string,
) service.ChannelMappingResult {
	if apiKey != nil && apiKey.Group != nil && !isOpenAILegacyCompactPath(c) {
		if target, matched := apiKey.Group.ResolveOpenAIModelMapping(requestedModel); matched {
			target = strings.TrimSpace(target)
			service.MarkOpenAIGroupModelMapping(c, requestedModel, target)
			return service.ChannelMappingResult{
				MappedModel:        target,
				Mapped:             true,
				BillingModelSource: service.BillingModelSourceGroupMapped,
			}
		}
	}
	if h == nil || h.gatewayService == nil || apiKey == nil {
		return service.ChannelMappingResult{MappedModel: requestedModel}
	}
	mapping, _ := h.gatewayService.ResolveChannelMappingAndRestrict(c.Request.Context(), apiKey.GroupID, requestedModel)
	return mapping
}
