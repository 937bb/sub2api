package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type quotaBypassGroupAdminServiceStub struct {
	service.AdminService
	createInput *service.CreateGroupInput
	updateInput *service.UpdateGroupInput
}

func (s *quotaBypassGroupAdminServiceStub) CreateGroup(_ context.Context, input *service.CreateGroupInput) (*service.Group, error) {
	s.createInput = input
	return &service.Group{
		ID:                                       42,
		Name:                                     input.Name,
		Platform:                                 service.PlatformOpenAI,
		Status:                                   service.StatusActive,
		QuotaBypassEnabled:                       input.QuotaBypassEnabled,
		QuotaBypassConcentratedSchedulingEnabled: input.QuotaBypassConcentratedSchedulingEnabled,
	}, nil
}

func (s *quotaBypassGroupAdminServiceStub) UpdateGroup(_ context.Context, id int64, input *service.UpdateGroupInput) (*service.Group, error) {
	s.updateInput = input
	concentrated := false
	if input.QuotaBypassConcentratedSchedulingEnabled != nil {
		concentrated = *input.QuotaBypassConcentratedSchedulingEnabled
	}
	return &service.Group{
		ID:                                       id,
		Name:                                     "openai-bypass",
		Platform:                                 service.PlatformOpenAI,
		Status:                                   service.StatusActive,
		QuotaBypassEnabled:                       true,
		QuotaBypassConcentratedSchedulingEnabled: concentrated,
	}, nil
}

func setupQuotaBypassGroupRouter(svc service.AdminService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewGroupHandler(svc, nil, nil)
	router.POST("/api/v1/admin/groups", handler.Create)
	router.PUT("/api/v1/admin/groups/:id", handler.Update)
	return router
}

func TestGroupHandlerCreatePassesQuotaBypassConcentratedScheduling(t *testing.T) {
	svc := &quotaBypassGroupAdminServiceStub{}
	router := setupQuotaBypassGroupRouter(svc)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/groups", strings.NewReader(`{
		"name":"openai-bypass-concentrated",
		"platform":"openai",
		"rate_multiplier":1,
		"quota_bypass_enabled":true,
		"quota_bypass_concentrated_scheduling_enabled":true
	}`))
	request.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.NotNil(t, svc.createInput)
	require.True(t, svc.createInput.QuotaBypassEnabled)
	require.True(t, svc.createInput.QuotaBypassConcentratedSchedulingEnabled)
	require.Contains(t, recorder.Body.String(), `"quota_bypass_concentrated_scheduling_enabled":true`)
}

func TestGroupHandlerUpdatePassesQuotaBypassConcentratedSchedulingTriState(t *testing.T) {
	t.Run("omitted remains nil", func(t *testing.T) {
		svc := &quotaBypassGroupAdminServiceStub{}
		router := setupQuotaBypassGroupRouter(svc)
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPut, "/api/v1/admin/groups/42", strings.NewReader(`{}`))
		request.Header.Set("Content-Type", "application/json")

		router.ServeHTTP(recorder, request)

		require.Equal(t, http.StatusOK, recorder.Code)
		require.NotNil(t, svc.updateInput)
		require.Nil(t, svc.updateInput.QuotaBypassConcentratedSchedulingEnabled)
	})

	t.Run("explicit false is preserved", func(t *testing.T) {
		svc := &quotaBypassGroupAdminServiceStub{}
		router := setupQuotaBypassGroupRouter(svc)
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPut, "/api/v1/admin/groups/42", strings.NewReader(`{
			"quota_bypass_concentrated_scheduling_enabled":false
		}`))
		request.Header.Set("Content-Type", "application/json")

		router.ServeHTTP(recorder, request)

		require.Equal(t, http.StatusOK, recorder.Code)
		require.NotNil(t, svc.updateInput)
		require.NotNil(t, svc.updateInput.QuotaBypassConcentratedSchedulingEnabled)
		require.False(t, *svc.updateInput.QuotaBypassConcentratedSchedulingEnabled)
		require.Contains(t, recorder.Body.String(), `"quota_bypass_concentrated_scheduling_enabled":false`)
	})
}
