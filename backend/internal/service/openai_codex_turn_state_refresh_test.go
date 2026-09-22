package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/chatgptrelay"
	"github.com/stretchr/testify/require"
)

type turnStateRefreshScanRepo struct {
	OpenAICodexTurnStateScannerRepository
	accountIDs []int64
	pendingIDs []int64
	scans      map[openAICodexTurnStateBucketKey]*OpenAICodexTurnStateScan
}

func (r *turnStateRefreshScanRepo) GetOpenAICodexTurnStateScan(_ context.Context, accountID int64, model string) (*OpenAICodexTurnStateScan, error) {
	scan := r.scans[openAICodexTurnStateBucketKey{accountID: accountID, model: model}]
	if scan == nil {
		return nil, nil
	}
	copyScan := *scan
	return &copyScan, nil
}

func (r *turnStateRefreshScanRepo) UpsertOpenAICodexTurnStateScan(_ context.Context, scan *OpenAICodexTurnStateScan) error {
	copyScan := *scan
	r.scans[openAICodexTurnStateBucketKey{accountID: scan.AccountID, model: scan.Model}] = &copyScan
	return nil
}

func (r *turnStateRefreshScanRepo) ListOpenAICodexTurnStateProxies(context.Context, bool) ([]*OpenAICodexTurnStateProxy, error) {
	return nil, nil
}

func (r *turnStateRefreshScanRepo) ListReusableOpenAICodexTurnStateProxies(context.Context) ([]*OpenAICodexTurnStateProxy, error) {
	return nil, nil
}

func (r *turnStateRefreshScanRepo) ListRecentlyUsedOpenAICodexAccountIDs(context.Context, time.Time) ([]int64, error) {
	return r.accountIDs, nil
}

func (r *turnStateRefreshScanRepo) ListPendingOpenAICodexTurnStateAccountIDs(context.Context) ([]int64, error) {
	return r.pendingIDs, nil
}

func (r *turnStateRefreshScanRepo) ListObservedOpenAICodexTurnStateModels(context.Context, int64, time.Time) ([]string, error) {
	return nil, nil
}

type turnStateRefreshAccountRepo struct {
	AccountRepository
	accounts map[int64]*Account
}

func (r *turnStateRefreshAccountRepo) GetByID(_ context.Context, accountID int64) (*Account, error) {
	return r.accounts[accountID], nil
}

func (r *turnStateRefreshAccountRepo) GetByIDs(_ context.Context, accountIDs []int64) ([]*Account, error) {
	accounts := make([]*Account, 0, len(accountIDs))
	for _, accountID := range accountIDs {
		if account := r.accounts[accountID]; account != nil {
			accounts = append(accounts, account)
		}
	}
	return accounts, nil
}

func turnStateRefreshAccount(accountID int64, now time.Time) *Account {
	account := ticketTestAccount(accountID)
	account.Status = StatusActive
	account.Schedulable = true
	account.LastUsedAt = &now
	return account
}

func turnStateRefreshRecord(t *testing.T, pool *openAICodexTurnStatePool, accountID int64, model, value string) *OpenAICodexTurnStateRecord {
	t.Helper()
	pool.mu.RLock()
	defer pool.mu.RUnlock()
	for _, record := range pool.entries {
		if record.SourceAccountID != nil && *record.SourceAccountID == accountID && record.SourceModel == model && record.StateValue == value {
			return cloneOpenAICodexTurnStateRecord(record)
		}
	}
	t.Fatalf("turn-state record not found for account %d model %s", accountID, model)
	return nil
}

