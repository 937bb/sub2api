package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestOpenAICodexTurnStatePlanModelSettings(t *testing.T) {
	settings := scopedTurnStatePoolSettings(t)
	for _, tc := range []struct {
		plan, model string
		lengths     []int
	}{
		{"pro", "gpt-6-astra", []int{292}},
		{" Pro20x ", "gpt-5.6-terra", []int{292}},
		{"prolite", "gpt-5.6-terra", []int{292}},
		{"chatgpt_pro", "gpt-6-astra", []int{292}},
		{"team", "gpt-5.6-terra", []int{286}},
		{"self_serve_business_usage_based", " GPT-6-ASTRA ", []int{273}},
		{"self_serve_business_prolite", "gpt-5.6-terra", []int{286}},
		{"team", "gpt-5.5", []int{332, 292}},
		{"", "gpt-6-astra", []int{332, 292}},
	} {
		require.Equal(t, tc.lengths, settings.TargetLengthsFor(tc.plan, tc.model))
	}
	settings.Rules = append(settings.Rules,
		OpenAICodexTurnStateLengthRule{PlanType: "*", Model: "*", TargetLengths: []int{300}},
		OpenAICodexTurnStateLengthRule{PlanType: "*", Model: "gpt-6-astra", TargetLengths: []int{301}},
		OpenAICodexTurnStateLengthRule{PlanType: "pro", Model: "gpt-6-astra", TargetLengths: []int{302}},
	)
	require.Equal(t, []int{302}, settings.TargetLengthsFor("pro", "gpt-6-astra"))
	require.Equal(t, []int{292}, settings.TargetLengthsFor("pro", "gpt-5.5"))
	require.Equal(t, []int{301}, settings.TargetLengthsFor("plus", "gpt-6-astra"))
	require.Equal(t, []int{300}, settings.TargetLengthsFor("plus", "gpt-5.5"))
	cloned := settings.clone()
	cloned.Rules[0].TargetLengths[0] = 999
	require.Equal(t, 292, settings.Rules[0].TargetLengths[0])
	account := &Account{Credentials: map[string]any{"plan_type": " ", "chatgpt_plan_type": "business", "subscription_plan": "pro"}}
	require.Equal(t, "team", OpenAICodexStatePlanType(account))
	require.Equal(t, []int{273}, settings.forAccountModel(account, "gpt-6-astra").TargetLengths)
	imported := &Account{Credentials: map[string]any{"chatgpt_account_id": "workspace-1"}, Extra: map[string]any{"team_oauth_verified": true, "team_oauth_verified_workspace_id": "workspace-1"}}
	require.Equal(t, "team", OpenAICodexStatePlanType(imported))
	imported.Extra["team_oauth_verified_workspace_id"] = "workspace-2"
	require.Empty(t, OpenAICodexStatePlanType(imported), "a different workspace must not establish a Team tier")
	imported.Extra["team_oauth_verified_workspace_id"] = "workspace-1"
	imported.Credentials["plan_type"] = "prolite"
	require.Equal(t, "pro", OpenAICodexStatePlanType(imported), "explicit credentials outrank the import fallback")
}

func TestOpenAICodexTurnStateDefaultsUseTeam356Then332Then292(t *testing.T) {
	settings := defaultOpenAICodexTurnStateScanSettings()
	require.True(t, settings.IsStateRequiredBeforeRouting())
	require.False(t, settings.IsRouteBindingRequired())
	for _, plan := range []string{"pro", "team", "plus", "free", "enterprise"} {
		require.True(t, settings.IsPlanScanEnabled(plan))
	}
	require.Equal(t, []OpenAICodexTurnStateLengthRule{
		{PlanType: "pro", Model: "*", TargetLengths: []int{332, 292}},
		{PlanType: "team", Model: "*", TargetLengths: []int{356, 332, 292}},
	}, settings.Rules)
	for _, plan := range []string{"pro", "pro20x", "prolite"} {
		for _, model := range []string{"gpt-5.6-terra", "gpt-6-astra", "gpt-5.5"} {
			lengths := settings.TargetLengthsFor(plan, model)
			require.Equal(t, []int{332, 292}, lengths, "plan %s model %s", plan, model)
		}
	}
	for _, plan := range []string{"team", "business", "self_serve_business_prolite"} {
		for _, model := range []string{"gpt-5.6-terra", "gpt-6-astra", "gpt-5.5"} {
			lengths := settings.TargetLengthsFor(plan, model)
			require.Equal(t, []int{356, 332, 292}, lengths, "plan %s model %s", plan, model)
		}
	}
	settings.Rules = append(settings.Rules, OpenAICodexTurnStateLengthRule{PlanType: "team", Model: "gpt-6-astra", TargetLengths: []int{273}})
	require.Equal(t, []int{273}, settings.TargetLengthsFor("team", "gpt-6-astra"), "manual per-model overrides must remain supported")
	require.Equal(t, []int{356, 332, 292}, settings.TargetLengthsFor("team", "gpt-5.6-terra"))
	require.Equal(t, []int{332, 292}, settings.TargetLengthsFor("pro", "gpt-6-astra"))
}

