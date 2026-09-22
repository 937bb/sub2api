package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func testScopedOpenAICodexTurnState(length int, issuedAt time.Time, marker byte) string {
	return testOpenAICodexTurnState((length+3)/4*4, issuedAt, marker)[:length]
}

func TestOpenAICodexTurnStateScopedLengthsPreserveIssuedAtValidation(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	for _, length := range []int{273, 286, 292, 332} {
		value := testScopedOpenAICodexTurnState(length, now, 't')
		issuedAt, ok := parseOpenAICodexTurnStateIssuedAt(value)
		require.True(t, ok, "length %d", length)
		require.Equal(t, now, issuedAt)
		require.False(t, newObservedOpenAICodexTurnStateRecord(value, nil, "", "", "http", now.Add(time.Hour)).Active)
		require.Nil(t, newObservedOpenAICodexTurnStateRecord(value, nil, "", "", "http", now.Add(-time.Second)))
	}
	valid := testScopedOpenAICodexTurnState(273, now, 't')
	for _, value := range []string{
		"A" + valid[1:],
		valid[:11],
		valid[:12] + "." + valid[13:],
		valid[:12] + "=" + valid[13:],
		valid + "===",
		valid[:12] + "\r\n" + valid[14:],
		strings.Repeat("_", 273),
	} {
		_, ok := parseOpenAICodexTurnStateIssuedAt(value)
		require.False(t, ok, "invalid prefix or alphabet must be rejected")
	}
}

func scopedTurnStatePoolSettings(t *testing.T) *OpenAICodexTurnStateScanSettings {
	t.Helper()
	settings := defaultOpenAICodexTurnStateScanSettings()
	require.NoError(t, json.Unmarshal([]byte(`{
		"target_lengths": [332,292],
		"rules": [
			{"plan_type":"pro","model":"*","target_lengths":[292]},
			{"plan_type":"team","model":"gpt-5.6-terra","target_lengths":[286]},
			{"plan_type":"team","model":"gpt-6-astra","target_lengths":[273]}
		]
	}`), settings))
	return settings
}

func TestOpenAICodexTurnStateScopedPoolAppliesPlanAndModelPolicy(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	pool := newOpenAICodexTurnStatePool()
	pool.now = func() time.Time { return now }
	pool.setScanSettings(scopedTurnStatePoolSettings(t))
	proID, teamID, unknownID := int64(11), int64(22), int64(33)
	pool.setAccountPlan(proID, "pro")
	pool.setAccountPlan(teamID, "team")
	for _, accountID := range []int64{proID, teamID, unknownID} {
		for _, model := range []string{"gpt-5.6-terra", "gpt-6-astra"} {
			for _, length := range []int{332, 292, 286, 273} {
				pool.observe(testScopedOpenAICodexTurnState(length, now, byte(accountID)), &accountID, "session", model, "scanner")
			}
		}
	}
	for _, tc := range []struct {
		accountID  int64
		model      string
		wantLength int
	}{
		{proID, "gpt-5.6-terra", 292},
		{proID, "gpt-6-astra", 292},
		{teamID, "gpt-5.6-terra", 286},
		{teamID, "gpt-6-astra", 273},
		{unknownID, "gpt-5.6-terra", 332},
		{unknownID, "gpt-6-astra", 332},
	} {
		state, ok := pool.preferredForBucket(tc.accountID, tc.model)
		require.True(t, ok, "account %d, model %s", tc.accountID, tc.model)
		require.Len(t, state, tc.wantLength)
		require.True(t, pool.hasReusableStateBeyond(tc.accountID, tc.model, now.Add(time.Minute)))
	}
	_, found := pool.preferredForBucket(44, "gpt-6-astra")
	require.False(t, found)
	_, found = pool.preferredForBucket(teamID, "gpt-6-terra")
	require.False(t, found)
}

