package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type opsAccountStatsRepoStub struct {
	AccountRepository
	accounts       []Account
	platformFilter string
	groupIDFilter  *int64
	listCalls      int
}

func (r *opsAccountStatsRepoStub) ListOpsAccountsForStats(_ context.Context, platform string, groupID *int64) ([]Account, error) {
	r.platformFilter = platform
	r.groupIDFilter = groupID
	r.listCalls++
	return r.accounts, nil
}

func (r *opsAccountStatsRepoStub) ListWithFilters(_ context.Context, _ pagination.PaginationParams, _ AccountListFilters) ([]Account, *pagination.PaginationResult, error) {
	r.listCalls++
	return nil, nil, nil
}

type opsAccountFallbackRepoStub struct {
	AccountRepository
	filters []AccountListFilters
}

func (r *opsAccountFallbackRepoStub) ListWithFilters(_ context.Context, _ pagination.PaginationParams, filters AccountListFilters) ([]Account, *pagination.PaginationResult, error) {
	r.filters = append(r.filters, filters)
	return []Account{{ID: 9}}, &pagination.PaginationResult{Total: 1}, nil
}

func TestListAllAccountsForOpsOptimizedFallbackFilterParity(t *testing.T) {
	zero := int64(0)
	negative := int64(-7)
	positive := int64(42)
	tests := []struct {
		name         string
		platform     string
		groupID      *int64
		wantPlatform string
		wantGroupID  int64
	}{
		{name: "whitespace padded platform", platform: " openai ", groupID: &positive, wantPlatform: "openai", wantGroupID: 42},
		{name: "nil group", platform: " openai ", groupID: nil, wantPlatform: "openai"},
		{name: "zero group", platform: " openai ", groupID: &zero, wantPlatform: "openai"},
		{name: "negative group", platform: " openai ", groupID: &negative, wantPlatform: "openai", wantGroupID: -7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			optimized := &opsAccountStatsRepoStub{accounts: []Account{{ID: 7}}}
			optimizedAccounts, err := (&OpsService{accountRepo: optimized}).listAllAccountsForOps(context.Background(), tt.platform, tt.groupID)
			require.NoError(t, err)
			require.Equal(t, []Account{{ID: 7}}, optimizedAccounts)
			require.Equal(t, tt.wantPlatform, optimized.platformFilter)
			require.Same(t, tt.groupID, optimized.groupIDFilter)
			require.Equal(t, 1, optimized.listCalls)

			fallback := &opsAccountFallbackRepoStub{}
			fallbackAccounts, err := (&OpsService{accountRepo: fallback}).listAllAccountsForOps(context.Background(), tt.platform, tt.groupID)
			require.NoError(t, err)
			require.Equal(t, []Account{{ID: 9}}, fallbackAccounts)
			require.Equal(t, []AccountListFilters{{Platform: tt.wantPlatform, GroupID: tt.wantGroupID}}, fallback.filters)
		})
	}
}
