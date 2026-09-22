package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/chatgptrelay"
	"github.com/stretchr/testify/require"
)

func TestRandomOpenAICodexRouteIPv6sAreUniqueAndInsidePrefix(t *testing.T) {
	prefix := netip.MustParsePrefix("2a02:ae02:1a:2c00::/64")
	values, err := randomOpenAICodexRouteIPv6s(prefix, 5)
	require.NoError(t, err)
	require.Len(t, values, 5)
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		addr := netip.MustParseAddr(value)
		require.True(t, prefix.Contains(addr))
		require.NotEqual(t, prefix.Addr(), addr)
		_, exists := seen[value]
		require.False(t, exists)
		seen[value] = struct{}{}
	}
}

type codexTurnStateHarvestCaptureUpstream struct {
	HTTPUpstream
	responses []*http.Response
	requests  []*http.Request
	proxies   []string
}

func (u *codexTurnStateHarvestCaptureUpstream) Do(req *http.Request, proxyURL string, _ int64, _ int) (*http.Response, error) {
	u.requests = append(u.requests, req)
	u.proxies = append(u.proxies, proxyURL)
	response := u.responses[0]
	u.responses = u.responses[1:]
	return response, nil
}

func codexTurnStateHarvestResponse(state, model string) *http.Response {
	header := http.Header{}
	if state != "" {
		header.Set(openAICodexTurnStateHeader, state)
	}
	header.Set("OpenAI-Model", model)
	return &http.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(strings.NewReader(""))}
}

func TestNormalizeOpenAICodexTurnStateProxyURLAndMask(t *testing.T) {
	normalized, err := normalizeOpenAICodexTurnStateProxyURL("proxy.example:8443:scanner:secret")
	require.NoError(t, err)
	require.Equal(t, "http", strings.SplitN(normalized, ":", 2)[0])
	require.Contains(t, normalized, "scanner:secret@proxy.example:8443")

	masked := maskOpenAICodexTurnStateProxyURL(normalized)
	require.Contains(t, masked, "scanner:%2A%2A%2A@proxy.example:8443")
	require.NotContains(t, masked, "secret")

	_, err = normalizeOpenAICodexTurnStateProxyURL("ftp://proxy.example:21")
	require.Error(t, err)
}

func TestMergeOpenAICodexTurnStateScanProxiesDeduplicatesAndCopies(t *testing.T) {
	dedicated := &OpenAICodexTurnStateProxy{ID: 7, Source: "state", ProxyURL: "http://user:pass@proxy.example:8080"}
	sharedDuplicate := &OpenAICodexTurnStateProxy{Source: "shared", SourceID: 9, ProxyURL: "http://user:pass@proxy.example:8080"}
	sharedUnique := &OpenAICodexTurnStateProxy{Source: "shared", SourceID: 10, ProxyURL: "socks5://127.0.0.1:1080"}
	invalid := &OpenAICodexTurnStateProxy{Source: "shared", ProxyURL: "invalid"}

	merged := mergeOpenAICodexTurnStateScanProxies(
		[]*OpenAICodexTurnStateProxy{dedicated},
		[]*OpenAICodexTurnStateProxy{sharedDuplicate, sharedUnique, invalid},
	)

	require.Len(t, merged, 2)
	require.Equal(t, int64(7), merged[0].ID)
	require.Equal(t, int64(10), merged[1].SourceID)
	require.NotSame(t, dedicated, merged[0])
	require.Equal(t, "http://user:pass@proxy.example:8080", dedicated.ProxyURL)
}

func TestOpenAICodexTurnStateProxyIndexRotatesPerAccountModelAttempt(t *testing.T) {
	const proxyCount = 10

	for _, model := range []string{"gpt-5.5", "gpt-5.6-sol", "gpt-5.6-terra", "gpt-6-astra"} {
		seen := make(map[int]struct{}, proxyCount)
		for attempt := 1; attempt <= proxyCount; attempt++ {
			index := openAICodexTurnStateProxyIndex(62742, model, attempt, proxyCount)
			require.GreaterOrEqual(t, index, 0)
			require.Less(t, index, proxyCount)
			require.NotContains(t, seen, index, "model %s reused a proxy before completing a full rotation", model)
			seen[index] = struct{}{}
		}
		require.Len(t, seen, proxyCount)
	}

	require.Equal(t,
		openAICodexTurnStateProxyIndex(62742, "gpt-5.5", 1, proxyCount),
		openAICodexTurnStateProxyIndex(62742, "gpt-5.5", proxyCount+1, proxyCount),
	)
	require.Equal(t, 0, openAICodexTurnStateProxyIndex(62742, "gpt-5.5", 1, 0))
}

