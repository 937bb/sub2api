//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

type quotaBypassRateLimitRecoveryRepo struct {
	AccountRepository
	accounts   []Account
	listCalls  int
	clearedIDs []int64
}

func (r *quotaBypassRateLimitRecoveryRepo) SupportsOpenAIQuotaBypassRateLimitRecovery() bool {
	return true
}

func (r *quotaBypassRateLimitRecoveryRepo) ListModelAvailabilityCandidates(context.Context, *int64, []string, bool) ([]Account, error) {
	r.listCalls++
	return append([]Account(nil), r.accounts...), nil
}

func (r *quotaBypassRateLimitRecoveryRepo) ClearRateLimit(_ context.Context, id int64) error {
	r.clearedIDs = append(r.clearedIDs, id)
	return nil
}

func TestRecoverPersistedOpenAIQuotaBypassRateLimits(t *testing.T) {
	groupID := int64(91)
	group := &Group{ID: groupID, QuotaBypassEnabled: true}
	limitedAt := time.Now().Add(-time.Minute)
	resetAt := time.Now().Add(time.Hour)
	repo := &quotaBypassRateLimitRecoveryRepo{accounts: []Account{
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, RateLimitedAt: &limitedAt, RateLimitResetAt: &resetAt},
		{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, RateLimitedAt: &limitedAt, RateLimitResetAt: &resetAt, Extra: map[string]any{"quota_bypass_enabled": false}},
		{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusError, Schedulable: true, RateLimitedAt: &limitedAt, RateLimitResetAt: &resetAt},
		{ID: 4, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true},
	}}
	svc := &OpenAIGatewayService{accountRepo: repo}
	svc.BlockAccountScheduling(&repo.accounts[0], resetAt, "legacy_429")
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(&repo.accounts[0]))
	ctx := context.WithValue(context.Background(), ctxkey.Group, group)

	svc.recoverPersistedOpenAIQuotaBypassRateLimits(ctx, &groupID)
	svc.recoverPersistedOpenAIQuotaBypassRateLimits(ctx, &groupID)

	require.Equal(t, 1, repo.listCalls)
	require.Equal(t, []int64{1}, repo.clearedIDs)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(&repo.accounts[0]))
}

func TestRecoverPersistedOpenAIQuotaBypassRateLimits_RegularGroupOnlyRecoversAccountOverride(t *testing.T) {
	groupID := int64(92)
	limitedAt := time.Now().Add(-time.Minute)
	resetAt := time.Now().Add(time.Hour)
	repo := &quotaBypassRateLimitRecoveryRepo{accounts: []Account{
		{ID: 10, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, RateLimitedAt: &limitedAt, RateLimitResetAt: &resetAt},
		{ID: 11, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, RateLimitedAt: &limitedAt, RateLimitResetAt: &resetAt, Extra: map[string]any{"quota_bypass_enabled": true}},
	}}
	svc := &OpenAIGatewayService{accountRepo: repo}
	ctx := context.WithValue(context.Background(), ctxkey.Group, &Group{ID: groupID})

	svc.recoverPersistedOpenAIQuotaBypassRateLimits(ctx, &groupID)

	require.Equal(t, 1, repo.listCalls)
	require.Equal(t, []int64{11}, repo.clearedIDs)
}