func TestOpenAICodexTurnStateScopedPoolDefaultsKeep292FallbackOnlyForPro(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	pool := newOpenAICodexTurnStatePool()
	pool.now = func() time.Time { return now }
	for index, plan := range []string{"pro", "team"} {
		accountID := int64(index + 11)
		pool.setAccountPlan(accountID, plan)
		for _, model := range []string{"gpt-5.6-terra", "gpt-6-astra"} {
			primary := testScopedOpenAICodexTurnState(332, now.Add(-time.Minute), byte(accountID))
			fallback := testScopedOpenAICodexTurnState(292, now, byte(accountID))
			pool.observe(primary, &accountID, "session", model, "scanner")
			pool.observe(fallback, &accountID, "session", model, "scanner")
			selected, ok := pool.preferredForBucket(accountID, model)
			require.True(t, ok)
			require.Equal(t, primary, selected)
			pool.removeHashes([]string{hashOpenAICodexTurnState(primary)})
			selected, ok = pool.preferredForBucket(accountID, model)
			if plan == "pro" {
				require.True(t, ok)
				require.Equal(t, fallback, selected)
			} else {
				require.False(t, ok, "Team must never reuse a non-332 state")
			}
		}
	}
}

func TestOpenAICodexTurnStatePoolCanRequireAcquisitionRouteBinding(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	pool := newOpenAICodexTurnStatePool()
	pool.now = func() time.Time { return now }
	accountID := int64(11)
	const model = "gpt-5.6-terra"
	pool.setAccountPlan(accountID, "pro")
	unbound := testScopedOpenAICodexTurnState(332, now.Add(-time.Minute), 'u')
	require.NoError(t, pool.observeDurably(context.Background(), unbound, accountID, "session", model, "scanner", openAICodexTurnStateRouteTicket{SessionID: "session"}))
	_, ok := pool.preferredForBucket(accountID, model)
	require.True(t, ok)

	settings := defaultOpenAICodexTurnStateScanSettings()
	requireRouteBinding := true
	settings.RequireRouteBinding = &requireRouteBinding
	pool.setScanSettings(settings)
	_, ok = pool.preferredForBucket(accountID, model)
	require.False(t, ok, "an unbound legacy ticket must not remain reusable")

	proxyBound := testScopedOpenAICodexTurnState(332, now.Add(-time.Minute), 'p')
	require.ErrorContains(t, pool.observeDurably(context.Background(), proxyBound, accountID, "session", model, "scanner", openAICodexTurnStateRouteTicket{
		SessionID: "session",
		ProxyURL:  "http://scanner.example:8080",
	}), "not reusable", "scanner proxies must never satisfy strict business route binding")

	bound := testScopedOpenAICodexTurnState(332, now.Add(-time.Minute), 'b')
	require.NoError(t, pool.observeDurably(context.Background(), bound, accountID, "session", model, "scanner", openAICodexTurnStateRouteTicket{
		SessionID: "session",
		RouteIPv6: "2a02:ae02:1a:2c00::1234",
	}))
	selected, ok := pool.preferredRecordForBucket(accountID, model)
	require.True(t, ok)
	require.Equal(t, bound, selected.StateValue)
	require.Equal(t, "2a02:ae02:1a:2c00::1234", selected.RouteIPv6)
}

func TestOpenAICodexTurnStatePoolStrictBindingAcceptsManagedAirportOnly(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	pool := newOpenAICodexTurnStatePool()
	pool.now = func() time.Time { return now }
	settings := defaultOpenAICodexTurnStateScanSettings()
	requireRouteBinding := true
	settings.RequireRouteBinding = &requireRouteBinding
	pool.setScanSettings(settings)

	accountID := int64(11)
	const model = "gpt-6-astra"
	proxyID := int64(9)
	state := testScopedOpenAICodexTurnState(332, now.Add(-time.Minute), 'm')
	require.NoError(t, pool.observeDurably(context.Background(), state, accountID, "session", model, "scanner", openAICodexTurnStateRouteTicket{
		SessionID: "session",
		ProxyID:   &proxyID,
		ProxyURL:  "http://airport-node.example:8080",
	}))

	selected, ok := pool.preferredRecordForBucket(accountID, model)
	require.True(t, ok)
	require.Equal(t, proxyID, *selected.SourceProxyID)
	require.Equal(t, "http://airport-node.example:8080", selected.SourceProxyURL)
}