func TestOpenAICodexTurnStateProxyWindowAdvancesByFanout(t *testing.T) {
	proxies := make([]*OpenAICodexTurnStateProxy, 15)
	for index := range proxies {
		id := int64(index + 1)
		proxies[index] = &OpenAICodexTurnStateProxy{
			ID:       id,
			Source:   "state",
			SourceID: id,
			ProxyURL: fmt.Sprintf("http://127.0.0.1:%d", 17890+id),
		}
	}

	seen := make(map[int64]struct{}, len(proxies))
	for attempt := 1; attempt <= 3; attempt++ {
		selected := selectOpenAICodexTurnStateProxyWindow(proxies, 62742, "gpt-6-astra", attempt, 5)
		require.Len(t, selected, 5)
		for _, proxy := range selected {
			require.NotContains(t, seen, proxy.ID, "attempt %d reused an exit before covering the pool", attempt)
			seen[proxy.ID] = struct{}{}
		}
	}
	require.Len(t, seen, len(proxies))
}

func TestSortOpenAICodexTurnStateScanProxiesUsesStableIdentity(t *testing.T) {
	proxies := []*OpenAICodexTurnStateProxy{
		{Source: "shared", SourceID: 2, ProxyURL: "http://shared-2.example"},
		{ID: 8, Source: "state", SourceID: 8, ProxyURL: "http://state-8.example"},
		{ID: 3, Source: "state", SourceID: 3, ProxyURL: "http://state-3.example"},
		{Source: "shared", SourceID: 1, ProxyURL: "http://shared-1.example"},
	}

	sortOpenAICodexTurnStateScanProxies(proxies)

	require.Equal(t, []string{
		"http://state-3.example",
		"http://state-8.example",
		"http://shared-1.example",
		"http://shared-2.example",
	}, []string{proxies[0].ProxyURL, proxies[1].ProxyURL, proxies[2].ProxyURL, proxies[3].ProxyURL})
}

func TestOpenAICodexTurnStateRetryDelayCapsAtFiveMinutes(t *testing.T) {
	require.Equal(t, 5*time.Second, openAICodexTurnStateRetryDelay(1))
	require.Equal(t, 10*time.Second, openAICodexTurnStateRetryDelay(2))
	require.Equal(t, 40*time.Second, openAICodexTurnStateRetryDelay(4))
	require.Equal(t, 5*time.Minute, openAICodexTurnStateRetryDelay(7))
	require.Equal(t, 5*time.Minute, openAICodexTurnStateRetryDelay(100))
}

func TestIsRecentlyUsedOpenAICodexAccount(t *testing.T) {
	now := time.Date(2026, 9, 18, 18, 0, 0, 0, time.UTC)
	recent := now.Add(-30 * time.Minute)
	old := now.Add(-openAICodexTurnStateActiveUsageWindow - time.Second)
	future := now.Add(time.Minute)
	past := now.Add(-time.Minute)
	parentID := int64(1)

	base := Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		LastUsedAt:  &recent,
	}
	require.True(t, isRecentlyUsedOpenAICodexAccount(&base, now))

	cases := []struct {
		name   string
		mutate func(*Account)
	}{
		{name: "never used", mutate: func(account *Account) { account.LastUsedAt = nil }},
		{name: "used outside window", mutate: func(account *Account) { account.LastUsedAt = &old }},
		{name: "disabled", mutate: func(account *Account) { account.Status = StatusDisabled }},
		{name: "unschedulable", mutate: func(account *Account) { account.Schedulable = false }},
		{name: "child account", mutate: func(account *Account) { account.ParentAccountID = &parentID }},
		{name: "api key", mutate: func(account *Account) { account.Type = AccountTypeAPIKey }},
		{name: "expired", mutate: func(account *Account) { account.ExpiresAt = &past }},
		{name: "temporarily unavailable", mutate: func(account *Account) { account.TempUnschedulableUntil = &future }},
		{name: "overloaded", mutate: func(account *Account) { account.OverloadUntil = &future }},
		{name: "rate limited", mutate: func(account *Account) { account.RateLimitResetAt = &future }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			account := base
			tc.mutate(&account)
			require.False(t, isRecentlyUsedOpenAICodexAccount(&account, now))
		})
	}
}

