package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpenAICodexTurnStateScanSettingsValidation(t *testing.T) {
	defaults := defaultOpenAICodexTurnStateScanSettings()
	validated, err := validateOpenAICodexTurnStateScanSettings(defaults)
	require.NoError(t, err)
	require.Equal(t, []int{332, 292}, validated.TargetLengths)
	require.Equal(t, 5, validated.ParallelProbes)
	require.False(t, validated.DynamicProxyEnabled)

	for name, mutate := range map[string]func(*OpenAICodexTurnStateScanSettings){
		"empty lengths":       func(s *OpenAICodexTurnStateScanSettings) { s.TargetLengths = nil },
		"duplicate lengths":   func(s *OpenAICodexTurnStateScanSettings) { s.TargetLengths = []int{332, 332} },
		"length too small":    func(s *OpenAICodexTurnStateScanSettings) { s.TargetLengths = []int{63} },
		"length too large":    func(s *OpenAICodexTurnStateScanSettings) { s.TargetLengths = []int{4097} },
		"too many lengths":    func(s *OpenAICodexTurnStateScanSettings) { s.TargetLengths = make([]int, 17) },
		"no probes":           func(s *OpenAICodexTurnStateScanSettings) { s.ParallelProbes = 0 },
		"unbounded probes":    func(s *OpenAICodexTurnStateScanSettings) { s.ParallelProbes = 6 },
		"insecure URL":        func(s *OpenAICodexTurnStateScanSettings) { s.DynamicProxyURL = "http://example.com/proxies" },
		"URL credentials":     func(s *OpenAICodexTurnStateScanSettings) { s.DynamicProxyURL = "https://user:pass@example.com/proxies" },
		"URL fragment":        func(s *OpenAICodexTurnStateScanSettings) { s.DynamicProxyURL = "https://example.com/proxies#key" },
		"URL missing host":    func(s *OpenAICodexTurnStateScanSettings) { s.DynamicProxyURL = "https:///proxies" },
		"enabled missing URL": func(s *OpenAICodexTurnStateScanSettings) { s.DynamicProxyEnabled = true; s.DynamicProxyURL = "" },
	} {
		t.Run(name, func(t *testing.T) {
			settings := defaults.clone()
			mutate(settings)
			_, err := validateOpenAICodexTurnStateScanSettings(settings)
			require.Error(t, err)
		})
	}

	custom := defaults.clone()
	custom.TargetLengths = []int{292, 356, 332}
	validated, err = validateOpenAICodexTurnStateScanSettings(custom)
	require.NoError(t, err)
	require.Equal(t, 292, validated.primaryLength())
	require.True(t, validated.acceptsLength(356))
	require.False(t, validated.acceptsLength(312))
	require.Less(t, validated.lengthRank(356), validated.lengthRank(332))
}