func TestOpenAICodexTurnStateStrictRouteBindingOverridesClientStateWithAccountModelTicket(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	svc := &OpenAIGatewayService{}
	pool := svc.getOpenAICodexTurnStatePool()
	pool.now = func() time.Time { return now }
	settings := defaultOpenAICodexTurnStateScanSettings()
	requireRouteBinding := true
	settings.RequireRouteBinding = &requireRouteBinding
	pool.setScanSettings(settings)

	accountID := int64(12)
	const model = "gpt-6-astra"
	account := &Account{ID: accountID, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	ticketState := testScopedOpenAICodexTurnState(332, now.Add(-time.Minute), 't')
	require.NoError(t, pool.observeDurably(context.Background(), ticketState, accountID, "ticket-session-hash", model, "scanner", openAICodexTurnStateRouteTicket{
		SessionID: "ticket-session",
		RouteIPv6: "2a02:ae02:1a:2c00::5678",
	}))

	c, _ := newTurnStateTestContext(t, 7, "client-session")
	headers := http.Header{}
	headers.Set(openAICodexTurnStateHeader, testScopedOpenAICodexTurnState(332, now.Add(-time.Minute), 'c'))
	svc.guardOpenAICodexTurnStateEcho(c, account, headers, model)

	require.Equal(t, ticketState, headers.Get(openAICodexTurnStateHeader))
	require.Equal(t, "ticket-session", headers.Get("session-id"))
	proxyURL, routeIPv6 := svc.openAICodexTurnStateRoute(account, headers, "http://fallback.example:8080")
	require.Empty(t, proxyURL)
	require.Equal(t, "2a02:ae02:1a:2c00::5678", routeIPv6)
}

func TestOpenAICodexTurnStateScopedPoolUpdatesPolicyAndPlanWithoutCrossAccountReuse(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	pool := newOpenAICodexTurnStatePool()
	pool.now = func() time.Time { return now }
	settings := scopedTurnStatePoolSettings(t)
	pool.setScanSettings(settings)
	firstID, secondID := int64(11), int64(22)
	for _, accountID := range []int64{firstID, secondID} {
		pool.setAccountPlan(accountID, "pro")
		for _, length := range []int{292, 286, 332} {
			pool.observe(testScopedOpenAICodexTurnState(length, now, byte(accountID)), &accountID, "session", "gpt-5.6-terra", "scanner")
		}
	}
	pool.setAccountPlan(firstID, "team")
	first, ok := pool.preferredForBucket(firstID, "gpt-5.6-terra")
	require.True(t, ok)
	require.Len(t, first, 286)
	second, ok := pool.preferredForBucket(secondID, "gpt-5.6-terra")
	require.True(t, ok)
	require.Len(t, second, 292)
	require.NotEqual(t, first, second)

	require.NoError(t, json.Unmarshal([]byte(`{"rules":[{"plan_type":"team","model":"gpt-5.6-terra","target_lengths":[332]}]}`), settings))
	pool.setScanSettings(settings)
	first, ok = pool.preferredForBucket(firstID, "gpt-5.6-terra")
	require.True(t, ok)
	require.Len(t, first, 332)

	// Settings are detached from mutable input after publication.
	settings.Rules[0].TargetLengths[0] = 286
	first, ok = pool.preferredForBucket(firstID, "gpt-5.6-terra")
	require.True(t, ok)
	require.Len(t, first, 332)
	pool.setTargetLengths([]int{286})
	second, ok = pool.preferredForBucket(secondID, "gpt-5.6-terra")
	require.True(t, ok)
	require.Len(t, second, 286)
}

func TestOpenAICodexTurnStateScopedPoolBucketSyncUsesScopedTargets(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	pool := newOpenAICodexTurnStatePool()
	pool.now = func() time.Time { return now }
	pool.setScanSettings(scopedTurnStatePoolSettings(t))
	accountID := int64(11)
	pool.setAccountPlan(accountID, "team")
	state := testScopedOpenAICodexTurnState(286, now, 't')
	record := newObservedOpenAICodexTurnStateRecord(state, &accountID, "session", "gpt-5.6-terra", "scanner", now)
	record.SourceSessionID = "session"
	repo := &turnStateBucketSyncRepo{record: record}
	pool.repo = repo
	require.NoError(t, pool.refreshBucket(context.Background(), accountID, "gpt-5.6-terra"))
	require.Equal(t, []int{286}, repo.targetLengths)
	selected, ok := pool.preferredForBucket(accountID, "gpt-5.6-terra")
	require.True(t, ok)
	require.Equal(t, state, selected)
	require.NoError(t, pool.observeDurably(context.Background(), state, accountID, "session", "gpt-5.6-terra", "scanner", openAICodexTurnStateRouteTicket{SessionID: "session"}))
	require.Error(t, pool.observeDurably(context.Background(), testScopedOpenAICodexTurnState(292, now, 'p'), accountID, "session", "gpt-5.6-terra", "scanner", openAICodexTurnStateRouteTicket{SessionID: "session"}))
	require.Equal(t, 1, repo.writeCalls)
}

func TestOpenAICodexTurnStateScopedPoolBucketSyncNeverFallsBackForEmptyPolicy(t *testing.T) {
	pool := newOpenAICodexTurnStatePool()
	pool.setScanSettings(&OpenAICodexTurnStateScanSettings{})
	repo := &turnStateBucketSyncRepo{}
	pool.repo = repo
	require.NoError(t, pool.refreshBucket(context.Background(), 11, "gpt-5.6-terra"))
	require.Equal(t, 1, repo.readCalls)
}

func TestOpenAICodexTurnStateScopedPoolBucketIndexDropsRemovedAndExpiredValues(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	pool := newOpenAICodexTurnStatePool()
	pool.now = func() time.Time { return now }
	pool.setScanSettings(scopedTurnStatePoolSettings(t))
	accountID := int64(11)
	pool.setAccountPlan(accountID, "team")
	first := testScopedOpenAICodexTurnState(286, now, 'a')
	second := testScopedOpenAICodexTurnState(286, now, 'b')
	pool.observe(first, &accountID, "session", "gpt-5.6-terra", "scanner")
	pool.observe(second, &accountID, "session", "gpt-5.6-terra", "scanner")
	pool.removeHashes([]string{hashOpenAICodexTurnState(first)})
	selected, ok := pool.preferredForBucket(accountID, "gpt-5.6-terra")
	require.True(t, ok)
	require.Equal(t, second, selected)
	key, _ := newOpenAICodexTurnStateBucketKey(accountID, "gpt-5.6-terra")
	require.Len(t, pool.entriesByBucket[key], 1)
	now = now.Add(time.Hour)
	pool.cleanupExpired(now)
	_, ok = pool.preferredForBucket(accountID, "gpt-5.6-terra")
	require.False(t, ok)
	require.Empty(t, pool.entriesByBucket)
	require.False(t, pool.hasReusableStateBeyond(accountID, "gpt-5.6-terra", now))
}

func TestGuardOpenAICodexTurnStateScopedPoolUsesCredentialOwnersPlan(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	svc := &OpenAIGatewayService{}
	pool := svc.getOpenAICodexTurnStatePool()
	pool.now = func() time.Time { return now }
	pool.setScanSettings(scopedTurnStatePoolSettings(t))
	owner := &Account{ID: 11, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"plan_type": "team"}}
	shadow := &Account{ID: 22, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"plan_type": "pro"}}
	for _, length := range []int{286, 292, 332} {
		pool.observe(testScopedOpenAICodexTurnState(length, now, 't'), &owner.ID, "session", "gpt-5.6-terra", "scanner")
	}
	c, _ := newTurnStateTestContext(t, 7, "target-session")
	c.Set(codexAccountIdentitySourceContextKey, owner)
	headers := http.Header{}
	svc.guardOpenAICodexTurnStateEcho(c, shadow, headers, "gpt-5.6-terra")
	require.Len(t, headers.Get(openAICodexTurnStateHeader), 286)
	require.Equal(t, "team", pool.accountPlans[owner.ID])
	require.Empty(t, pool.accountPlans[shadow.ID])
	svc.observeOpenAICodexTurnState(c, shadow, testScopedOpenAICodexTurnState(286, now, 'n'), "http")
	require.Equal(t, "team", pool.accountPlans[owner.ID])
}