func TestManualCodexTurnStateScanAllowsUnusedEligibleAccount(t *testing.T) {
	now := time.Date(2026, 9, 18, 18, 0, 0, 0, time.UTC)
	account := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
	}

	require.True(t, isEligibleOpenAICodexTurnStateAccount(account, now))
	require.False(t, isRecentlyUsedOpenAICodexAccount(account, now))
}

func TestStrictRouteBindingRunsDemandJobForDormantLegacyAccount(t *testing.T) {
	now := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	lastUsedAt := now.Add(-openAICodexTurnStateActiveUsageWindow - time.Minute)
	account := &Account{
		ID:          42,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		LastUsedAt:  &lastUsedAt,
	}
	job := openAICodexTurnStateScanJob{accountID: account.ID, model: "gpt-6-astra"}

	legacy := defaultOpenAICodexTurnStateScanSettings()
	require.False(t, shouldRunOpenAICodexTurnStateScanJob(account, job, legacy, now))

	requireRouteBinding := true
	strict := legacy.clone()
	strict.RequireRouteBinding = &requireRouteBinding
	require.True(t, shouldRunOpenAICodexTurnStateScanJob(account, job, strict, now))
	require.False(t, isRecentlyUsedOpenAICodexAccount(account, now), "periodic sweep must still exclude the dormant account")
}

func TestStrictRouteBindingStillRejectsIneligibleDemandJob(t *testing.T) {
	now := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	requireRouteBinding := true
	settings := defaultOpenAICodexTurnStateScanSettings()
	settings.RequireRouteBinding = &requireRouteBinding
	job := openAICodexTurnStateScanJob{accountID: 42, model: "gpt-6-astra"}

	for _, account := range []*Account{
		{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true},
		{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusDisabled, Schedulable: true},
		{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: false},
	} {
		require.False(t, shouldRunOpenAICodexTurnStateScanJob(account, job, settings, now))
	}
}

func TestOpenAICodexTurnStateScanRouteModes(t *testing.T) {
	settings := defaultOpenAICodexTurnStateScanSettings()
	settings.DynamicProxyEnabled = true
	require.Equal(t, openAICodexTurnStateScanRouteDynamicProxy, selectOpenAICodexTurnStateScanRoute(settings, true))

	settings.ScanRouteMode = OpenAICodexTurnStateScanRouteManagedProxy
	require.Equal(t, openAICodexTurnStateScanRouteManagedProxy, selectOpenAICodexTurnStateScanRoute(settings, true))
	settings.ScanRouteMode = OpenAICodexTurnStateScanRouteIPv6
	require.Equal(t, openAICodexTurnStateScanRouteIPv6, selectOpenAICodexTurnStateScanRoute(settings, false))
	settings.ScanRouteMode = OpenAICodexTurnStateScanRouteDynamicProxy
	require.Equal(t, openAICodexTurnStateScanRouteDynamicProxy, selectOpenAICodexTurnStateScanRoute(settings, false))

	settings.ScanRouteMode = OpenAICodexTurnStateScanRouteAuto
	settings.DynamicProxyEnabled = false
	require.Equal(t, openAICodexTurnStateScanRouteIPv6, selectOpenAICodexTurnStateScanRoute(settings, true))
	require.Equal(t, openAICodexTurnStateScanRouteProxy, selectOpenAICodexTurnStateScanRoute(settings, false))
}

func TestOpenAICodexTurnStateManagedProxyModeExcludesSharedProxies(t *testing.T) {
	repo := &turnStateRefreshScanRepo{
		dedicated: []*OpenAICodexTurnStateProxy{{ID: 7, Source: "state", ProxyURL: "http://127.0.0.1:17891", Enabled: true}},
		shared:    []*OpenAICodexTurnStateProxy{{ID: 9, Source: "shared", ProxyURL: "http://account-proxy.example:8080", Enabled: true}},
	}
	scanner := &openAICodexTurnStateScanner{repo: repo}
	selected, err := scanner.selectManagedProxies(context.Background(), 42, "gpt-6-astra", 1, 5)
	require.NoError(t, err)
	require.Len(t, selected, 1)
	require.Equal(t, int64(7), selected[0].ID)
	require.Equal(t, "state", selected[0].Source)
}

