package service

import (
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

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
	require.Equal(t, "proxy connect failed", openAICodexTurnStateScanFailure("gpt-5.5", result))
}

func TestOpenAICodexTurnStateScanFailureValidatesSuccessfulProbe(t *testing.T) {
	state := strings.Repeat("a", openAICodexTurnStateLength332)
	result := openAICodexTurnStateHarvestResult{
		stateValue:    state,
		stateLength:   len(state),
		officialModel: "gpt-5.5",
		upstreamOK:    true,
		statusCode:    200,
	}
	require.Empty(t, openAICodexTurnStateScanFailure("gpt-5.5", result))

	result.officialModel = ""
	require.Equal(t, "upstream response did not declare an official model", openAICodexTurnStateScanFailure("gpt-5.5", result))

	result.officialModel = "gpt-5.6-luna"
	require.Equal(t, "upstream model mismatch: requested gpt-5.5, received gpt-5.6-luna", openAICodexTurnStateScanFailure("gpt-5.5", result))
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
