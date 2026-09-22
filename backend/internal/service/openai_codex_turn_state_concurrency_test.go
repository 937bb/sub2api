package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func stateConcurrencyProxies(count int) []*OpenAICodexTurnStateProxy {
	proxies := make([]*OpenAICodexTurnStateProxy, count)
	for i := range proxies {
		proxies[i] = &OpenAICodexTurnStateProxy{Source: "dynamic", ProxyURL: fmt.Sprintf("http://proxy-%d.example:8080", i)}
	}
	return proxies
}

func stateConcurrencyResult(length int, model string) openAICodexTurnStateHarvestResult {
	return openAICodexTurnStateHarvestResult{
		stateValue:  testOpenAICodexTurnState(length, time.Now().UTC().Add(-time.Minute), 'p'),
		stateLength: length, officialModel: model, upstreamOK: true, statusCode: http.StatusOK,
		sessionID: "codex:ts:test-harvest-session",
	}
}

func waitForStateProbes(t *testing.T, started <-chan string, count int) {
	t.Helper()
	for range count {
		select {
		case <-started:
		case <-time.After(3 * time.Second):
			t.Fatal("the full probe batch did not start concurrently")
		}
	}
}

func TestCodexStateFanoutStartsFirstBatchConcurrentlyAndDeduplicates(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	started := make(chan string, 10)
	release := make(chan struct{})
	var active, peak, calls atomic.Int32
	scanner := &openAICodexTurnStateScanner{probe: func(ctx context.Context, _ *Account, model, proxy string) openAICodexTurnStateHarvestResult {
		calls.Add(1)
		current := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); current > old && !peak.CompareAndSwap(old, current); old = peak.Load() {
		}
		started <- proxy
		select {
		case <-release:
			return stateConcurrencyResult(292, model)
		case <-ctx.Done():
			return openAICodexTurnStateHarvestResult{errorMessage: "cancelled"}
		}
	}}
	proxies := stateConcurrencyProxies(8)
	proxies = append([]*OpenAICodexTurnStateProxy{proxies[0], proxies[0]}, proxies...)
	done := make(chan []openAICodexTurnStateProbe, 1)
	go func() {
		done <- scanner.probeWithBoundedFanout(ctx, &Account{ID: 41}, "gpt-6-astra", proxies, defaultOpenAICodexTurnStateScanSettings())
	}()
	waitForStateProbes(t, started, 5)
	require.Equal(t, int32(5), peak.Load())
	close(release)
	select {
	case results := <-done:
		require.Len(t, results, 5)
		seen := make(map[string]bool)
		for _, result := range results {
			require.False(t, seen[result.proxy.ProxyURL])
			seen[result.proxy.ProxyURL] = true
		}
	case <-ctx.Done():
		t.Fatal("probe batch did not finish")
	}
	require.Equal(t, int32(5), calls.Load())
	require.Zero(t, active.Load())
}

func TestCodexStateFanoutCancelsSiblingsOnConfiguredPrimary(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	started := make(chan string, 5)
	release := make(chan struct{})
	var cancelled atomic.Int32
	proxies := stateConcurrencyProxies(5)
	settings := defaultOpenAICodexTurnStateScanSettings()
	settings.TargetLengths = []int{292, 332}
	scanner := &openAICodexTurnStateScanner{probe: func(ctx context.Context, _ *Account, model, proxy string) openAICodexTurnStateHarvestResult {
		started <- proxy
		select {
		case <-release:
		case <-ctx.Done():
			cancelled.Add(1)
			return openAICodexTurnStateHarvestResult{errorMessage: "cancelled"}
		}
		if proxy == proxies[0].ProxyURL {
			return stateConcurrencyResult(292, model)
		}
		<-ctx.Done()
		cancelled.Add(1)
		return openAICodexTurnStateHarvestResult{errorMessage: "cancelled"}
	}}
	done := make(chan []openAICodexTurnStateProbe, 1)
	go func() {
		done <- scanner.probeWithBoundedFanout(ctx, &Account{ID: 41}, "gpt-6-astra", proxies, settings)
	}()
	waitForStateProbes(t, started, 5)
	close(release)
	select {
	case results := <-done:
		result, _ := chooseOpenAICodexTurnStateResult("gpt-6-astra", results, settings)
		require.Equal(t, 292, result.stateLength)
		require.Equal(t, int32(4), cancelled.Load())
	case <-ctx.Done():
		t.Fatal("primary result did not cancel pending probes")
	}
}

