package smartws

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func adaptivePool(t *testing.T, perAccount, hard int, wait time.Duration) *Pool {
	p := testPool(t, perAccount)
	p.mu.Lock()
	p.config.adaptive = true
	p.config.perAccount = perAccount
	p.config.perKey = 64
	p.config.hardCapacity = hard
	p.config.wait = wait
	p.config.maxWaiters = 256
	p.config.accountWaiters = 32
	p.config.maxWaitingBytes = 64 << 20
	p.config.dialConcurrent = 128
	p.config.dialBurst = 10000
	p.config.dialRate = 100000
	p.mu.Unlock()
	return p
}
func countedServer(t *testing.T, hold <-chan struct{}) (*httptest.Server, *atomic.Int64, *atomic.Int64) {
	var opened, messages atomic.Int64
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		opened.Add(1)
		defer ws.Close()
		for {
			_, _, err = ws.ReadMessage()
			if err != nil {
				return
			}
			n := messages.Add(1)
			id := fmt.Sprintf("adaptive-%d", n)
			if emit(ws, "response.created", id, "in_progress") != nil {
				return
			}
			if hold != nil && strings.HasPrefix(r.Header.Get("Session_id"), "hold") {
				<-hold
			}
			if emit(ws, "response.completed", id, "completed") != nil {
				return
			}
		}
	}))
	t.Cleanup(s.Close)
	return s, &opened, &messages
}
func awaitWaiting(t *testing.T, p *Pool, n int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if p.Stats()["waiting"] == n {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("waiting wanted %d: %v", n, p.Stats())
}
func drainResponse(t *testing.T, r *http.Response) {
	t.Helper()
	_, err := io.Copy(io.Discard, r.Body)
	r.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func TestAdaptiveFiveAccounts150SimultaneousSockets(t *testing.T) {
	s, opened, messages := countedServer(t, nil)
	p := adaptivePool(t, 32, 256, 500*time.Millisecond)
	type result struct {
		response *http.Response
		handled  bool
		err      error
	}
	results := make(chan result, 150)
	var start sync.WaitGroup
	start.Add(1)
	for a := 0; a < 5; a++ {
		for i := 0; i < 30; i++ {
			go func(a, i int) {
				start.Wait()
				r := request(t, s.URL, validBody, fmt.Sprintf("a%d-s%d", a, i))
				r.Header.Set("ChatGPT-Account-Id", fmt.Sprintf("account-%d", a))
				response, h, e := p.Try(r, "", fmt.Sprintf("trusted-account-%d", a))
				results <- result{response, h, e}
			}(a, i)
		}
	}
	start.Done()
	var responses []*http.Response
	for i := 0; i < 150; i++ {
		select {
		case got := <-results:
			if got.err != nil || !got.handled || got.response == nil {
				t.Fatalf("admission %v %+v", p.Stats(), got)
			}
			responses = append(responses, got.response)
		case <-time.After(5 * time.Second):
			t.Fatal("150 socket admission stalled")
		}
	}
	stats := p.Stats()
	if stats["connections"] != 150 || stats["active"] != 150 || stats["capacity"] != 160 || stats["account_count"] != 5 {
		t.Fatal("accounts did not scale independently", stats)
	}
	for _, r := range responses {
		drainResponse(t, r)
	}
	if opened.Load() != 150 || messages.Load() != 150 || p.Stats()["active"] != 0 {
		t.Fatal("dispatch/lease leak", p.Stats())
	}
	if p.Stats()["waiting"] != 0 || p.Stats()["waiting_bytes"] != 0 {
		t.Fatal("wait leak", p.Stats())
	}
}

func TestAdaptiveLRUReclaimsIdleNeverActive(t *testing.T) {
	hold := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(hold) })
	s, opened, _ := countedServer(t, hold)
	p := adaptivePool(t, 2, 2, 100*time.Millisecond)
	active, h, e := p.Try(request(t, s.URL, validBody, "hold-active"), "", "scope")
	if e != nil || !h {
		t.Fatal(e)
	}
	consume(t, p, request(t, s.URL, validBody, "old-idle"), "", "scope")
	consume(t, p, request(t, s.URL, validBody, "replacement"), "", "scope")
	if p.Stats()["idle_reclaimed"] != uint64(1) || p.Stats()["active"] != 1 || opened.Load() != 3 {
		t.Fatal("pressure did not evict only idle", p.Stats())
	}
	once.Do(func() { close(hold) })
	drainResponse(t, active)
	if p.Stats()["completed"] != uint64(3) || p.Stats()["stream_failed"] != nil {
		t.Fatal("active lease was disturbed", p.Stats())
	}
}

func TestAdaptiveWaitReusesReleasedExactIdentity(t *testing.T) {
	s, opened, _ := countedServer(t, nil)
	p := adaptivePool(t, 1, 1, 500*time.Millisecond)
	first, h, e := p.Try(request(t, s.URL, validBody, "same"), "", "scope")
	if e != nil || !h {
		t.Fatal(e)
	}
	type result struct {
		r *http.Response
		h bool
		e error
	}
	done := make(chan result, 1)
	go func() { r, h, e := p.Try(request(t, s.URL, validBody, "same"), "", "scope"); done <- result{r, h, e} }()
	awaitWaiting(t, p, 1)
	drainResponse(t, first)
	got := <-done
	if got.e != nil || !got.h || got.r == nil {
		t.Fatalf("%+v", got)
	}
	drainResponse(t, got.r)
	if opened.Load() != 1 || p.Stats()["reused"] != uint64(1) || p.Stats()["wait_reused"] != uint64(1) {
		t.Fatal("wait did not reuse", p.Stats())
	}
	awaitWaiting(t, p, 0)
}

