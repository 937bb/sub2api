package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPromoHandlerUpdateClearsExpiryWhenExpiresAtIsZero(t *testing.T) {
	gin.SetMode(gin.TestMode)

	expiresAt := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	repo := newPromoHandlerTestRepo(&service.PromoCode{
		ID:          1,
		Code:        "WELCOME",
		BonusAmount: 10,
		MaxUses:     5,
		Status:      service.PromoCodeStatusActive,
		ExpiresAt:   &expiresAt,
	})
	handler := NewPromoHandler(service.NewPromoService(repo, nil, nil, nil, nil))

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	c.Request = httptest.NewRequest(
		http.MethodPut,
		"/api/v1/admin/promo-codes/1",
		bytes.NewBufferString(`{"expires_at":0}`),
	)
	c.Request.Header.Set("Content-Type", "application/json")

	handler.Update(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, repo.updated)
	require.Nil(t, repo.updated.ExpiresAt)

	var body struct {
		Code int `json:"code"`
		Data struct {
			ExpiresAt *time.Time `json:"expires_at"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Zero(t, body.Code)
	require.Nil(t, body.Data.ExpiresAt)
}

type promoHandlerTestRepo struct {
	promo   *service.PromoCode
	updated *service.PromoCode
}

func newPromoHandlerTestRepo(promo *service.PromoCode) *promoHandlerTestRepo {
	return &promoHandlerTestRepo{promo: clonePromoHandlerTestCode(promo)}
}

func (r *promoHandlerTestRepo) Create(_ context.Context, code *service.PromoCode) error {
	r.promo = clonePromoHandlerTestCode(code)
	return nil
}

func (r *promoHandlerTestRepo) GetByID(_ context.Context, id int64) (*service.PromoCode, error) {
	if r.promo == nil || r.promo.ID != id {
		return nil, service.ErrPromoCodeNotFound
	}
	return clonePromoHandlerTestCode(r.promo), nil
}

func (r *promoHandlerTestRepo) GetByCode(_ context.Context, code string) (*service.PromoCode, error) {
	if r.promo == nil || r.promo.Code != code {
		return nil, service.ErrPromoCodeNotFound
	}
	return clonePromoHandlerTestCode(r.promo), nil
}

func (r *promoHandlerTestRepo) GetByCodeForUpdate(ctx context.Context, code string) (*service.PromoCode, error) {
	return r.GetByCode(ctx, code)
}

func (r *promoHandlerTestRepo) Update(_ context.Context, code *service.PromoCode) error {
	r.updated = clonePromoHandlerTestCode(code)
	r.promo = clonePromoHandlerTestCode(code)
	return nil
}

func (r *promoHandlerTestRepo) Delete(_ context.Context, id int64) error {
	if r.promo != nil && r.promo.ID == id {
		r.promo = nil
	}
	return nil
}

func (r *promoHandlerTestRepo) List(_ context.Context, _ pagination.PaginationParams) ([]service.PromoCode, *pagination.PaginationResult, error) {
	return nil, nil, nil
}

func (r *promoHandlerTestRepo) ListWithFilters(_ context.Context, _ pagination.PaginationParams, _, _ string) ([]service.PromoCode, *pagination.PaginationResult, error) {
	return nil, nil, nil
}

func (r *promoHandlerTestRepo) CreateUsage(_ context.Context, _ *service.PromoCodeUsage) error {
	return nil
}

func (r *promoHandlerTestRepo) GetUsageByPromoCodeAndUser(_ context.Context, _, _ int64) (*service.PromoCodeUsage, error) {
	return nil, nil
}

func (r *promoHandlerTestRepo) ListUsagesByPromoCode(_ context.Context, _ int64, _ pagination.PaginationParams) ([]service.PromoCodeUsage, *pagination.PaginationResult, error) {
	return nil, nil, nil
}

func (r *promoHandlerTestRepo) IncrementUsedCount(_ context.Context, _ int64) error {
	return nil
}

func clonePromoHandlerTestCode(code *service.PromoCode) *service.PromoCode {
	if code == nil {
		return nil
	}
	clone := *code
	if code.ExpiresAt != nil {
		expiresAt := *code.ExpiresAt
		clone.ExpiresAt = &expiresAt
	}
	return &clone
}