func TestCodexStateFanoutDoesNotAcceptWrongModelOrExpiredPrimary(t *testing.T) {
	for _, reason := range []string{"wrong model", "expired"} {
		t.Run(reason, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			started := make(chan string, 2)
			primaryReturned := make(chan struct{})
			fallbackReady := make(chan struct{})
			proxies := stateConcurrencyProxies(2)
			settings := defaultOpenAICodexTurnStateScanSettings()
			settings.ParallelProbes = 2
			scanner := &openAICodexTurnStateScanner{probe: func(ctx context.Context, _ *Account, model, proxy string) openAICodexTurnStateHarvestResult {
				started <- proxy
				if proxy == proxies[0].ProxyURL {
					result := stateConcurrencyResult(332, model)
					if reason == "wrong model" {
						result.officialModel = "gpt-5.6-luna"
					} else {
						result.stateValue = testOpenAICodexTurnState(332, time.Now().Add(-2*time.Hour), 'e')
					}
					close(primaryReturned)
					return result
				}
				select {
				case <-fallbackReady:
					return stateConcurrencyResult(292, model)
				case <-ctx.Done():
					return openAICodexTurnStateHarvestResult{errorMessage: "cancelled"}
				}
			}}
			done := make(chan []openAICodexTurnStateProbe, 1)
			go func() {
				done <- scanner.probeWithBoundedFanout(ctx, &Account{ID: 41}, "gpt-6-astra", proxies, settings)
			}()
			waitForStateProbes(t, started, 2)
			<-primaryReturned
			close(fallbackReady)
			select {
			case results := <-done:
				result, _ := chooseOpenAICodexTurnStateResult("gpt-6-astra", results, settings)
				require.Equal(t, 292, result.stateLength)
			case <-ctx.Done():
				t.Fatal("fallback probe did not finish")
			}
		})
	}
}

type stateHealthRecordingRepo struct {
	OpenAICodexTurnStateScannerRepository
	mu      sync.Mutex
	updated []int64
}

func (r *stateHealthRecordingRepo) UpdateOpenAICodexTurnStateProxyHealth(_ context.Context, proxy *OpenAICodexTurnStateProxy) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.updated = append(r.updated, proxy.ID)
	return nil
}

func TestCodexStateProbeNeverChangesBusinessProxyHealth(t *testing.T) {
	repo := &stateHealthRecordingRepo{}
	scanner := &openAICodexTurnStateScanner{repo: repo}
	for _, proxy := range []*OpenAICodexTurnStateProxy{
		{ID: 1, Source: "shared"}, {Source: "dynamic"}, {ID: 2, Source: "state"},
	} {
		scanner.updateProxyHealth(context.Background(), proxy, openAICodexTurnStateHarvestResult{errorMessage: "transport failed"})
	}
	require.Equal(t, []int64{2}, repo.updated)
}

func TestCodexStateSelectionRespectsConfiguredOrderInsteadOfNumericLength(t *testing.T) {
	settings := defaultOpenAICodexTurnStateScanSettings()
	settings.TargetLengths = []int{292, 356, 332}
	probes := []openAICodexTurnStateProbe{
		{result: stateConcurrencyResult(332, "gpt-6-astra")},
		{result: stateConcurrencyResult(356, "gpt-6-astra")},
		{result: stateConcurrencyResult(292, "gpt-6-astra")},
	}
	result, _ := chooseOpenAICodexTurnStateResult("gpt-6-astra", probes, settings)
	require.Equal(t, 292, result.stateLength)
	result, _ = chooseOpenAICodexTurnStateResult("gpt-6-astra", probes[:2], settings)
	require.Equal(t, 356, result.stateLength)
}

