package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type redeemHandlerRepo struct {
	code      service.RedeemCode
	useCalled bool
}

func (r *redeemHandlerRepo) Create(context.Context, *service.RedeemCode) error       { return nil }
func (r *redeemHandlerRepo) CreateBatch(context.Context, []service.RedeemCode) error { return nil }
func (r *redeemHandlerRepo) GetByID(context.Context, int64) (*service.RedeemCode, error) {
	return nil, service.ErrRedeemCodeNotFound
}
func (r *redeemHandlerRepo) GetByCode(_ context.Context, code string) (*service.RedeemCode, error) {
	if code != r.code.Code {
		return nil, service.ErrRedeemCodeNotFound
	}
	clone := r.code
	return &clone, nil
}
func (r *redeemHandlerRepo) Update(context.Context, *service.RedeemCode) error { return nil }
func (r *redeemHandlerRepo) BatchUpdate(context.Context, []int64, service.RedeemCodeBatchUpdateFields) (int64, error) {
	return 0, nil
}
func (r *redeemHandlerRepo) Delete(context.Context, int64) error { return nil }
func (r *redeemHandlerRepo) Use(context.Context, int64, int64) error {
	r.useCalled = true
	return nil
}
func (r *redeemHandlerRepo) List(context.Context, pagination.PaginationParams) ([]service.RedeemCode, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *redeemHandlerRepo) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string) ([]service.RedeemCode, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *redeemHandlerRepo) ListByUser(context.Context, int64, int) ([]service.RedeemCode, error) {
	return nil, nil
}
func (r *redeemHandlerRepo) ListByUserPaginated(context.Context, int64, pagination.PaginationParams, string) ([]service.RedeemCode, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *redeemHandlerRepo) SumPositiveBalanceByUser(context.Context, int64) (float64, error) {
	return 0, nil
}

type redeemHandlerCache struct{ increments int }

func (*redeemHandlerCache) GetRedeemAttemptCount(context.Context, int64) (int, error) { return 0, nil }
func (c *redeemHandlerCache) IncrementRedeemAttemptCount(context.Context, int64, time.Duration) error {
	c.increments++
	return nil
}
func (*redeemHandlerCache) DeleteRedeemAttemptCount(context.Context, int64) error { return nil }
func (*redeemHandlerCache) AcquireRedeemLock(context.Context, string, time.Duration) (bool, error) {
	return true, nil
}
func (*redeemHandlerCache) ReleaseRedeemLock(context.Context, string) error { return nil }

func TestRedeemHandler_UnsupportedStoredTypeReturnsStructuredBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name     string
		codeType string
		message  string
	}{
		{name: "invitation", codeType: service.RedeemTypeInvitation, message: "invitation codes can only be used during registration"},
		{name: "unknown", codeType: "unexpected", message: "unsupported redeem type: unexpected"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &redeemHandlerRepo{code: service.RedeemCode{ID: 1, Code: "KNOWN-CODE", Type: tt.codeType, Status: service.StatusUnused}}
			cache := &redeemHandlerCache{}
			handler := NewRedeemHandler(service.NewRedeemService(repo, nil, nil, cache, nil, nil, nil, nil))

			router := gin.New()
			router.POST("/api/v1/redeem", func(c *gin.Context) {
				c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
				handler.Redeem(c)
			})

			request := httptest.NewRequest(http.MethodPost, "/api/v1/redeem", bytes.NewBufferString(`{"code":"KNOWN-CODE"}`))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			require.Equal(t, http.StatusBadRequest, response.Code)
			var body struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
				Reason  string `json:"reason"`
			}
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
			require.Equal(t, http.StatusBadRequest, body.Code)
			require.Equal(t, tt.message, body.Message)
			require.Equal(t, "REDEEM_CODE_UNSUPPORTED_TYPE", body.Reason)
			require.False(t, repo.useCalled)
			require.Equal(t, 0, cache.increments)
		})
	}
}