func TestOpenAICodexTurnStateScannerDeduplicatesAccountModelJobs(t *testing.T) {
	scanner := &openAICodexTurnStateScanner{
		queue:    make(chan openAICodexTurnStateScanJob, 4),
		inFlight: make(map[openAICodexTurnStateBucketKey]struct{}),
	}

	require.True(t, scanner.Enqueue(42, " GPT-5.5 ", false))
	require.False(t, scanner.Enqueue(42, "gpt-5.5", true))
	require.True(t, scanner.Enqueue(42, "gpt-5.6-sol", false))
	require.True(t, scanner.Enqueue(43, "gpt-5.5", false))

	key, ok := newOpenAICodexTurnStateBucketKey(42, "gpt-5.5")
	require.True(t, ok)
	scanner.finish(key)
	require.True(t, scanner.Enqueue(42, "gpt-5.5", false))
}

func TestOpenAICodexTurnStateScannerTargetsConfiguredModels(t *testing.T) {
	gateway := &OpenAIGatewayService{cfg: &config.Config{}}
	gateway.cfg.Gateway.OpenAICodexTicket.Models = []string{" GPT-6-ASTRA ", "gpt-5.6-sol", "gpt-6-astra", ""}
	scanner := &openAICodexTurnStateScanner{
		gateway:  gateway,
		queue:    make(chan openAICodexTurnStateScanJob, 4),
		inFlight: make(map[openAICodexTurnStateBucketKey]struct{}),
	}

	require.Equal(t, []string{"gpt-6-astra", "gpt-5.6-sol"}, scanner.targetModels())
	require.Equal(t, 2, scanner.enqueueModels(42, scanner.targetModels(), true))

	first := <-scanner.queue
	second := <-scanner.queue
	require.Equal(t, int64(42), first.accountID)
	require.Equal(t, "gpt-6-astra", first.model)
	require.True(t, first.force)
	require.Equal(t, int64(42), second.accountID)
	require.Equal(t, "gpt-5.6-sol", second.model)
	require.True(t, second.force)
}

func TestOpenAICodexTurnStateScannerOnlyQueuesMissingOrExpiringStates(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	accountID := int64(42)
	gateway := &OpenAIGatewayService{}
	pool := gateway.getOpenAICodexTurnStatePool()
	pool.now = func() time.Time { return now }
	pool.observe(testOpenAICodexTurnState(openAICodexTurnStateLength332, now.Add(-time.Minute), 'r'), &accountID, "ready-session", "gpt-6-astra", "scanner")
	pool.observe(testOpenAICodexTurnState(openAICodexTurnStateLength292, now.Add(-210*time.Second), 'e'), &accountID, "expiring-session", "gpt-5.6-sol", "scanner")

	scanner := &openAICodexTurnStateScanner{
		gateway:  gateway,
		queue:    make(chan openAICodexTurnStateScanJob, 4),
		inFlight: make(map[openAICodexTurnStateBucketKey]struct{}),
	}

	queued := scanner.enqueueModels(accountID, []string{"gpt-6-astra", "gpt-5.6-sol", "gpt-5.6-terra"}, false)
	require.Equal(t, 2, queued)

	first := <-scanner.queue
	second := <-scanner.queue
	require.Equal(t, "gpt-5.6-sol", first.model)
	require.Equal(t, "gpt-5.6-terra", second.model)
	require.False(t, first.force)
	require.False(t, second.force)
}

func TestObservedOpenAICodexTurnStateModelsExcludeNonConversationModels(t *testing.T) {
	require.True(t, isObservedOpenAICodexTurnStateModel("gpt-5.5"))
	require.True(t, isObservedOpenAICodexTurnStateModel(" GPT-6-ASTRA "))
	require.False(t, isObservedOpenAICodexTurnStateModel("gpt-image-2-auto"))
	require.False(t, isObservedOpenAICodexTurnStateModel("codex-auto-review"))
}

func TestOpenAICodexTurnStateModelsMatchRejectsMismatch(t *testing.T) {
	require.False(t, openAICodexTurnStateModelsMatch("gpt-5.5", ""))
	require.True(t, openAICodexTurnStateModelsMatch(" GPT-5.5 ", "gpt-5.5"))
	require.False(t, openAICodexTurnStateModelsMatch("gpt-5.5", "gpt-5.6-luna"))
}

func TestOpenAICodexTurnStateScanFailurePreservesUpstreamError(t *testing.T) {
	result := openAICodexTurnStateHarvestResult{errorMessage: "proxy connect failed"}
	require.Equal(t, "proxy connect failed", openAICodexTurnStateScanFailure("gpt-5.5", result, time.Now()))
}