func TestOpenAICodexTurnStatePlanScanSwitchStopsOnlyAcquisition(t *testing.T) {
	settings := defaultOpenAICodexTurnStateScanSettings()
	settings.PlanScanEnabled["pro"] = false
	require.False(t, settings.IsPlanScanEnabled("pro20x"))
	require.True(t, settings.IsPlanScanEnabled("team"))
	require.True(t, settings.IsPlanScanEnabled("new-plan"), "unknown plans remain enabled until explicitly supported")

	now := time.Now().UTC().Truncate(time.Second)
	account := turnStateRefreshAccount(41, now)
	account.Credentials["plan_type"] = "pro"
	model := "gpt-6-astra"
	repo := &turnStateRefreshScanRepo{scans: make(map[openAICodexTurnStateBucketKey]*OpenAICodexTurnStateScan)}
	gateway := ticketTestService(t, config.OpenAICodexTicketConfig{Models: []string{model}}, nil)
	pool := gateway.getOpenAICodexTurnStatePool()
	pool.setScanSettings(settings)
	scanner := newOpenAICodexTurnStateScanner(repo, &turnStateRefreshAccountRepo{accounts: map[int64]*Account{41: account}}, gateway)
	scanner.settings.Store(settings)
	called := false
	scanner.probe = func(context.Context, *Account, string, string) openAICodexTurnStateHarvestResult {
		called = true
		return stateConcurrencyResult(292, model)
	}
	svc := &OpsService{codexTurnStateScanner: scanner}
	require.False(t, svc.EnqueueOpenAICodexTurnStateScan(context.Background(), account.ID, model))
	require.Empty(t, scanner.queue)
	scanner.runJob(context.Background(), openAICodexTurnStateScanJob{accountID: account.ID, model: model, force: true})
	require.False(t, called)
	require.Empty(t, repo.scans, "disabling a plan must not create scan records")

	account.Credentials["plan_type"] = "team"
	require.True(t, svc.EnqueueOpenAICodexTurnStateScan(context.Background(), account.ID, model))
	scanner.runJob(context.Background(), <-scanner.queue)
	require.True(t, called, "disabling Pro must not stop Team acquisition")
}

func TestOpenAICodexTurnStateRuleSettingsValidationAndLegacyDefaults(t *testing.T) {
	for name, rule := range map[string]OpenAICodexTurnStateLengthRule{
		"unknown plan":      {PlanType: "maybe-pro", Model: "*", TargetLengths: []int{292}},
		"blank model":       {PlanType: "pro", TargetLengths: []int{292}},
		"partial wildcard":  {PlanType: "pro", Model: "gpt-*", TargetLengths: []int{292}},
		"missing lengths":   {PlanType: "pro", Model: "*"},
		"duplicate lengths": {PlanType: "team", Model: "*", TargetLengths: []int{273, 273}},
	} {
		t.Run(name, func(t *testing.T) {
			s := defaultOpenAICodexTurnStateScanSettings()
			s.Rules = []OpenAICodexTurnStateLengthRule{rule}
			_, err := validateOpenAICodexTurnStateScanSettings(s)
			require.Error(t, err)
		})
	}
	s := defaultOpenAICodexTurnStateScanSettings()
	s.Rules = append(s.Rules, OpenAICodexTurnStateLengthRule{PlanType: "Pro20x", Model: "*", TargetLengths: []int{332}})
	_, err := validateOpenAICodexTurnStateScanSettings(s)
	require.ErrorContains(t, err, "duplicate")
	s = defaultOpenAICodexTurnStateScanSettings()
	s.PlanScanEnabled["pro20x"] = false
	_, err = validateOpenAICodexTurnStateScanSettings(s)
	require.ErrorContains(t, err, "duplicate normalized plans")
	legacy := &OpenAICodexTurnStateScanSettings{}
	require.NoError(t, json.Unmarshal([]byte(`{"target_lengths":[332,292],"parallel_probes":5}`), legacy))
	validated, err := validateOpenAICodexTurnStateScanSettings(legacy)
	require.NoError(t, err)
	require.True(t, validated.IsStateRequiredBeforeRouting(), "legacy settings must enable the routing guard")
	require.False(t, validated.IsRouteBindingRequired(), "legacy settings must preserve unbound State compatibility")
	require.Equal(t, []int{332, 292}, validated.TargetLengthsFor("pro", "gpt-5.5"))
	require.True(t, validated.IsPlanScanEnabled("pro"), "legacy settings must keep scanning enabled")
	legacy.Rules = []OpenAICodexTurnStateLengthRule{}
	validated, err = validateOpenAICodexTurnStateScanSettings(legacy)
	require.NoError(t, err)
	require.Empty(t, validated.Rules)
	require.Equal(t, []int{332, 292}, validated.TargetLengthsFor("pro", "gpt-5.5"))
}