func TestAdaptiveNoLRUEvictionWithoutHandshakePermit(t *testing.T) {
	s, opened, _ := countedServer(t, nil)
	p := adaptivePool(t, 1, 1, 0)
	p.config.dialBurst = 1
	p.config.dialRate = 0.0001
	consume(t, p, request(t, s.URL, validBody, "healthy"), "", "scope")
	_, h, e := p.Try(request(t, s.URL, validBody, "other"), "", "scope")
	if h || e != nil || p.Stats()["idle_reclaimed"] != nil || p.Stats()["connections"] != 1 {
		t.Fatal("healthy idle destroyed without permit", p.Stats())
	}
	consume(t, p, request(t, s.URL, validBody, "healthy"), "", "scope")
	if opened.Load() != 1 || p.Stats()["reused"] != uint64(1) {
		t.Fatal(p.Stats())
	}
}

func TestAdaptiveNegativeCacheAllowsExistingHealthyReuse(t *testing.T) {
	s, opened, _ := countedServer(t, nil)
	p := adaptivePool(t, 4, 4, 0)
	consume(t, p, request(t, s.URL, validBody, "healthy"), "", "scope")
	p.mu.Lock()
	for c := range p.connections {
		p.negative[c.capabilityKey] = time.Now().Add(time.Minute)
	}
	p.mu.Unlock()
	consume(t, p, request(t, s.URL, validBody, "healthy"), "", "scope")
	_, h, e := p.Try(request(t, s.URL, validBody, "different"), "", "scope")
	if h || e != nil || opened.Load() != 1 || p.Stats()["unsupported_cached"] != uint64(1) {
		t.Fatal(p.Stats())
	}
}

func TestAdaptiveAccountRateSurvivesTokenBetaAndProxyChanges(t *testing.T) {
	s, opened, _ := countedServer(t, nil)
	p := adaptivePool(t, 16, 64, 0)
	p.config.dialBurst = 1
	p.config.dialRate = 0.0001
	consume(t, p, request(t, s.URL, validBody, "first"), "", "scope")
	var proxyCalls atomic.Int64
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { proxyCalls.Add(1); w.WriteHeader(502) }))
	defer proxy.Close()
	for i := 0; i < 3; i++ {
		r := request(t, s.URL, validBody, fmt.Sprint(i))
		if i == 0 {
			r.Header.Set("Authorization", "Bearer rotated")
		}
		if i == 1 {
			r.Header.Set("OpenAI-Beta", "responses_websockets=changed")
		}
		proxyURL := ""
		if i == 2 {
			proxyURL = proxy.URL
		}
		_, h, e := p.Try(r, proxyURL, "scope")
		if h || e != nil {
			t.Fatal("stable account got new bucket", i, p.Stats())
		}
	}
	r := request(t, s.URL, validBody, "another-account")
	r.Header.Set("ChatGPT-Account-Id", "genuinely-different-account")
	consume(t, p, r, "", "scope")
	if opened.Load() != 2 || proxyCalls.Load() != 0 || p.Stats()["dial_rate_limited"] != uint64(3) || p.Stats()["account_count"] != 2 {
		t.Fatal("limiter identity mismatch", p.Stats())
	}
}

func TestAdaptiveDialChurnCannotResetRecentBuckets(t *testing.T) {
	p := adaptivePool(t, 4, 64, 0)
	p.config.dialBurst = 2
	p.config.dialRate = 0.8
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Unix(10000, 0)
	for i := 0; i < maxNegativeEntries; i++ {
		permit, why := p.admitDialLocked(fmt.Sprint(i), now)
		if permit == nil {
			t.Fatal(why)
		}
		p.finishDialLocked(permit, "success", now)
	}
	if permit, why := p.admitDialLocked("new", now.Add(time.Second)); permit != nil || why != "dial_state_full" {
		t.Fatal("recent bucket eviction resets rate", why)
	}
	permit, why := p.admitDialLocked("0", now)
	if permit == nil {
		t.Fatal(why)
	}
	p.finishDialLocked(permit, "success", now)
	if permit, why = p.admitDialLocked("0", now); permit != nil || why != "dial_rate_limited" {
		t.Fatal("spent bucket replenished", why)
	}
	if permit, why = p.admitDialLocked("new", now.Add(2*time.Minute)); permit == nil {
		t.Fatal("quiet full buckets never retire", why)
	}
	p.finishDialLocked(permit, "success", now.Add(2*time.Minute))
	if len(p.dials) != maxNegativeEntries {
		t.Fatal(len(p.dials))
	}
}

func TestAdaptiveRetiredFailedAccountsDoNotPermanentlyExhaustState(t *testing.T) {
	p := adaptivePool(t, 4, 64, 0)
	p.config.dialBurst = 2
	p.config.dialRate = 0.8
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Unix(20000, 0)
	for i := 0; i < maxNegativeEntries; i++ {
		permit, why := p.admitDialLocked(fmt.Sprint(i), now)
		if permit == nil {
			t.Fatal(why)
		}
		p.finishDialLocked(permit, "blank_403", now)
	}
	if permit, why := p.admitDialLocked("new", now.Add(time.Minute)); permit != nil || why != "dial_state_full" {
		t.Fatal("recent failed bucket evicted", why)
	}
	permit, why := p.admitDialLocked("new", now.Add(3*time.Minute))
	if permit == nil {
		t.Fatal("retired failed accounts permanently block new account", why)
	}
	p.finishDialLocked(permit, "success", now.Add(3*time.Minute))
}