func TestCodexStateSelectionUsesFreshnessForUnconfiguredValidLengths(t *testing.T) {
	settings := defaultOpenAICodexTurnStateScanSettings()
	settings.TargetLengths = []int{332, 292}
	older := time.Now().UTC().Add(-2 * time.Minute)
	newer := older.Add(time.Minute)
	probes := []openAICodexTurnStateProbe{
		{result: openAICodexTurnStateHarvestResult{
			stateValue: testOpenAICodexTurnState(312, older, 'a'), stateLength: 312,
			officialModel: "gpt-6-astra", upstreamOK: true, statusCode: http.StatusOK,
		}},
		{result: openAICodexTurnStateHarvestResult{
			stateValue: testOpenAICodexTurnState(308, newer, 'b'), stateLength: 308,
			officialModel: "gpt-6-astra", upstreamOK: true, statusCode: http.StatusOK,
		}},
	}
	result, _ := chooseOpenAICodexTurnStateResult("gpt-6-astra", probes, settings)
	require.Equal(t, 308, result.stateLength)
}

func TestCodexStateRotatingGatewayKeepsIndependentlyVerifiedExits(t *testing.T) {
	proxies := []*OpenAICodexTurnStateProxy{
		{Source: "dynamic", ProxyURL: "http://gateway.example:8080", ExitIP: "8.8.8.1"},
		{Source: "dynamic", ProxyURL: "http://gateway.example:8080", ExitIP: "8.8.8.2"},
		{Source: "dynamic", ProxyURL: "http://gateway.example:8080", ExitIP: "8.8.8.3"},
		{Source: "dynamic", ProxyURL: "http://other.example:8080", ExitIP: "8.8.8.1"},
	}
	selected := mergeOpenAICodexTurnStateScanProxies(proxies)
	require.Len(t, selected, 3)
	for _, proxy := range selected {
		require.Equal(t, "http://gateway.example:8080", proxy.ProxyURL)
	}
	var calls atomic.Int32
	scanner := &openAICodexTurnStateScanner{probe: func(_ context.Context, _ *Account, model, proxy string) openAICodexTurnStateHarvestResult {
		calls.Add(1)
		return stateConcurrencyResult(292, model)
	}}
	results := scanner.probeWithBoundedFanout(context.Background(), &Account{ID: 42}, "gpt-6-astra", selected, defaultOpenAICodexTurnStateScanSettings())
	require.Len(t, results, 3)
	require.Equal(t, int32(3), calls.Load())
}

type stateHarvestPoolUpstream struct {
	HTTPUpstream
	capacity int
	profile  HTTPUpstreamProfile
	close    bool
}

func (u *stateHarvestPoolUpstream) Do(req *http.Request, _ string, _ int64, concurrency int) (*http.Response, error) {
	u.capacity = concurrency
	u.profile = HTTPUpstreamProfileFromContext(req.Context())
	u.close = req.Close
	return nil, io.EOF
}

func TestCodexStateHarvestTransportCanDialBatchWithoutChangingAccountLimit(t *testing.T) {
	upstream := &stateHarvestPoolUpstream{}
	gateway := ticketTestService(t, config.OpenAICodexTicketConfig{}, upstream)
	account := ticketTestAccount(41)
	account.Concurrency = 1
	before := account.Mode1EffectiveConcurrency()
	gateway.harvestOpenAICodexTurnState(context.Background(), account, "gpt-6-astra", "http://gateway.example:8080")
	require.GreaterOrEqual(t, upstream.capacity, 5)
	require.Equal(t, HTTPUpstreamProfileOpenAIHarvest, upstream.profile)
	require.True(t, upstream.close)
	require.Equal(t, before, account.Mode1EffectiveConcurrency())
}

func TestCodexStateWorkspaceLockSerializesTeamMembers(t *testing.T) {
	scanner := &openAICodexTurnStateScanner{}
	first := ticketTestAccount(41)
	second := ticketTestAccount(42)
	first.Credentials["chatgpt_account_id"] = "workspace-1"
	second.Credentials["chatgpt_account_id"] = "workspace-1"

	unlockFirst := scanner.lockWorkspace(first)
	acquiredSecond := make(chan func(), 1)
	go func() {
		acquiredSecond <- scanner.lockWorkspace(second)
	}()

	select {
	case unlockSecond := <-acquiredSecond:
		unlockSecond()
		t.Fatal("a second Team member acquired the same workspace lock")
	case <-time.After(50 * time.Millisecond):
	}

	unlockFirst()
	select {
	case unlockSecond := <-acquiredSecond:
		unlockSecond()
	case <-time.After(time.Second):
		t.Fatal("the second Team member did not acquire the released workspace lock")
	}
}