func TestOpenAICodexTurnStateScannerUsesPlanModelRules(t *testing.T) {
	for _, tc := range []struct {
		plan, model string
		length      int
		ready       bool
	}{
		{"pro", "gpt-6-astra", 292, true}, {"pro", "gpt-6-astra", 332, true},
		{"prolite", "gpt-6-astra", 292, true},
		{"team", "gpt-5.6-terra", 286, true}, {"team", "gpt-5.6-terra", 292, true},
		{"team", "gpt-6-astra", 273, true}, {"team", "gpt-6-astra", 286, true},
	} {
		t.Run(tc.plan+tc.model+time.Duration(tc.length).String(), func(t *testing.T) {
			now := time.Now().UTC().Truncate(time.Second)
			account := turnStateRefreshAccount(41, now)
			account.Credentials["plan_type"] = tc.plan
			repo := &turnStateRefreshScanRepo{scans: make(map[openAICodexTurnStateBucketKey]*OpenAICodexTurnStateScan)}
			gateway := ticketTestService(t, config.OpenAICodexTicketConfig{Models: []string{tc.model}}, nil)
			scanner := newOpenAICodexTurnStateScanner(repo, &turnStateRefreshAccountRepo{accounts: map[int64]*Account{41: account}}, gateway)
			settings := scopedTurnStatePoolSettings(t)
			scanner.settings.Store(settings)
			gateway.getOpenAICodexTurnStatePool().setScanSettings(settings)
			calls := 0
			scanner.probe = func(context.Context, *Account, string, string) openAICodexTurnStateHarvestResult {
				calls++
				result := stateConcurrencyResult(292, tc.model)
				result.stateLength = tc.length
				result.stateValue = testScopedOpenAICodexTurnState(tc.length, now, 'p')
				return result
			}
			scanner.runJob(context.Background(), openAICodexTurnStateScanJob{accountID: 41, model: tc.model})
			scan := repo.scans[openAICodexTurnStateBucketKey{accountID: 41, model: tc.model}]
			require.NotNil(t, scan)
			if tc.ready {
				require.Equal(t, "ready", scan.Status)
				_, ok := gateway.getOpenAICodexTurnStatePool().preferredForBucket(41, tc.model)
				require.True(t, ok)
				scanner.runJob(context.Background(), openAICodexTurnStateScanJob{accountID: 41, model: tc.model})
				require.Equal(t, 1, calls, "valid scoped state must suppress reacquisition")
			} else {
				require.Equal(t, "retry_wait", scan.Status)
			}
		})
	}
}

func TestOpenAICodexTurnStateManualScanLoadsPlanBeforeSkipping(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	account := turnStateRefreshAccount(41, now)
	account.Credentials["plan_type"] = "pro"
	model := "gpt-6-astra"
	repo := &turnStateRefreshScanRepo{scans: make(map[openAICodexTurnStateBucketKey]*OpenAICodexTurnStateScan)}
	gateway := ticketTestService(t, config.OpenAICodexTicketConfig{Models: []string{model}}, nil)
	pool := gateway.getOpenAICodexTurnStatePool()
	settings := scopedTurnStatePoolSettings(t)
	pool.setScanSettings(settings)
	pool.observe(testOpenAICodexTurnState(332, now, 'x'), &account.ID, "session", model, "scanner")
	scanner := newOpenAICodexTurnStateScanner(repo, &turnStateRefreshAccountRepo{accounts: map[int64]*Account{41: account}}, gateway)
	scanner.settings.Store(settings)
	calls := 0
	scanner.probe = func(context.Context, *Account, string, string) openAICodexTurnStateHarvestResult {
		calls++
		return stateConcurrencyResult(292, model)
	}
	svc := &OpsService{codexTurnStateScanner: scanner}
	require.True(t, svc.EnqueueOpenAICodexTurnStateScan(context.Background(), 41, model))
	require.False(t, svc.EnqueueOpenAICodexTurnStateScan(context.Background(), 41, model), "same account/model must remain deduplicated")
	scanner.runJob(context.Background(), <-scanner.queue)
	require.Equal(t, 1, calls, "Pro must replace 332 even when its plan was unknown at enqueue time")
	selected, ok := pool.preferredForBucket(41, model)
	require.True(t, ok)
	require.Len(t, selected, 292)
}