func TestOpenAICodexTurnStateScanFailureValidatesSuccessfulProbe(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	state := testOpenAICodexTurnState(openAICodexTurnStateLength332, now, 's')
	result := openAICodexTurnStateHarvestResult{
		stateValue:    state,
		stateLength:   len(state),
		officialModel: "gpt-5.5",
		upstreamOK:    true,
		statusCode:    200,
	}
	require.Empty(t, openAICodexTurnStateScanFailure("gpt-5.5", result, now))

	result.officialModel = ""
	require.Equal(t, "upstream response did not declare an official model", openAICodexTurnStateScanFailure("gpt-5.5", result, now))

	result.officialModel = "gpt-5.6-luna"
	require.Equal(t, "upstream model mismatch: requested gpt-5.5, received gpt-5.6-luna", openAICodexTurnStateScanFailure("gpt-5.5", result, now))

	result.safetyFaster = "gpt-5.6-luna"
	require.True(t, isOpenAICodexTurnStateSafetyBuffered("gpt-5.5", result))
	require.Equal(t, "upstream safety-buffering routed gpt-5.5 to gpt-5.6-luna", openAICodexTurnStateScanFailure("gpt-5.5", result, now))
	require.Equal(t, openAICodexTurnStateSafetyRetryDelay, openAICodexTurnStateResultRetryDelay("gpt-5.5", result, 1))
}

func TestOpenAICodexTurnStateScanFailureValidatesExpiry(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		state string
		error string
	}{
		{name: "fresh 332", state: testOpenAICodexTurnState(332, now, 'a')},
		{name: "fresh 292", state: testOpenAICodexTurnState(292, now, 'b')},
		{name: "older but usable", state: testOpenAICodexTurnState(332, now.Add(-2*time.Minute), 'c')},
		{name: "malformed", state: strings.Repeat("a", 332), error: "invalid issuance timestamp"},
		{name: "future", state: testOpenAICodexTurnState(332, now.Add(time.Second), 'd'), error: "future issuance timestamp"},
		{name: "expired", state: testOpenAICodexTurnState(332, now.Add(-4*time.Minute-time.Second), 'e'), error: "already expired"},
		{name: "at expiry", state: testOpenAICodexTurnState(332, now.Add(-4*time.Minute), 'f'), error: "already expired"},
		{name: "expiring", state: testOpenAICodexTurnState(332, now.Add(-3*time.Minute-30*time.Second), 'g'), error: "refresh is still required"},
		{name: "at refresh boundary", state: testOpenAICodexTurnState(332, now.Add(-3*time.Minute), 'h'), error: "refresh is still required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			failure := openAICodexTurnStateScanFailure("gpt-5.5", openAICodexTurnStateHarvestResult{
				stateValue: tt.state, stateLength: len(tt.state), officialModel: "gpt-5.5",
				upstreamOK: true, statusCode: http.StatusOK,
			}, now)
			if tt.error == "" {
				require.Empty(t, failure)
			} else {
				require.Contains(t, failure, tt.error)
			}
		})
	}
}

func TestChooseOpenAICodexTurnStateResultPrefers332Over292(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	state292 := testOpenAICodexTurnState(openAICodexTurnStateLength292, now, 'a')
	state332 := testOpenAICodexTurnState(openAICodexTurnStateLength332, now.Add(-time.Minute), 'b')
	proxies := []*OpenAICodexTurnStateProxy{
		{ID: 1, Source: "state", ProxyURL: "http://proxy-1.example"},
		{ID: 2, Source: "state", ProxyURL: "http://proxy-2.example"},
	}
	result, proxy := chooseOpenAICodexTurnStateResult("gpt-5.5", []openAICodexTurnStateProbe{
		{proxy: proxies[0], result: openAICodexTurnStateHarvestResult{
			stateValue: state292, stateLength: len(state292), officialModel: "gpt-5.5", upstreamOK: true, statusCode: http.StatusOK,
		}},
		{proxy: proxies[1], result: openAICodexTurnStateHarvestResult{
			stateValue: state332, stateLength: len(state332), officialModel: "gpt-5.5", upstreamOK: true, statusCode: http.StatusOK,
		}},
	})

	require.Equal(t, state332, result.stateValue)
	require.Same(t, proxies[1], proxy)
}