func TestOpenAICodexTurnStateScanSettingsPersistBeforePublishAndRefresh(t *testing.T) {
	repo := newRuntimeSettingRepoStub()
	gateway := &OpenAIGatewayService{}
	scanner := &openAICodexTurnStateScanner{}
	svc := &OpsService{settingRepo: repo, openAIGatewayService: gateway, codexTurnStateScanner: scanner}
	svc.initRuntimeSettings(context.Background())
	now := time.Now().UTC().Truncate(time.Second)
	accountID := int64(42)
	pool := gateway.getOpenAICodexTurnStatePool()
	state332 := testOpenAICodexTurnState(332, now, 'a')
	state292 := testOpenAICodexTurnState(292, now, 'b')
	pool.observe(state332, &accountID, "session1", "gpt-5.5", "http")
	pool.observe(state292, &accountID, "session2", "gpt-5.5", "http")

	next := svc.GetOpenAICodexTurnStateScanSettings()
	next.TargetLengths = []int{292, 356}
	next.ParallelProbes = 3
	repo.setFn = func(_, _ string) error { return errors.New("database unavailable") }
	_, err := svc.UpdateOpenAICodexTurnStateScanSettings(context.Background(), next)
	require.ErrorContains(t, err, "database unavailable")
	require.Equal(t, []int{332, 292}, svc.GetOpenAICodexTurnStateScanSettings().TargetLengths)
	require.Equal(t, []int{332, 292}, scanner.settings.Load().TargetLengths)
	selected, ok := pool.preferredForBucket(accountID, "gpt-5.5")
	require.True(t, ok)
	require.Equal(t, state332, selected)

	repo.setFn = nil
	saved, err := svc.UpdateOpenAICodexTurnStateScanSettings(context.Background(), next)
	require.NoError(t, err)
	require.Equal(t, []int{292, 356}, saved.TargetLengths)
	selected, ok = pool.preferredForBucket(accountID, "gpt-5.5")
	require.True(t, ok)
	require.Equal(t, state292, selected)
	require.Equal(t, []int{292, 356}, scanner.settings.Load().TargetLengths)

	next.TargetLengths[0] = 999
	saved.TargetLengths[0] = 888
	read := svc.GetOpenAICodexTurnStateScanSettings()
	read.TargetLengths[0] = 777
	require.Equal(t, []int{292, 356}, svc.GetOpenAICodexTurnStateScanSettings().TargetLengths)

	other := &OpsService{settingRepo: repo, openAIGatewayService: &OpenAIGatewayService{}}
	other.initRuntimeSettings(context.Background())
	require.Equal(t, svc.GetOpenAICodexTurnStateScanSettings(), other.GetOpenAICodexTurnStateScanSettings())
	repo.values[SettingKeyOpenAICodexTurnStateScanSettings] = `{"target_lengths":[356,332],"parallel_probes":4}`
	require.NoError(t, other.RefreshRuntimeSettings(context.Background()))
	require.Equal(t, []int{356, 332}, other.GetOpenAICodexTurnStateScanSettings().TargetLengths)
	require.Equal(t, []int{292, 356}, svc.GetOpenAICodexTurnStateScanSettings().TargetLengths)

	repo.values[SettingKeyOpenAICodexTurnStateScanSettings] = `{"target_lengths":[]}`
	require.Error(t, other.RefreshRuntimeSettings(context.Background()))
	require.Equal(t, []int{356, 332}, other.GetOpenAICodexTurnStateScanSettings().TargetLengths)
}

func TestOpenAICodexTurnStateConfiguredPolicyKeepsAccountModelAndExpiryIsolation(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	gateway := &OpenAIGatewayService{}
	pool := gateway.getOpenAICodexTurnStatePool()
	pool.now = func() time.Time { return now }
	accountID, otherAccountID := int64(42), int64(84)
	state356 := testOpenAICodexTurnState(356, now.Add(-time.Minute), 'a')
	state292 := testOpenAICodexTurnState(292, now.Add(-time.Minute), 'b')
	otherState := testOpenAICodexTurnState(356, now, 'c')
	pool.observe(state356, &accountID, "session1", "gpt-5.5", "http")
	pool.observe(state292, &accountID, "session2", "gpt-5.5", "http")
	pool.observe(otherState, &otherAccountID, "session3", "gpt-5.5", "http")
	pool.observe(otherState, &accountID, "session4", "gpt-6-astra", "http")
	pool.setTargetLengths([]int{356, 292})

	c, _ := newTurnStateTestContext(t, 7, "client-session")
	h := http.Header{}
	gateway.guardOpenAICodexTurnStateEcho(c, &Account{ID: accountID, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, h, "gpt-5.5")
	require.Equal(t, state356, h.Get(openAICodexTurnStateHeader))
	_, ok := pool.preferredForBucket(accountID, "gpt-6-terra")
	require.False(t, ok)
	_, ok = pool.preferredForBucket(99, "gpt-5.5")
	require.False(t, ok)
	require.True(t, pool.hasReusableStateBeyond(accountID, "gpt-5.5", now.Add(15*time.Minute)))

	pool.setTargetLengths([]int{292, 356})
	selected, ok := pool.preferredForBucket(accountID, "gpt-5.5")
	require.True(t, ok)
	require.Equal(t, state292, selected)
	pool.setTargetLengths([]int{332})
	_, ok = pool.preferredForBucket(accountID, "gpt-5.5")
	require.False(t, ok)
	pool.setTargetLengths([]int{356})
	now = now.Add(time.Hour)
	_, ok = pool.preferredForBucket(accountID, "gpt-5.5")
	require.False(t, ok)
	require.False(t, pool.hasReusableStateBeyond(accountID, "gpt-5.5", now))
}