func TestOpenAICodexTurnStateScanSettingsValidation(t *testing.T) {
	defaults := defaultOpenAICodexTurnStateScanSettings()
	validated, err := validateOpenAICodexTurnStateScanSettings(defaults)
	require.NoError(t, err)
	require.Equal(t, []int{332, 292}, validated.TargetLengths)
	require.Equal(t, 5, validated.ParallelProbes)
	require.Equal(t, OpenAICodexTurnStateScanRouteAuto, validated.ScanRouteMode)
	require.False(t, validated.DynamicProxyEnabled)
	require.True(t, validated.IsStateRequiredBeforeRouting())

	for name, mutate := range map[string]func(*OpenAICodexTurnStateScanSettings){
		"empty lengths":       func(s *OpenAICodexTurnStateScanSettings) { s.TargetLengths = nil },
		"duplicate lengths":   func(s *OpenAICodexTurnStateScanSettings) { s.TargetLengths = []int{332, 332} },
		"length too small":    func(s *OpenAICodexTurnStateScanSettings) { s.TargetLengths = []int{63} },
		"length too large":    func(s *OpenAICodexTurnStateScanSettings) { s.TargetLengths = []int{4097} },
		"too many lengths":    func(s *OpenAICodexTurnStateScanSettings) { s.TargetLengths = make([]int, 17) },
		"no probes":           func(s *OpenAICodexTurnStateScanSettings) { s.ParallelProbes = 0 },
		"unbounded probes":    func(s *OpenAICodexTurnStateScanSettings) { s.ParallelProbes = 6 },
		"unknown scan route":  func(s *OpenAICodexTurnStateScanSettings) { s.ScanRouteMode = "airport_magic" },
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
	require.True(t, validated.acceptsLength(312), "configured lengths rank valid states; they do not define validity")
	require.False(t, validated.acceptsLength(63))
	require.False(t, validated.acceptsLength(4097))
	require.Less(t, validated.lengthRank(356), validated.lengthRank(332))

	requireStateBeforeRouting := false
	custom.RequireStateBeforeRouting = &requireStateBeforeRouting
	validated, err = validateOpenAICodexTurnStateScanSettings(custom)
	require.NoError(t, err)
	require.False(t, validated.IsStateRequiredBeforeRouting())
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
	pool.observe(state332, &accountID, "session1", "gpt-5.5", "scanner")
	pool.observe(state292, &accountID, "session2", "gpt-5.5", "scanner")

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
	pool.observe(state356, &accountID, "session1", "gpt-5.5", "scanner")
	pool.observe(state292, &accountID, "session2", "gpt-5.5", "scanner")
	pool.observe(otherState, &otherAccountID, "session3", "gpt-5.5", "scanner")
	pool.observe(otherState, &accountID, "session4", "gpt-6-astra", "scanner")
	pool.setTargetLengths([]int{356, 292})

	c, _ := newTurnStateTestContext(t, 7, "client-session")
	h := http.Header{}
	gateway.guardOpenAICodexTurnStateEcho(c, &Account{ID: accountID, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, h, "gpt-5.5")
	require.Equal(t, state356, h.Get(openAICodexTurnStateHeader))
	_, ok := pool.preferredForBucket(accountID, "gpt-6-terra")
	require.False(t, ok)
	_, ok = pool.preferredForBucket(99, "gpt-5.5")
	require.False(t, ok)
	require.True(t, pool.hasReusableStateBeyond(accountID, "gpt-5.5", now.Add(time.Minute)))

	pool.setTargetLengths([]int{292, 356})
	selected, ok := pool.preferredForBucket(accountID, "gpt-5.5")
	require.True(t, ok)
	require.Equal(t, state292, selected)
	pool.setTargetLengths([]int{332})
	selected, ok = pool.preferredForBucket(accountID, "gpt-5.5")
	require.True(t, ok, "a replay-verified state remains valid when its length is not a configured preference")
	require.NotEmpty(t, selected)
	pool.setTargetLengths([]int{356})
	now = now.Add(time.Hour)
	_, ok = pool.preferredForBucket(accountID, "gpt-5.5")
	require.False(t, ok)
	require.False(t, pool.hasReusableStateBeyond(accountID, "gpt-5.5", now))
}
