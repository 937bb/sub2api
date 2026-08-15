package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestResolveOpenAIRequestModelMappingRequiresGroupSwitch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &OpenAIGatewayHandler{}
	group := &service.Group{
		Platform: service.PlatformOpenAI,
		OpenAIModelMapping: map[string]string{
			"gpt-5.6-sol": "gpt-5.6-sol-wm",
		},
	}
	apiKey := &service.APIKey{Group: group}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	disabled := h.resolveOpenAIRequestModelMapping(c, apiKey, "gpt-5.6-sol")
	require.False(t, disabled.Mapped)
	require.Equal(t, "gpt-5.6-sol", disabled.MappedModel)

	group.OpenAIModelMappingEnabled = true
	enabled := h.resolveOpenAIRequestModelMapping(c, apiKey, "gpt-5.6-sol")
	require.True(t, enabled.Mapped)
	require.Equal(t, "gpt-5.6-sol-wm", enabled.MappedModel)
	require.Equal(t, service.BillingModelSourceGroupMapped, enabled.BillingModelSource)
}

func TestResolveOpenAIRequestModelMappingSkipsCompact(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &OpenAIGatewayHandler{}
	apiKey := &service.APIKey{Group: &service.Group{
		Platform:                  service.PlatformOpenAI,
		OpenAIModelMappingEnabled: true,
		OpenAIModelMapping:        map[string]string{"gpt-5.6-sol": "gpt-5.6-sol-wm"},
	}}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", nil)

	result := h.resolveOpenAIRequestModelMapping(c, apiKey, "gpt-5.6-sol")
	require.False(t, result.Mapped)
	require.Equal(t, "gpt-5.6-sol", result.MappedModel)
}
