package smartws

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func governedPool(t *testing.T) *Pool {
	p := testPool(t, 128)
	p.config.dialConcurrent = 2
	p.config.dialBurst = 4
	p.config.dialRate = 4
	p.config.dialBackoff = 40 * time.Millisecond
	p.config.dialBackoffMax = 160 * time.Millisecond
	return p
}
func TestBlank403FallbackUsesOriginalHTTPBodyOnce(t *testing.T) {
	var upgrades, httpCalls atomic.Int64
	original := `{"model":"test","stream":true,"instructions":"complete context","input":[{"role":"user","content":"unchanged <>"}],"tools":[{"type":"function","name":"calc","parameters":{"type":"object"}}],"reasoning":{"effort":"high"},"service_tier":"priority"}`
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if websocket.IsWebSocketUpgrade(r) {
			upgrades.Add(1)
			w.Header().Set("Cf-Ray", "abcd1234-LAX")
			w.WriteHeader(403)
			return
		}
		httpCalls.Add(1)
		raw, _ := io.ReadAll(r.Body)
		if string(raw) != original {
			t.Errorf("fallback body changed")
		}
		w.WriteHeader(200)
		_, _ = io.WriteString(w, "HTTP success")
	}))
	defer s.Close()
	p := governedPool(t)
	r := request(t, s.URL, original, "s")
	resp, handled, err := p.Try(r, "", "trusted-scope")
	if resp != nil || handled || err != nil {
		t.Fatalf("blank403 not safe fallback: %v %v", handled, err)
	}
	result, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, result.Body)
	result.Body.Close()
	if upgrades.Load() != 1 || httpCalls.Load() != 1 {
		t.Fatal("duplicate or missing generation")
	}
	for i := 0; i < 3; i++ {
		r = request(t, s.URL, original, fmt.Sprint(i))
		_, handled, err = p.Try(r, "", "other-scope")
		if handled || err != nil {
			t.Fatal("circuit not fallback")
		}
	}
	if upgrades.Load() != 1 {
		t.Fatal("circuit allowed new handshakes")
	}
	stats := p.Stats()
	if stats["blank_403_fallback"] != uint64(1) || stats["dial_backoff"] != uint64(3) {
		t.Fatal(stats)
	}
	d := stats["handshake_diagnostics"].([]handshakeDiagnostic)[0]
	if d.Status != 403 || d.BodyBytes != 0 || !d.BodyComplete || d.CFRay != "abcd1234-LAX" || d.Outcome != "blank_403" {
		t.Fatal(d)
	}
	if p.Stats()["dispatched"] != nil {
		t.Fatal("WS dispatched before fallback")
	}
}
func TestWhitespace403IsNotBlank(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403); io.WriteString(w, "  \n") }))
	defer s.Close()
	p := governedPool(t)
	resp, handled, err := p.Try(request(t, s.URL, validBody, "s"), "", "scope")
	if !handled || err != nil || resp.StatusCode != 403 {
		t.Fatal("nonempty rejection was swallowed")
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(raw) != "  \n" {
		t.Fatal(string(raw))
	}
}
func TestHandshakeBodySamplePreservesExplicitError(t *testing.T) {
	original := bytes.Repeat([]byte("denied-secret"), 2000)
	resp := &http.Response{Body: io.NopCloser(bytes.NewReader(original))}
	sample, complete := sampleHandshakeBody(resp)
	if complete || len(sample) != maxHandshakeBodySample+1 {
		t.Fatal("unbounded or incorrectly classified")
	}
	raw, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil || !bytes.Equal(raw, original) {
		t.Fatal("explicit body truncated")
	}
}
func TestDialBurstAndCredentialIsolation(t *testing.T) {
	s, connections, _, _ := successServer(t)
	p := governedPool(t)
	p.config.dialRate = 0.0001
	for i := 0; i < 4; i++ {
		consume(t, p, request(t, s.URL, validBody, fmt.Sprint(i)), "", "scope")
	}
	resp, handled, err := p.Try(request(t, s.URL, validBody, "fifth"), "", "scope")
	if resp != nil || handled || err != nil || connections.Load() != 4 || p.Stats()["dial_rate_limited"] != uint64(1) {
		t.Fatal("burst escaped", p.Stats())
	}
	r := request(t, s.URL, validBody, "fifth")
	r.Header.Set("Authorization", "Bearer different")
	r.Header.Set("ChatGPT-Account-Id", "different-account")
	consume(t, p, r, "", "scope")
	if connections.Load() != 5 {
		t.Fatal("separate credential was rate limited")
	}
}
func TestDialConcurrencyAcrossDifferentSessionKeys(t *testing.T) {
	arrived := make(chan struct{}, 4)
	release := make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { arrived <- struct{}{}; <-release; w.WriteHeader(401) }))
	defer s.Close()
	p := governedPool(t)
	done := make(chan struct{}, 2)
	for i := 0; i < 2; i++ {
		go func(n int) {
			resp, _, _ := p.Try(request(t, s.URL, validBody, fmt.Sprint(n)), "", "scope")
			if resp != nil {
				resp.Body.Close()
			}
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-arrived:
		case <-time.After(time.Second):
			t.Fatal("two dials not admitted")
		}
	}
	_, handled, err := p.Try(request(t, s.URL, validBody, "third"), "", "scope")
	if handled || err != nil || p.Stats()["dial_concurrency_limited"] != uint64(1) {
		t.Fatal(p.Stats())
	}
	close(release)
	<-done
	<-done
}
func TestHealthyReuseSurvivesAnotherKeyHandshakeCircuit(t *testing.T) {
	var connections, requests atomic.Int64
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Session_id") == "bad" {
			w.WriteHeader(403)
			return
		}
		ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		connections.Add(1)
		defer ws.Close()
		for {
			_, _, err = ws.ReadMessage()
			if err != nil {
				return
			}
			id := fmt.Sprint("resp-", requests.Add(1))
			emit(ws, "response.created", id, "in_progress")
			emit(ws, "response.completed", id, "completed")
		}
	}))
	defer s.Close()
	p := governedPool(t)
	consume(t, p, request(t, s.URL, validBody, "good"), "", "scope")
	_, handled, err := p.Try(request(t, s.URL, validBody, "bad"), "", "scope")
	if handled || err != nil {
		t.Fatal("expected fallback")
	}
	consume(t, p, request(t, s.URL, validBody, "good"), "", "scope")
	if connections.Load() != 1 || requests.Load() != 2 || p.Stats()["reused"] != uint64(1) {
		t.Fatal("healthy connection suppressed", p.Stats())
	}
}
func TestSingleHalfOpenAndExponentialBackoff(t *testing.T) {
	var attempts atomic.Int64
	arrived := make(chan struct{}, 1)
	release := make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n == 2 {
			arrived <- struct{}{}
			<-release
		}
		w.WriteHeader(403)
	}))
	defer s.Close()
	p := governedPool(t)
	p.Try(request(t, s.URL, validBody, "s"), "", "scope")
	p.mu.Lock()
	var st *dialState
	for _, x := range p.dials {
		st = x
	}
	st.blockedUntil = time.Now().Add(-time.Second)
	st.tokens = 4
	p.mu.Unlock()
	done := make(chan struct{})
	go func() { p.Try(request(t, s.URL, validBody, "probe"), "", "scope"); close(done) }()
	select {
	case <-arrived:
	case <-time.After(time.Second):
		t.Fatal("half-open not admitted")
	}
	for i := 0; i < 3; i++ {
		_, handled, err := p.Try(request(t, s.URL, validBody, fmt.Sprint("other", i)), "", "scope")
		if handled || err != nil {
			t.Fatal("parallel probe escaped")
		}
	}
	close(release)
	<-done
	if attempts.Load() != 2 || p.Stats()["dial_half_open_busy"] != uint64(3) {
		t.Fatal(p.Stats())
	}
	p.mu.Lock()
	failures, wait := st.failures, time.Until(st.blockedUntil)
	p.mu.Unlock()
	if failures != 2 || wait <= 40*time.Millisecond || wait > 90*time.Millisecond {
		t.Fatalf("backoff failures=%d wait=%v", failures, wait)
	}
}
func TestLateSuccessfulDialDoesNotClearNewRefusal(t *testing.T) {
	p := governedPool(t)
	p.mu.Lock()
	now := time.Now()
	first, _ := p.admitDialLocked("cap", now)
	second, _ := p.admitDialLocked("cap", now)
	p.finishDialLocked(first, "blank_403", now)
	p.finishDialLocked(second, "success", now)
	st := p.dials["cap"]
	if st.failures != 1 || !st.blockedUntil.After(now) {
		t.Fatal("older success erased refusal")
	}
	p.mu.Unlock()
}
func TestSuccessfulHalfOpenRestoresDialAdmission(t *testing.T) {
	p := governedPool(t)
	p.mu.Lock()
	now := time.Now()
	first, _ := p.admitDialLocked("cap", now)
	p.finishDialLocked(first, "blank_403", now)
	later := now.Add(time.Second)
	probe, denied := p.admitDialLocked("cap", later)
	if probe == nil || denied != "" || !probe.probe {
		t.Fatal("missing half-open")
	}
	p.finishDialLocked(probe, "success", later)
	if p.dials["cap"].failures != 0 || p.dials["cap"].probing {
		t.Fatal("half-open did not recover")
	}
	next, denied := p.admitDialLocked("cap", later)
	if next == nil || denied != "" {
		t.Fatal("admission not restored")
	}
	p.finishDialLocked(next, "success", later)
	p.mu.Unlock()
}
func TestInitialHandshakeHeadersPreservedWithoutStaleReuse(t *testing.T) {
	var requestCount atomic.Int64
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers := http.Header{"X-Codex-Turn-State": []string{"turn-state"}, "X-Codex-Primary-Used-Percent": []string{"12"}, "Set-Cookie": []string{"private-cookie"}}
		ws, err := (&websocket.Upgrader{}).Upgrade(w, r, headers)
		if err != nil {
			return
		}
		defer ws.Close()
		for {
			_, _, err := ws.ReadMessage()
			if err != nil {
				return
			}
			id := fmt.Sprint("resp-", requestCount.Add(1))
			emit(ws, "response.created", id, "in_progress")
			emit(ws, "response.completed", id, "completed")
		}
	}))
	defer s.Close()
	p := governedPool(t)
	for i := 0; i < 2; i++ {
		resp, handled, err := p.Try(request(t, s.URL, validBody, "s"), "", "scope")
		if err != nil || !handled {
			t.Fatal(err)
		}
		if i == 0 && (resp.Header.Get("X-Codex-Turn-State") != "turn-state" || resp.Header.Get("X-Codex-Primary-Used-Percent") != "12") {
			t.Fatal("metadata lost", resp.Header)
		}
		if i == 1 && (resp.Header.Get("X-Codex-Turn-State") != "" || resp.Header.Get("X-Codex-Primary-Used-Percent") != "") {
			t.Fatal("stale metadata replayed")
		}
		if resp.Header.Get("Set-Cookie") != "" {
			t.Fatal("unallowlisted header")
		}
		_, err = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
}
func TestHandshakeDiagnosticsBoundedAndSanitized(t *testing.T) {
	p := governedPool(t)
	headers := http.Header{"Authorization": []string{"Bearer supersecret"}, "X-Codex-Turn-State": []string{"private-turn"}, "Arbitrary-Secret-Name": []string{"hidden"}}
	for i := 0; i < 30; i++ {
		resp := &http.Response{StatusCode: 403, Header: http.Header{"Cf-Ray": []string{"secret raw\nvalue"}}}
		p.recordHandshake(strings.Repeat("a", 64), strings.Repeat("b", 64), headers, resp, []byte("secret body"), true, "http_error")
	}
	stats := p.Stats()
	diags := stats["handshake_diagnostics"].([]handshakeDiagnostic)
	if len(diags) != 16 {
		t.Fatal(len(diags))
	}
	for _, d := range diags {
		if d.CFRay != "" || d.HeaderLengths["authorization"] != 0 || len(d.HeaderLengths) != 1 {
			t.Fatal(d)
		}
	}
	rendered := fmt.Sprint(stats)
	for _, secret := range []string{"supersecret", "private-turn", "Arbitrary-Secret-Name", "hidden", "secret body"} {
		if strings.Contains(rendered, secret) {
			t.Fatal("secret in stats")
		}
	}
}
func TestDialStateBudgetRemainsBounded(t *testing.T) {
	p := governedPool(t)
	p.mu.Lock()
	for i := 0; i < maxNegativeEntries+40; i++ {
		now := time.Now().Add(time.Duration(i) * time.Minute)
		permit, why := p.admitDialLocked(fmt.Sprint(i), now)
		if permit == nil {
			t.Fatal(why)
		}
		p.finishDialLocked(permit, "success", now)
	}
	if len(p.dials) != maxNegativeEntries {
		t.Fatal(len(p.dials))
	}
	p.mu.Unlock()
}

func TestIncompleteOrUnknownLength403IsNotFallback(t *testing.T) {
	for _, length := range []int64{-1, 30} {
		resp := &http.Response{StatusCode: 403, ContentLength: length, Body: io.NopCloser(strings.NewReader(""))}
		data, complete := sampleHandshakeBody(resp)
		if complete || len(data) != 0 {
			t.Fatal("unproven empty body classified complete")
		}
		resp.Body.Close()
	}
}
