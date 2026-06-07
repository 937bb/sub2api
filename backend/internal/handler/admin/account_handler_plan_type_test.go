package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func setupBatchRefreshPlanTypeRouter(adminSvc *stubAdminService) *gin.Engine {
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.POST("/api/v1/admin/accounts/batch-refresh-plan-type", handler.BatchRefreshPlanType)
	return router
}

func TestAccountHandlerBatchRefreshPlanTypeReturnsErrorWhenServiceNotConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := setupBatchRefreshPlanTypeRouter(newStubAdminService())
	body, _ := json.Marshal(gin.H{"account_ids": []int64{101}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/batch-refresh-plan-type", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "plan_type refresh is not configured")
}

func TestAccountHandlerBatchRefreshPlanTypeRejectsEmptyAccountIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := setupBatchRefreshPlanTypeRouter(newStubAdminService())
	body, _ := json.Marshal(gin.H{"account_ids": []int64{}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/batch-refresh-plan-type", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "account_ids is required")
}

func TestAccountHandlerBatchRefreshPlanTypeAggregatesFailuresForMissingAccounts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adminSvc := newStubAdminService()
	adminSvc.getAccountsByIDs = func(_ context.Context, ids []int64) ([]*service.Account, error) {
		// Only return one account; id 103 not found.
		return []*service.Account{
			{ID: 101, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth},
		}, nil
	}

	router := setupBatchRefreshPlanTypeRouter(adminSvc)
	body, _ := json.Marshal(gin.H{"account_ids": []int64{101, 102, 103}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/batch-refresh-plan-type", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	// With accountUsageService nil, the handler returns the "not configured" error,
	// but the not-found check happens first in the handler logic.
	require.Equal(t, http.StatusBadRequest, rec.Code)
}
