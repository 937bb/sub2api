package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

func TestPromoServiceUpdateClearsExpiryWithZeroTimeSentinel(t *testing.T) {
	ctx := context.Background()
	expiresAt := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	repo := newPromoServiceTestRepo(&PromoCode{
		ID:          1,
		Code:        "WELCOME",
		BonusAmount: 10,
		MaxUses:     5,
		Status:      PromoCodeStatusActive,
		ExpiresAt:   &expiresAt,
	})
	svc := NewPromoService(repo, nil, nil, nil, nil)

	updated, err := svc.Update(ctx, 1, &UpdatePromoCodeInput{ExpiresAt: &time.Time{}})
	require.NoError(t, err)
	require.Nil(t, updated.ExpiresAt)
	require.Nil(t, repo.updated.ExpiresAt)
}

func TestPromoServiceUpdateKeepsExpiryWhenOmitted(t *testing.T) {
	ctx := context.Background()
	expiresAt := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	repo := newPromoServiceTestRepo(&PromoCode{
		ID:          1,
		Code:        "WELCOME",
		BonusAmount: 10,
		MaxUses:     5,
		Status:      PromoCodeStatusActive,
		ExpiresAt:   &expiresAt,
	})
	svc := NewPromoService(repo, nil, nil, nil, nil)

	updated, err := svc.Update(ctx, 1, &UpdatePromoCodeInput{})
	require.NoError(t, err)
	require.NotNil(t, updated.ExpiresAt)
	require.True(t, updated.ExpiresAt.Equal(expiresAt))
	require.NotNil(t, repo.updated.ExpiresAt)
	require.True(t, repo.updated.ExpiresAt.Equal(expiresAt))
}

type promoServiceTestRepo struct {
	promo   *PromoCode
	updated *PromoCode
}

func newPromoServiceTestRepo(promo *PromoCode) *promoServiceTestRepo {
	return &promoServiceTestRepo{promo: clonePromoCode(promo)}
}

func (r *promoServiceTestRepo) Create(_ context.Context, code *PromoCode) error {
	r.promo = clonePromoCode(code)
	return nil
}

func (r *promoServiceTestRepo) GetByID(_ context.Context, id int64) (*PromoCode, error) {
	if r.promo == nil || r.promo.ID != id {
		return nil, ErrPromoCodeNotFound
	}
	return clonePromoCode(r.promo), nil
}

func (r *promoServiceTestRepo) GetByCode(_ context.Context, code string) (*PromoCode, error) {
	if r.promo == nil || r.promo.Code != code {
		return nil, ErrPromoCodeNotFound
	}
	return clonePromoCode(r.promo), nil
}

func (r *promoServiceTestRepo) GetByCodeForUpdate(ctx context.Context, code string) (*PromoCode, error) {
	return r.GetByCode(ctx, code)
}

func (r *promoServiceTestRepo) Update(_ context.Context, code *PromoCode) error {
	r.updated = clonePromoCode(code)
	r.promo = clonePromoCode(code)
	return nil
}

func (r *promoServiceTestRepo) Delete(_ context.Context, id int64) error {
	if r.promo != nil && r.promo.ID == id {
		r.promo = nil
	}
	return nil
}

func (r *promoServiceTestRepo) List(_ context.Context, _ pagination.PaginationParams) ([]PromoCode, *pagination.PaginationResult, error) {
	return nil, nil, nil
}

func (r *promoServiceTestRepo) ListWithFilters(_ context.Context, _ pagination.PaginationParams, _, _ string) ([]PromoCode, *pagination.PaginationResult, error) {
	return nil, nil, nil
}

func (r *promoServiceTestRepo) CreateUsage(_ context.Context, _ *PromoCodeUsage) error {
	return nil
}

func (r *promoServiceTestRepo) GetUsageByPromoCodeAndUser(_ context.Context, _, _ int64) (*PromoCodeUsage, error) {
	return nil, nil
}

func (r *promoServiceTestRepo) ListUsagesByPromoCode(_ context.Context, _ int64, _ pagination.PaginationParams) ([]PromoCodeUsage, *pagination.PaginationResult, error) {
	return nil, nil, nil
}

func (r *promoServiceTestRepo) IncrementUsedCount(_ context.Context, _ int64) error {
	return nil
}

func clonePromoCode(code *PromoCode) *PromoCode {
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