func TestOpenAICodexTurnStateRefreshRejectsOldResultThenAcceptsFreshResult(t *testing.T) {
	for _, tc := range []struct {
		name   string
		length int
		age    time.Duration
	}{
		{name: "expiring 332", length: openAICodexTurnStateLength332, age: 210 * time.Second},
		{name: "expired 332", length: openAICodexTurnStateLength332, age: 5 * time.Minute},
		{name: "expiring 292", length: openAICodexTurnStateLength292, age: 210 * time.Second},
		{name: "expired 292", length: openAICodexTurnStateLength292, age: 5 * time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now().UTC().Truncate(time.Second)
			const model = "gpt-6-astra"
			const otherModel = "gpt-5.6-sol"
			accountID, otherAccountID := int64(41), int64(42)
			account := turnStateRefreshAccount(accountID, now)
			oldIssuedAt := now.Add(-tc.age)
			oldState := testOpenAICodexTurnState(tc.length, oldIssuedAt, 'o')
			freshIssuedAt := now.Add(-30 * time.Second)
			freshState := testOpenAICodexTurnState(tc.length, freshIssuedAt, 'n')
			responseState := oldState
			calls := 0
			upstream := &codexTicketFuncUpstream{do: func(*http.Request) (*http.Response, error) {
				calls++
				header := http.Header{}
				header.Set(openAICodexTurnStateHeader, responseState)
				header.Set("OpenAI-Model", model)
				return &http.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(strings.NewReader(""))}, nil
			}}
			gateway := ticketTestService(t, config.OpenAICodexTicketConfig{Models: []string{model}}, upstream)
			pool := gateway.getOpenAICodexTurnStatePool()
			pool.now = func() time.Time { return now }
			pool.observe(oldState, &accountID, "old-session", model, "scanner")
			oldRecord := turnStateRefreshRecord(t, pool, accountID, model, oldState)
			otherAccountState := testOpenAICodexTurnState(tc.length, now.Add(-time.Minute), 'a')
			otherModelState := testOpenAICodexTurnState(tc.length, now.Add(-time.Minute), 'm')
			pool.observe(otherAccountState, &otherAccountID, "other-account-session", model, "scanner")
			pool.observe(otherModelState, &accountID, "other-model-session", otherModel, "scanner")
			lastSuccess := oldIssuedAt
			repo := &turnStateRefreshScanRepo{scans: map[openAICodexTurnStateBucketKey]*OpenAICodexTurnStateScan{
				{accountID: accountID, model: model}: {
					AccountID: accountID, Model: model, Status: "ready", AttemptCount: 1, LastSuccessAt: &lastSuccess,
				},
			}}
			scanner := newOpenAICodexTurnStateScanner(repo, &turnStateRefreshAccountRepo{accounts: map[int64]*Account{accountID: account}}, gateway)
			job := openAICodexTurnStateScanJob{accountID: accountID, model: model}

			scanner.runJob(context.Background(), job)

			retried, err := repo.GetOpenAICodexTurnStateScan(context.Background(), accountID, model)
			require.NoError(t, err)
			require.Equal(t, 2, calls)
			require.Equal(t, "retry_wait", retried.Status)
			require.Equal(t, 2, retried.AttemptCount)
			require.NotEmpty(t, retried.LastError)
			require.Equal(t, &lastSuccess, retried.LastSuccessAt, "receiving an old state must not count as a successful refresh")
			require.NotNil(t, retried.NextAttemptAt)
			require.True(t, retried.NextAttemptAt.After(time.Now()))
			require.WithinDuration(t, time.Now(), *retried.NextAttemptAt, openAICodexTurnStateScanRetryMax+time.Second)
			replayedRecord := turnStateRefreshRecord(t, pool, accountID, model, oldState)
			require.Equal(t, oldRecord.ExpiresAt, replayedRecord.ExpiresAt, "a repeated state must not extend its signed lifetime")
			require.Equal(t, oldRecord.LastSeenAt, replayedRecord.LastSeenAt)

			responseState = freshState
			backoffElapsed := time.Now().Add(-time.Second)
			repo.scans[openAICodexTurnStateBucketKey{accountID: accountID, model: model}].NextAttemptAt = &backoffElapsed
			scanner.runJob(context.Background(), job)

			ready, err := repo.GetOpenAICodexTurnStateScan(context.Background(), accountID, model)
			require.NoError(t, err)
			require.Equal(t, 4, calls)
			require.Equal(t, "ready", ready.Status)
			require.Equal(t, 3, ready.AttemptCount)
			require.Empty(t, ready.LastError)
			require.NotNil(t, ready.LastSuccessAt)
			require.True(t, ready.LastSuccessAt.After(lastSuccess))
			require.NotNil(t, ready.NextAttemptAt)
			require.Equal(t, freshIssuedAt.Add(openAICodexTurnStateProactiveRefreshAfter), ready.NextAttemptAt.UTC())
			selected, ok := pool.preferredForBucket(accountID, model)
			require.True(t, ok)
			require.Equal(t, freshState, selected)
			expiresAt, ok := pool.preferredExpiryForBucket(accountID, model)
			require.True(t, ok)
			require.Equal(t, freshIssuedAt.Add(openAICodexTurnStateTTL), expiresAt)

			selected, ok = pool.preferredForBucket(otherAccountID, model)
			require.True(t, ok)
			require.Equal(t, otherAccountState, selected)
			selected, ok = pool.preferredForBucket(accountID, otherModel)
			require.True(t, ok)
			require.Equal(t, otherModelState, selected)
			require.Len(t, repo.scans, 1, "refreshing one bucket must not change another account or model's scan status")

			scanner.runJob(context.Background(), job)
			require.Equal(t, 4, calls, "a state with sufficient remaining lifetime must not trigger another probe")
		})
	}
}