func TestChooseOpenAICodexTurnStateResultDoesNotTreatInvalid332AsSuccess(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	state292 := testOpenAICodexTurnState(openAICodexTurnStateLength292, now, 'a')
	invalid332 := strings.Repeat("x", openAICodexTurnStateLength332)
	result, proxy := chooseOpenAICodexTurnStateResult("gpt-5.5", []openAICodexTurnStateProbe{
		{proxy: &OpenAICodexTurnStateProxy{ID: 1}, result: openAICodexTurnStateHarvestResult{
			stateValue: state292, stateLength: len(state292), officialModel: "gpt-5.5", upstreamOK: true, statusCode: http.StatusOK,
		}},
		{proxy: &OpenAICodexTurnStateProxy{ID: 2}, result: openAICodexTurnStateHarvestResult{
			stateValue: invalid332, stateLength: len(invalid332), officialModel: "gpt-5.5", upstreamOK: true, statusCode: http.StatusOK,
		}},
	})

	require.Equal(t, state292, result.stateValue)
	require.Equal(t, int64(1), proxy.ID)
}

func TestIsOpenAICodexTurnStateAuthFailureStopsFanout(t *testing.T) {
	for _, result := range []openAICodexTurnStateHarvestResult{
		{statusCode: http.StatusUnauthorized},
		{errorMessage: "token revoked by upstream"},
		{errorMessage: "invalidated OAuth token"},
		{errorMessage: "invalid API key"},
	} {
		require.True(t, isOpenAICodexTurnStateAuthFailure(result), "%+v", result)
	}
	require.False(t, isOpenAICodexTurnStateAuthFailure(openAICodexTurnStateHarvestResult{
		statusCode:   http.StatusTooManyRequests,
		errorMessage: "rate limited",
	}))
}

func TestOpenAICodexTurnStateProbeRanksByIssuedAtWithinLength(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	earlier := openAICodexTurnStateProbe{result: openAICodexTurnStateHarvestResult{
		stateValue:  testOpenAICodexTurnState(openAICodexTurnStateLength332, now.Add(-time.Minute), 'a'),
		stateLength: openAICodexTurnStateLength332,
	}}
	later := openAICodexTurnStateProbe{result: openAICodexTurnStateHarvestResult{
		stateValue:  testOpenAICodexTurnState(openAICodexTurnStateLength332, now, 'b'),
		stateLength: openAICodexTurnStateLength332,
	}}
	require.True(t, openAICodexTurnStateProbeRanksBefore(later, earlier))
	require.False(t, openAICodexTurnStateProbeRanksBefore(earlier, later))
}

func TestReadOpenAICodexTurnStateOfficialModelFromSSE(t *testing.T) {
	body := strings.NewReader("event: response.created\n" +
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-5.5"}}` + "\n\n" +
		`data: {"type":"response.output_text.delta","delta":"unused"}` + "\n\n")

	require.Equal(t, "gpt-5.5", readOpenAICodexTurnStateOfficialModel(body))
	require.Empty(t, readOpenAICodexTurnStateOfficialModel(strings.NewReader(
		`data: {"type":"response.created","response":{"id":"resp_2"}}`+"\n\n")))
}

func TestOpenAICodexTurnStateScannerPersistsActualSessionAndModelScope(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	pool := newOpenAICodexTurnStatePool()
	pool.now = func() time.Time { return now }
	accountID := int64(42)
	sessionID := "codex:ts:actual-scanner-session"
	state := testOpenAICodexTurnState(openAICodexTurnStateLength332, now, 's')

	pool.observe(state, &accountID, hashOpenAICodexTurnState(sessionID), "gpt-5.5", "scanner")

	key, ok := newOpenAICodexTurnStateBucketKey(accountID, "gpt-5.5")
	require.True(t, ok)
	pool.mu.RLock()
	record := pool.preferredByBucket[key]
	pool.mu.RUnlock()
	require.NotNil(t, record)
	require.Equal(t, accountID, *record.SourceAccountID)
	require.Equal(t, "gpt-5.5", record.SourceModel)
	require.Equal(t, hashOpenAICodexTurnState(sessionID), record.SourceSessionHash)

	_, otherModel := pool.preferredForBucket(accountID, "gpt-5.6-luna")
	require.False(t, otherModel)
}

