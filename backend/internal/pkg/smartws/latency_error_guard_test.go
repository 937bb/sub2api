package smartws

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLatencyExplicit429RetainsAdmissionWait(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(429)
		_, _ = io.WriteString(w, `{"error":{"code":"rate_limit_exceeded","message":"fixture"}}`)
	}))
	defer s.Close()
	p := latencyPool(t)
	p.config.wait = 1500 * time.Millisecond
	for i := 0; i < 2; i++ {
		response, handled, err := p.Try(latencyRequest(t, s.URL, validBody, "error-window"), "", "group:37/user:1/key:1797")
		if err != nil || !handled || response == nil || response.StatusCode != 429 {
			t.Fatalf("explicit error lost: %v %v %v", response, handled, err)
		}
		response.Body.Close()
	}
	started := time.Now()
	response, handled, err := p.Try(latencyRequest(t, s.URL, validBody, "error-window"), "", "group:37/user:1/key:1797")
	elapsed := time.Since(started)
	if err != nil || !handled || response == nil || response.StatusCode != 429 {
		t.Fatalf("429 bypassed: %v %v %v", response, handled, err)
	}
	response.Body.Close()
	if elapsed < time.Second || p.Stats()["cold_identity_fast_fallback"] != nil {
		t.Fatalf("did not preserve admission: %v %v", elapsed, p.Stats())
	}
	p.mu.Lock()
	for _, st := range p.dials {
		st.httpErrorUntil = time.Now().Add(-time.Second)
		st.tokens = 0
		st.refilled = time.Now()
	}
	p.mu.Unlock()
	started = time.Now()
	response, handled, err = p.Try(latencyRequest(t, s.URL, validBody, "recovered-window"), "", "group:37/user:1/key:1797")
	if response != nil || handled || err != nil || time.Since(started) > 300*time.Millisecond {
		t.Fatal("expired guard did not restore fast path")
	}
	t.Logf("explicit 429 retained after %.3f ms; expired guard restored cold fallback", float64(elapsed)/float64(time.Millisecond))
}
func TestLatencyExplicitErrorGuardCannotBeErasedByOlderSuccess(t *testing.T) {
	p := latencyPool(t)
	now := time.Now()
	p.mu.Lock()
	a, _ := p.admitDialLocked("account", now)
	b, _ := p.admitDialLocked("account", now)
	p.finishDialLocked(b, "http_error", now)
	until := b.state.httpErrorUntil
	p.finishDialLocked(a, "success", now.Add(time.Millisecond))
	if !a.state.httpErrorUntil.Equal(until) {
		p.mu.Unlock()
		t.Fatal("older success erased explicit-error guard")
	}
	p.mu.Unlock()
}
func TestLatencyRecentExplicitErrorStateNotEvicted(t *testing.T) {
	p := latencyPool(t)
	now := time.Now()
	p.mu.Lock()
	for i := 0; i < maxNegativeEntries; i++ {
		key := string(rune(i + 1))
		p.dials[key] = &dialState{tokens: 2, used: now.Add(-90 * time.Second), refilled: now.Add(-90 * time.Second), httpErrorUntil: now.Add(30 * time.Second)}
	}
	permit, reason := p.admitDialLocked("fresh-account", now)
	p.mu.Unlock()
	if permit != nil || reason != "dial_state_full" {
		t.Fatal("active error guard evicted")
	}
}