func TestOpenAICodexTurnStateRefreshSweepUsesPoolExpiryAndPreservesRetryBackoff(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     string
		stateAge   time.Duration
		nextOffset time.Duration
		wantQueued bool
	}{
		{name: "missing state ignores stale ready deadline", status: "ready", nextOffset: 40 * time.Minute, wantQueued: true},
		{name: "expiring state ignores stale ready deadline", status: "ready", stateAge: 210 * time.Second, nextOffset: 40 * time.Minute, wantQueued: true},
		{name: "expired state ignores stale ready deadline", status: "ready", stateAge: 5 * time.Minute, nextOffset: 40 * time.Minute, wantQueued: true},
		{name: "usable state skips overdue scan", status: "ready", stateAge: time.Minute, nextOffset: -time.Minute},
		{name: "missing state respects retry backoff", status: "retry_wait", nextOffset: time.Minute},
		{name: "expiring state respects retry backoff", status: "retry_wait", stateAge: 210 * time.Second, nextOffset: time.Minute},
		{name: "expired retry backoff queues missing state", status: "retry_wait", nextOffset: -time.Minute, wantQueued: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now().UTC().Truncate(time.Second)
			const model = "gpt-6-astra"
			accountID := int64(41)
			account := turnStateRefreshAccount(accountID, now)
			nextAttempt := now.Add(tc.nextOffset)
			repo := &turnStateRefreshScanRepo{
				accountIDs: []int64{accountID},
				scans: map[openAICodexTurnStateBucketKey]*OpenAICodexTurnStateScan{
					{accountID: accountID, model: model}: {AccountID: accountID, Model: model, Status: tc.status, NextAttemptAt: &nextAttempt},
				},
			}
			gateway := ticketTestService(t, config.OpenAICodexTicketConfig{Models: []string{model}}, nil)
			if tc.stateAge > 0 {
				state := testOpenAICodexTurnState(openAICodexTurnStateLength332, now.Add(-tc.stateAge), 's')
				gateway.getOpenAICodexTurnStatePool().observe(state, &accountID, "session", model, "scanner")
			}
			scanner := newOpenAICodexTurnStateScanner(repo, &turnStateRefreshAccountRepo{accounts: map[int64]*Account{accountID: account}}, gateway)

			scanner.enqueueSweep(context.Background())

			if !tc.wantQueued {
				require.Empty(t, scanner.queue)
				return
			}
			require.Len(t, scanner.queue, 1)
			require.Equal(t, openAICodexTurnStateScanJob{accountID: accountID, model: model}, <-scanner.queue)
		})
	}
}

func TestOpenAICodexTurnStateRefreshKeepsHealthy292FallbackUntilRefreshWindow(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	const model = "gpt-6-astra"
	accountID := int64(41)
	account := turnStateRefreshAccount(accountID, now)
	calls := 0
	upstream := &codexTicketFuncUpstream{do: func(*http.Request) (*http.Response, error) {
		calls++
		return nil, io.EOF
	}}
	gateway := ticketTestService(t, config.OpenAICodexTicketConfig{Models: []string{model}}, upstream)
	pool := gateway.getOpenAICodexTurnStatePool()
	pool.observe(testOpenAICodexTurnState(openAICodexTurnStateLength332, now.Add(-210*time.Second), 'a'), &accountID, "older-session", model, "scanner")
	pool.observe(testOpenAICodexTurnState(openAICodexTurnStateLength292, now.Add(-time.Minute), 'b'), &accountID, "newer-session", model, "scanner")
	repo := &turnStateRefreshScanRepo{
		accountIDs: []int64{accountID},
		scans:      make(map[openAICodexTurnStateBucketKey]*OpenAICodexTurnStateScan),
	}
	scanner := newOpenAICodexTurnStateScanner(repo, &turnStateRefreshAccountRepo{accounts: map[int64]*Account{accountID: account}}, gateway)

	scanner.runJob(context.Background(), openAICodexTurnStateScanJob{accountID: accountID, model: model})
	scanner.enqueueSweep(context.Background())

	require.Zero(t, calls, "a usable configured fallback must not trigger repeated acquisition")
	require.Empty(t, scanner.queue)
	scan := repo.scans[openAICodexTurnStateBucketKey{accountID: accountID, model: model}]
	require.Nil(t, scan)
}

func TestOpenAICodexTurnStateProactiveRefreshKeepsCurrentTicketReusable(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	const model = "gpt-6-astra"
	accountID := int64(41)
	gateway := ticketTestService(t, config.OpenAICodexTicketConfig{Models: []string{model}}, nil)
	pool := gateway.getOpenAICodexTurnStatePool()
	pool.now = func() time.Time { return now }
	state := testOpenAICodexTurnState(openAICodexTurnStateLength332, now.Add(-91*time.Second), 'p')
	pool.observe(state, &accountID, "session", model, "scanner")
	scanner := &openAICodexTurnStateScanner{gateway: gateway}

	require.True(t, scanner.stateNeedsRefresh(accountID, model, now))
	selected, reusable := pool.preferredForBucket(accountID, model)
	require.True(t, reusable)
	require.Equal(t, state, selected)
}

