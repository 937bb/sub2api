package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

type siwcLocalUsageRepo struct {
	usageBatchLogRepoStub
	starts     []time.Time
	accountIDs []int64
	fail       bool
	stats      usagestats.AccountStats
}

func (r *siwcLocalUsageRepo) GetAccountWindowStats(_ context.Context, id int64, start time.Time) (*usagestats.AccountStats, error) {
	r.starts = append(r.starts, start)
	r.accountIDs = append(r.accountIDs, id)
	if r.fail && len(r.starts)%2 == 1 {
		return nil, errors.New("database unavailable")
	}
	stats := r.stats
	return &stats, nil
}

func TestSIWCLocalUsageSurvivesMissingWebToken(t *testing.T) {
	for _, batch := range []bool{false, true} {
		t.Run(map[bool]string{false: "single", true: "batch"}[batch], func(t *testing.T) {
			account := newSIWCIsolationAccount()
			repo := &siwcLocalUsageRepo{stats: usagestats.AccountStats{
				Requests: 150, Tokens: 4565559, Cost: 18.178916, StandardCost: 18.178916, UserCost: 1.70135049,
			}}
			svc := &AccountUsageService{accountRepo: &stubOpenAIAccountRepo{accounts: []Account{*account}}, usageLogRepo: repo, cache: NewUsageCache()}
			start := time.Now()
			var usage *UsageInfo
			if batch {
				result, failures, err := svc.GetUsageBatch(context.Background(), []int64{account.ID}, true)
				require.NoError(t, err)
				require.Empty(t, failures)
				usage = result[account.ID]
			} else {
				var err error
				usage, err = svc.GetUsage(context.Background(), account.ID, true)
				require.NoError(t, err)
			}
			require.NotNil(t, usage)
			require.Equal(t, "siwc_quota_web_token_required", usage.ErrorCode)
			require.Nil(t, usage.FiveHour, "local accounting must not fabricate remote quota")
			require.Nil(t, usage.SevenDay)
			require.Empty(t, usage.LocalUsageError)
			for _, window := range []*LocalUsageWindow{usage.LocalFiveHour, usage.LocalSevenDay} {
				require.NotNil(t, window)
				require.True(t, window.Rolling)
				require.Equal(t, int64(150), window.Stats.Requests)
				require.Equal(t, int64(4565559), window.Stats.Tokens)
				require.Equal(t, 18.178916, window.Stats.Cost)
				require.Equal(t, 1.70135049, window.Stats.UserCost)
			}
			require.WithinDuration(t, start.Add(-5*time.Hour), usage.LocalFiveHour.StartAt, time.Second)
			require.WithinDuration(t, start.Add(-7*24*time.Hour), usage.LocalSevenDay.StartAt, time.Second)
			require.Equal(t, []int64{account.ID, account.ID}, repo.accountIDs)
		})
	}
}

func TestSIWCLocalUsageRefreshesWithCachedOrRejectedQuota(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusUnauthorized} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			account := siwcWebQuotaAccount()
			quotaCalls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				quotaCalls++
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = w.Write([]byte(siwcWebQuotaFixture))
			}))
			defer srv.Close()
			repo := &siwcLocalUsageRepo{stats: usagestats.AccountStats{Requests: 150, Tokens: 4565559, Cost: 18.178916, UserCost: 1.70135049}}
			svc := &AccountUsageService{cache: NewUsageCache(), usageLogRepo: repo,
				openAIQuotaService: &OpenAIQuotaService{privacyClientFactory: newQuotaRedirectingFactory(srv)}}
			first, err := svc.getOpenAIUsage(context.Background(), account, false)
			require.NoError(t, err)
			require.Equal(t, int64(150), first.LocalFiveHour.Stats.Requests)
			repo.stats.Requests = 151
			repo.stats.UserCost = 2.5
			second, err := svc.getOpenAIUsage(context.Background(), account, false)
			require.NoError(t, err)
			require.Equal(t, 1, quotaCalls)
			require.Len(t, repo.starts, 4)
			require.Equal(t, int64(151), second.LocalFiveHour.Stats.Requests)
			require.Equal(t, 2.5, second.LocalSevenDay.Stats.UserCost)
			require.Equal(t, 1.70135049, first.LocalFiveHour.Stats.UserCost)
			if status == http.StatusOK {
				require.False(t, second.LocalFiveHour.Rolling)
				require.False(t, second.LocalSevenDay.Rolling)
				require.Equal(t, second.FiveHour.ResetsAt.Add(-5*time.Hour), second.LocalFiveHour.StartAt)
				require.Equal(t, second.SevenDay.ResetsAt.Add(-7*24*time.Hour), second.LocalSevenDay.StartAt)
				require.Equal(t, 98.0, second.FiveHour.Utilization)
			} else {
				require.True(t, second.LocalFiveHour.Rolling)
				require.Nil(t, second.FiveHour)
				require.Equal(t, "siwc_quota_web_token_invalid", second.ErrorCode)
			}
		})
	}
}

func TestSIWCLocalUsageReadFailureIsNotZero(t *testing.T) {
	repo := &siwcLocalUsageRepo{fail: true, stats: usagestats.AccountStats{Requests: 150, Cost: 18}}
	svc := &AccountUsageService{usageLogRepo: repo}
	usage, err := svc.getOpenAIUsage(context.Background(), newSIWCIsolationAccount(), false)
	require.NoError(t, err)
	require.Equal(t, "local_usage_unavailable", usage.LocalUsageError)
	require.Nil(t, usage.LocalFiveHour)
	require.Equal(t, int64(150), usage.LocalSevenDay.Stats.Requests)
	require.Equal(t, 18.0, usage.LocalSevenDay.Stats.Cost)
	require.Nil(t, usage.FiveHour)
	require.Equal(t, "siwc_quota_web_token_required", usage.ErrorCode)
}
