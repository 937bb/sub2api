package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

type channelSelectionAccountRepo struct {
	AccountRepository
	accounts []Account
}

func (r *channelSelectionAccountRepo) ListSchedulableByGroupIDAndPlatform(context.Context, int64, string) ([]Account, error) {
	return r.accounts, nil
}

func TestOpenAISelectionChannelRestrictionReachesSchedulers(t *testing.T) {
	channelSvc := &ChannelService{}
	channelSvc.cache.Store(populateChannelCache([]Channel{{
		ID: 6, Status: StatusActive, GroupIDs: []int64{36}, RestrictModels: true,
		BillingModelSource: BillingModelSourceUpstream,
		ModelPricing:       []ChannelModelPricing{{Platform: PlatformOpenAI, Models: []string{"gemini-3-pro"}}},
	}}, map[int64]string{36: PlatformOpenAI}))
	accounts := []Account{{ID: 34476, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{"model_mapping": map[string]any{"gpt-6-astra": "gpt-6-astra"}},
	}}
	svc := &OpenAIGatewayService{channelService: channelSvc, accountRepo: &channelSelectionAccountRepo{accounts: accounts}}
	groupID := int64(36)
	_, err := svc.SelectAccountForModelWithExclusions(context.Background(), &groupID, "", "gpt-6-astra", nil)
	require.ErrorIs(t, err, ErrOpenAIChannelModelRestricted)
	require.ErrorIs(t, err, ErrNoAvailableAccounts)
	_, err = svc.SelectAccountWithLoadAwareness(context.Background(), &groupID, "", "gpt-6-astra", nil)
	require.ErrorIs(t, err, ErrOpenAIChannelModelRestricted)
	scheduler := &defaultOpenAIAccountScheduler{service: svc}
	_, _, _, _, err = scheduler.selectByLoadBalance(context.Background(), OpenAIAccountScheduleRequest{
		GroupID: &groupID, Platform: PlatformOpenAI, RequestedModel: "gpt-6-astra",
		prefetchedAccounts: accounts, prefetchedAccountsReady: true,
	})
	require.ErrorIs(t, err, ErrOpenAIChannelModelRestricted)
	require.ErrorIs(t, err, ErrNoAvailableAccounts)
}

func TestOpenAISelectionChannelRestrictionClassification(t *testing.T) {
	for _, tc := range []struct {
		name                string
		pool                int
		reasons             map[string]int
		compact, restricted bool
	}{
		{"upstream", 2, map[string]int{"channel_upstream_restricted": 2}, false, true},
		{"legacy", 1, map[string]int{"channel_restricted": 1}, false, true},
		{"mixed", 2, map[string]int{"channel_upstream_restricted": 1, "runtime_blocked": 1}, false, false},
		{"partial", 2, map[string]int{"channel_upstream_restricted": 1}, false, false},
		{"empty", 0, nil, false, false},
		{"rate_limit", 1, map[string]int{"model_rate_limited": 1}, false, false},
		{"compact", 1, map[string]int{"channel_upstream_restricted": 1}, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stats := openAISelectionFilterStats{pool: tc.pool, reasons: tc.reasons}
			err := fmt.Errorf("selection: %w", stats.noAvailableError("gpt-6-astra", tc.compact, ""))
			require.Equal(t, tc.restricted, errors.Is(err, ErrOpenAIChannelModelRestricted))
			if tc.compact {
				require.ErrorIs(t, err, ErrNoAvailableCompactAccounts)
			} else {
				require.ErrorIs(t, err, ErrNoAvailableAccounts)
				require.Contains(t, err.Error(), "gpt-6-astra")
				require.Contains(t, err.Error(), stats.summary(""))
			}
		})
	}
}