func TestOpenAICodexTurnStateRefreshPrefersBoundCookieRouteBeforeFallback(t *testing.T) {
	for _, testCase := range []struct {
		name          string
		firstSucceeds bool
		wantCalls     int
	}{
		{name: "bound route renews ticket", firstSucceeds: true, wantCalls: 1},
		{name: "bound route failure falls back", firstSucceeds: false, wantCalls: 2},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			now := time.Now().UTC().Truncate(time.Second)
			const model = "gpt-6-astra"
			const accountID int64 = 41
			account := turnStateRefreshAccount(accountID, now)
			gateway := ticketTestService(t, config.OpenAICodexTicketConfig{Models: []string{model}}, nil)
			pool := gateway.getOpenAICodexTurnStatePool()
			pool.now = func() time.Time { return now }
			oldState := testOpenAICodexTurnState(openAICodexTurnStateLength332, now.Add(-210*time.Second), 'o')
			require.NoError(t, pool.observeDurably(
				context.Background(), oldState, accountID, "old-session-hash", model, "scanner",
				openAICodexTurnStateRouteTicket{
					SessionID:   "old-session",
					RouteIPv6:   "2a02:ae02:1a:2c00::1234",
					RouteCookie: "__cflb=old-route",
				},
			))
			repo := &turnStateRefreshScanRepo{scans: make(map[openAICodexTurnStateBucketKey]*OpenAICodexTurnStateScan)}
			scanner := newOpenAICodexTurnStateScanner(repo, &turnStateRefreshAccountRepo{accounts: map[int64]*Account{accountID: account}}, gateway)
			var refreshRoutes []openAICodexTurnStateRefreshRoute
			var routeIPv6s []string
			scanner.probe = func(ctx context.Context, _ *Account, requestedModel, _ string) openAICodexTurnStateHarvestResult {
				refresh, _ := ctx.Value(openAICodexTurnStateRefreshRouteContextKey{}).(openAICodexTurnStateRefreshRoute)
				refreshRoutes = append(refreshRoutes, refresh)
				routeIPv6s = append(routeIPv6s, chatgptrelay.SourceIPv6FromContext(ctx))
				if len(refreshRoutes) == 1 && !testCase.firstSucceeds {
					return openAICodexTurnStateHarvestResult{errorMessage: "bound route failed"}
				}
				result := stateConcurrencyResult(openAICodexTurnStateLength332, requestedModel)
				result.routeCookie = "__cflb=renewed-route"
				return result
			}

			scanner.runJob(context.Background(), openAICodexTurnStateScanJob{accountID: accountID, model: model})

			require.Len(t, refreshRoutes, testCase.wantCalls)
			require.Equal(t, openAICodexTurnStateRefreshRoute{sessionID: "old-session", routeCookie: "__cflb=old-route"}, refreshRoutes[0])
			require.Equal(t, "2a02:ae02:1a:2c00::1234", routeIPv6s[0])
			if !testCase.firstSucceeds {
				require.Empty(t, refreshRoutes[1])
				require.Empty(t, routeIPv6s[1])
			}
			record, ok := pool.preferredRecordForBucket(accountID, model)
			require.True(t, ok)
			require.Equal(t, "__cflb=renewed-route", record.RouteCookie)
		})
	}
}

func TestOpenAICodexTurnStateSweepQueuesNewPendingAccountWithoutUsage(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	const model = "gpt-6-astra"
	const accountID int64 = 41
	account := turnStateRefreshAccount(accountID, now)
	account.LastUsedAt = nil
	account.Extra = map[string]any{openAICodexStateRoutingRequiredExtraKey: true}
	repo := &turnStateRefreshScanRepo{
		pendingIDs: []int64{accountID},
		scans:      make(map[openAICodexTurnStateBucketKey]*OpenAICodexTurnStateScan),
	}
	gateway := ticketTestService(t, config.OpenAICodexTicketConfig{Models: []string{model}}, nil)
	scanner := newOpenAICodexTurnStateScanner(repo, &turnStateRefreshAccountRepo{accounts: map[int64]*Account{accountID: account}}, gateway)

	scanner.enqueueSweep(context.Background())

	require.Len(t, scanner.queue, 1)
	require.Equal(t, openAICodexTurnStateScanJob{accountID: accountID, model: model}, <-scanner.queue)
}