func TestOpenAICodexTurnStateRouteTicketKeepsScannersOffBusinessEgress(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		proxy     *OpenAICodexTurnStateProxy
		wantProxy string
		wantID    bool
	}{
		{name: "direct"},
		{name: "dynamic scan only", proxy: &OpenAICodexTurnStateProxy{Source: "dynamic", ProxyURL: "http://dynamic.example:8080", ExitIP: "203.0.113.10"}},
		{name: "shared", proxy: &OpenAICodexTurnStateProxy{Source: "shared", SourceID: 7, ProxyURL: "http://shared.example:8080"}},
		{name: "scan only", proxy: &OpenAICodexTurnStateProxy{ID: 8, Source: "state", ProxyURL: "http://scan.example:8080"}, wantID: true},
		{name: "static binding", proxy: &OpenAICodexTurnStateProxy{ID: 9, Source: "state", ProxyURL: "http://static.example:8080", RouteBindingEnabled: true}, wantProxy: "http://static.example:8080", wantID: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			ticket := openAICodexTurnStateRouteTicketForProxy("harvest-session", testCase.proxy)
			require.Equal(t, "harvest-session", ticket.SessionID)
			require.Equal(t, testCase.wantProxy, ticket.ProxyURL)
			require.Equal(t, testCase.wantID, ticket.ProxyID != nil)
			if testCase.name == "dynamic scan only" {
				require.WithinDuration(t, time.Now().Add(openAICodexDynamicRouteTTL), ticket.ExpiresAt, time.Second)
				require.Equal(t, "203.0.113.10", ticket.ExitIP)
			} else {
				require.True(t, ticket.ExpiresAt.IsZero())
			}
		})
	}
}

func TestOpenAICodexTurnStateDynamicRouteCapsRecordLifetime(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	pool := newOpenAICodexTurnStatePool()
	pool.now = func() time.Time { return now }
	accountID := int64(42)
	state := testOpenAICodexTurnState(openAICodexTurnStateLength332, now, 'd')
	routeExpiry := now.Add(openAICodexDynamicRouteTTL)

	require.NoError(t, pool.observeDurably(
		context.Background(), state, accountID, "session-hash", "gpt-6-astra", "scanner",
		openAICodexTurnStateRouteTicket{
			SessionID: "harvest-session",
			ProxyURL:  "http://dynamic.example:8080",
			ExpiresAt: routeExpiry,
		},
	))
	record, ok := pool.preferredRecordForBucket(accountID, "gpt-6-astra")
	require.True(t, ok)
	require.Equal(t, routeExpiry, record.ExpiresAt)

	now = routeExpiry.Add(time.Second)
	_, ok = pool.preferredRecordForBucket(accountID, "gpt-6-astra")
	require.False(t, ok)
}

func TestHarvestOpenAICodexTurnStateRejectsReplayDowngrade(t *testing.T) {
	now := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	firstState := testOpenAICodexTurnState(332, now, 'a')
	secondState := testOpenAICodexTurnState(332, now, 'b')
	upstream := &codexTurnStateHarvestCaptureUpstream{responses: []*http.Response{
		codexTurnStateHarvestResponse(firstState, "gpt-6-astra"),
		codexTurnStateHarvestResponse(secondState, "gpt-5.6-luna"),
	}}
	svc := ticketTestService(t, config.OpenAICodexTicketConfig{}, upstream)

	result := svc.harvestOpenAICodexTurnState(context.Background(), ticketTestAccount(42), "gpt-6-astra", "http://proxy.example:8080")

	require.Contains(t, result.errorMessage, "route ticket replay verification failed")
	require.Contains(t, result.errorMessage, "received gpt-5.6-luna")
	require.Len(t, upstream.requests, 2)
	require.Equal(t, firstState, upstream.requests[1].Header.Get(openAICodexTurnStateHeader))
	require.NotEmpty(t, upstream.requests[0].Header.Get("session-id"))
	require.Equal(t, upstream.requests[0].Header.Get("session-id"), upstream.requests[1].Header.Get("session-id"))
	require.Equal(t, []string{"http://proxy.example:8080", "http://proxy.example:8080"}, upstream.proxies)
}

func TestHarvestOpenAICodexTurnStateReturnsSecondVerifiedState(t *testing.T) {
	now := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	firstState := testOpenAICodexTurnState(332, now, 'a')
	secondState := testOpenAICodexTurnState(332, now, 'b')
	upstream := &codexTurnStateHarvestCaptureUpstream{responses: []*http.Response{
		codexTurnStateHarvestResponse(firstState, "gpt-6-astra"),
		codexTurnStateHarvestResponse(secondState, "gpt-6-astra"),
	}}
	svc := ticketTestService(t, config.OpenAICodexTicketConfig{}, upstream)

	result := svc.harvestOpenAICodexTurnState(context.Background(), ticketTestAccount(42), "gpt-6-astra", "http://proxy.example:8080")

	require.Empty(t, result.errorMessage)
	require.Equal(t, secondState, result.stateValue)
	require.Equal(t, "gpt-6-astra", result.officialModel)
	require.NotEmpty(t, result.sessionID)
	require.Equal(t, result.sessionID, upstream.requests[0].Header.Get("session-id"))
	require.Equal(t, result.sessionID, upstream.requests[1].Header.Get("session-id"))
}

func TestHarvestOpenAICodexTurnStateCarriesOnlyAffinityCookiesIntoReplay(t *testing.T) {
	now := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	firstState := testOpenAICodexTurnState(332, now, 'a')
	secondState := testOpenAICodexTurnState(332, now, 'b')
	first := codexTurnStateHarvestResponse(firstState, "gpt-6-astra")
	first.Header.Add("Set-Cookie", "__cflb=route-a; Path=/; Secure; HttpOnly")
	first.Header.Add("Set-Cookie", "unrelated=must-not-replay; Path=/")
	second := codexTurnStateHarvestResponse(secondState, "gpt-6-astra")
	second.Header.Add("Set-Cookie", "__oailb=route-b; Path=/; Secure; HttpOnly")
	upstream := &codexTurnStateHarvestCaptureUpstream{responses: []*http.Response{first, second}}
	svc := ticketTestService(t, config.OpenAICodexTicketConfig{}, upstream)

	result := svc.harvestOpenAICodexTurnState(context.Background(), ticketTestAccount(42), "gpt-6-astra", "")

	require.Empty(t, result.errorMessage)
	require.Equal(t, "__cflb=route-a", upstream.requests[1].Header.Get("Cookie"))
	require.Equal(t, "__cflb=route-a; __oailb=route-b", result.routeCookie)
	require.NotContains(t, result.routeCookie, "unrelated")
}

func TestNormalizeOpenAICodexAffinityCookieHeaderDropsForeignCookies(t *testing.T) {
	require.Equal(t, "__cflb=a; __oailb=b; __cf_bm=c",
		normalizeOpenAICodexAffinityCookieHeader("foreign=x; __oailb=b; __cf_bm=c; __cflb=a"))
	require.Equal(t, "cf_clearance=d; cf_chl_rc_i=e",
		normalizeOpenAICodexAffinityCookieHeader("foreign=x; cf_chl_rc_i=e; cf_clearance=d"))
}

func TestHarvestOpenAICodexTurnStateAcceptsReplayModelConfirmationWithoutNewState(t *testing.T) {
	now := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	firstState := testOpenAICodexTurnState(332, now, 'a')
	upstream := &codexTurnStateHarvestCaptureUpstream{responses: []*http.Response{
		codexTurnStateHarvestResponse(firstState, "gpt-6-astra"),
		codexTurnStateHarvestResponse("", "gpt-6-astra"),
	}}
	svc := ticketTestService(t, config.OpenAICodexTicketConfig{}, upstream)

	result := svc.harvestOpenAICodexTurnState(context.Background(), ticketTestAccount(42), "gpt-6-astra", "http://proxy.example:8080")

	require.Empty(t, result.errorMessage)
	require.Equal(t, firstState, result.stateValue)
	require.Equal(t, "gpt-6-astra", result.officialModel)
	require.NotEmpty(t, result.sessionID)
	require.Equal(t, result.sessionID, upstream.requests[0].Header.Get("session-id"))
	require.Equal(t, result.sessionID, upstream.requests[1].Header.Get("session-id"))
	require.Equal(t, firstState, upstream.requests[1].Header.Get(openAICodexTurnStateHeader))
}

func TestHarvestOpenAICodexTurnStateKeepsIPv6AcrossReplay(t *testing.T) {
	now := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	firstState := testOpenAICodexTurnState(332, now, 'a')
	secondState := testOpenAICodexTurnState(332, now, 'b')
	upstream := &codexTurnStateHarvestCaptureUpstream{responses: []*http.Response{
		codexTurnStateHarvestResponse(firstState, "gpt-6-astra"),
		codexTurnStateHarvestResponse(secondState, "gpt-6-astra"),
	}}
	svc := ticketTestService(t, config.OpenAICodexTicketConfig{}, upstream)
	ctx := chatgptrelay.WithSourceIPv6(context.Background(), "2a02:ae02:1a:2c00::1234")

	result := svc.harvestOpenAICodexTurnState(ctx, ticketTestAccount(42), "gpt-6-astra", "")

	require.Empty(t, result.errorMessage)
	require.Equal(t, "2a02:ae02:1a:2c00::1234", result.routeIPv6)
	require.Len(t, upstream.requests, 2)
	for _, request := range upstream.requests {
		require.Equal(t, result.routeIPv6, chatgptrelay.SourceIPv6FromContext(request.Context()))
	}
}
