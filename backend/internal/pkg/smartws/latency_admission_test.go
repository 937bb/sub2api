package smartws

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func latencyPool(t *testing.T) *Pool {
	t.Helper()
	// Production defaults except a loopback-only test endpoint and dialer.
	p := NewPool()
	p.config.allowed = func(u *url.URL) bool { return u != nil && u.Scheme == "http" && u.Hostname() == "127.0.0.1" }
	p.config.dialer = &websocket.Dialer{HandshakeTimeout: 5 * time.Second, NetDialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext}
	t.Cleanup(p.Close)
	return p
}
func latencyRequest(t *testing.T, target, body, window string) *http.Request {
	r := request(t, target, body, "same-session")
	r.Header.Set("X-Codex-Window-Id", window)
	r.Header.Set("X-Codex-Turn-State", "opaque-turn")
	r.Header.Set("X-Codex-Turn-Metadata", "opaque-metadata")
	return r
}

type latencyResult struct {
	r *http.Response
	h bool
	e error
}

func latencyStart(p *Pool, r *http.Request) <-chan latencyResult {
	ch := make(chan latencyResult, 1)
	go func() { a, b, c := p.Try(r, "", "group:37/user:1/key:1797"); ch <- latencyResult{a, b, c} }()
	return ch
}
func latencyGet(t *testing.T, ch <-chan latencyResult) latencyResult {
	t.Helper()
	select {
	case got := <-ch:
		return got
	case <-time.After(6 * time.Second):
		t.Fatal("request stalled")
		return latencyResult{}
	}
}
func latencyDrain(t *testing.T, got latencyResult) {
	t.Helper()
	if got.e != nil || !got.h || got.r == nil {
		t.Fatalf("expected WS response: %+v", got)
	}
	drainResponse(t, got.r)
}
func latencyClean(t *testing.T, p *Pool) {
	t.Helper()
	s := p.Stats()
	for _, k := range []string{"active", "connecting", "waiting", "waiting_bytes"} {
		if s[k] != 0 {
			t.Fatalf("leaked %s=%v: %v", k, s[k], s)
		}
	}
}
func latencyGatedServer(t *testing.T) (*httptest.Server, chan struct{}, func(), *atomic.Int64, *atomic.Int64) {
	gate := make(chan struct{})
	arrived := make(chan struct{}, 16)
	var once sync.Once
	release := func() { once.Do(func() { close(gate) }) }
	var sockets, messages atomic.Int64
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		arrived <- struct{}{}
		<-gate
		ws, e := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if e != nil {
			return
		}
		sockets.Add(1)
		defer ws.Close()
		for {
			_, _, e = ws.ReadMessage()
			if e != nil {
				return
			}
			n := messages.Add(1)
			id := fmt.Sprintf("latency-%d", n)
			if emit(ws, "response.created", id, "in_progress") != nil {
				return
			}
			if emit(ws, "response.completed", id, "completed") != nil {
				return
			}
		}
	}))
	t.Cleanup(func() { release(); s.Close() })
	return s, arrived, release, &sockets, &messages
}
func latencyArrivals(t *testing.T, ch <-chan struct{}, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		select {
		case <-ch:
		case <-time.After(2 * time.Second):
			t.Fatal("handshake did not arrive")
		}
	}
}

// Both releases execute this same actual-socket test using unchanged production
// 2500ms waiting, 0.8/s burst2, concurrent2 admission settings.
func TestLatencyColdBlockedHandshake(t *testing.T) {
	s, arrived, release, sockets, messages := latencyGatedServer(t)
	p := latencyPool(t)
	a := latencyStart(p, latencyRequest(t, s.URL, validBody, "window-a"))
	b := latencyStart(p, latencyRequest(t, s.URL, validBody, "window-b"))
	latencyArrivals(t, arrived, 2)
	body := "{\n \"model\":\"gpt-6-astra\", \"stream\":true, \"service_tier\":\"priority\", \"reasoning\":{\"effort\":\"xhigh\"}, \"input\":[{\"role\":\"user\",\"content\":[{\"type\":\"input_text\",\"text\":\"exact 完整上下文 <>&\"},{\"type\":\"input_image\",\"image_url\":\"data:image/png;base64,aGVsbG8=\"}]}], \"tools\":[{\"type\":\"function\",\"name\":\"lookup\",\"parameters\":{\"type\":\"object\"}}], \"include\":[\"reasoning.encrypted_content\"], \"prompt_cache_key\":\"unchanged\", \"store\":false\n}"
	r := latencyRequest(t, s.URL, body, "window-c")
	beforeHeaders := r.Header.Clone()
	before := time.Now()
	response, handled, err := p.Try(r, "", "group:37/user:1/key:1797")
	elapsed := time.Since(before)
	if response != nil || handled || err != nil {
		t.Fatalf("cold request dispatched %+v %v %v", response, handled, err)
	}
	raw, e := io.ReadAll(r.Body)
	if e != nil || string(raw) != body || !reflect.DeepEqual(r.Header, beforeHeaders) {
		t.Fatal("fallback altered original body or headers")
	}
	baseline := os.Getenv("EXPECT_BASELINE") == "1"
	if baseline {
		if elapsed < 2400*time.Millisecond || elapsed > 4*time.Second {
			t.Fatalf("old delay not reproduced: %s", elapsed)
		}
		if p.Stats()["wait_timeout"] != uint64(1) || p.Stats()["cold_identity_fast_fallback"] != nil {
			t.Fatal(p.Stats())
		}
	} else {
		if elapsed > 250*time.Millisecond {
			t.Fatalf("new cold fallback too slow: %s", elapsed)
		}
		if p.Stats()["cold_identity_fast_fallback"] != uint64(1) || p.Stats()["wait_started"] != nil || p.Stats()["wait_timeout"] != nil {
			t.Fatal(p.Stats())
		}
	}
	if p.Stats()["dial_concurrency_limited"] != uint64(1) || p.Stats()["waiting"] != 0 || p.Stats()["waiting_bytes"] != 0 {
		t.Fatal(p.Stats())
	}
	release()
	latencyDrain(t, latencyGet(t, a))
	latencyDrain(t, latencyGet(t, b))
	if sockets.Load() != 2 || messages.Load() != 2 {
		t.Fatalf("cold fallback sent extra generation sockets=%d messages=%d", sockets.Load(), messages.Load())
	}
	latencyClean(t, p)
	t.Logf("baseline=%t cold_fallback_ms=%.3f sockets=%d generations=%d body_bytes=%d body_and_headers_intact=true", baseline, float64(elapsed)/float64(time.Millisecond), sockets.Load(), messages.Load(), len(body))
}

func TestLatencyRateFastFallbackThenWarmReuse(t *testing.T) {
	s, sockets, messages, _ := successServer(t)
	p := latencyPool(t)
	for _, window := range []string{"a", "b"} {
		consume(t, p, latencyRequest(t, s.URL, validBody, window), "", "group:37/user:1/key:1797")
	}
	before := time.Now()
	r, h, e := p.Try(latencyRequest(t, s.URL, validBody, "c"), "", "group:37/user:1/key:1797")
	if e != nil || h || r != nil || time.Since(before) > 250*time.Millisecond {
		t.Fatal("cold rate wait or dispatch", time.Since(before), p.Stats())
	}
	consume(t, p, latencyRequest(t, s.URL, validBody, "a"), "", "group:37/user:1/key:1797")
	if sockets.Load() != 2 || messages.Load() != 3 || p.Stats()["reused"] != uint64(1) || p.Stats()["cold_identity_fast_fallback"] != uint64(1) || p.Stats()["dial_rate_limited"] != uint64(1) {
		t.Fatal(p.Stats())
	}
	latencyClean(t, p)
}

func TestLatencyImmediatePermitAndWindowIsolation(t *testing.T) {
	s, sockets, messages, _ := successServer(t)
	p := latencyPool(t)
	for _, window := range []string{"first", "second", "first"} {
		consume(t, p, latencyRequest(t, s.URL, validBody, window), "", "group:37/user:1/key:1797")
	}
	if sockets.Load() != 2 || messages.Load() != 3 || p.Stats()["reused"] != uint64(1) || p.Stats()["cold_identity_fast_fallback"] != nil {
		t.Fatal("identity isolation or immediate permit regressed", p.Stats())
	}
	latencyClean(t, p)
}

func TestLatencyWarmWaitingFIFOAndColdBypass(t *testing.T) {
	s, sockets, messages, _ := successServer(t)
	p := latencyPool(t)
	p.config.perKey = 1
	first := latencyGet(t, latencyStart(p, latencyRequest(t, s.URL, validBody, "warm")))
	a := latencyStart(p, latencyRequest(t, s.URL, validBody, "warm"))
	awaitWaiting(t, p, 1)
	b := latencyStart(p, latencyRequest(t, s.URL, validBody, "warm"))
	awaitWaiting(t, p, 2)
	before := time.Now()
	r, h, e := p.Try(latencyRequest(t, s.URL, validBody, "cold"), "", "group:37/user:1/key:1797")
	if r != nil || h || e != nil || time.Since(before) > 250*time.Millisecond || p.Stats()["waiting"] != 2 {
		t.Fatal("cold request stole FIFO or waited", p.Stats())
	}
	latencyDrain(t, first)
	ar := latencyGet(t, a)
	select {
	case <-b:
		t.Fatal("later warm waiter stole leased socket")
	default:
	}
	latencyDrain(t, ar)
	latencyDrain(t, latencyGet(t, b))
	if sockets.Load() != 1 || messages.Load() != 3 || p.Stats()["wait_reused"] != uint64(2) || p.Stats()["admission_fairness"] != uint64(1) {
		t.Fatal(p.Stats())
	}
	latencyClean(t, p)
}

func TestLatencyMatchingOpeningHandshakeStillWaits(t *testing.T) {
	s, arrived, release, sockets, messages := latencyGatedServer(t)
	p := latencyPool(t)
	p.config.perKey = 1
	first := latencyStart(p, latencyRequest(t, s.URL, validBody, "same"))
	latencyArrivals(t, arrived, 1)
	next := latencyStart(p, latencyRequest(t, s.URL, validBody, "same"))
	awaitWaiting(t, p, 1)
	if p.Stats()["cold_identity_fast_fallback"] != nil {
		t.Fatal("matching in-progress handshake considered cold")
	}
	release()
	latencyDrain(t, latencyGet(t, first))
	latencyDrain(t, latencyGet(t, next))
	if sockets.Load() != 1 || messages.Load() != 2 || p.Stats()["wait_reused"] != uint64(1) {
		t.Fatal(p.Stats())
	}
	latencyClean(t, p)
}

func TestLatencyWarmCancelBodyAndAccounting(t *testing.T) {
	s, sockets, messages, _ := successServer(t)
	p := latencyPool(t)
	p.config.perKey = 1
	first := latencyGet(t, latencyStart(p, latencyRequest(t, s.URL, validBody, "warm")))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := latencyRequest(t, s.URL, validBody, "warm").WithContext(ctx)
	next := latencyStart(p, req)
	awaitWaiting(t, p, 1)
	cancel()
	got := latencyGet(t, next)
	if !got.h || !errors.Is(got.e, context.Canceled) || got.r != nil {
		t.Fatalf("cancellation changed ownership %+v", got)
	}
	original, _ := io.ReadAll(req.Body)
	if string(original) != validBody {
		t.Fatal("cancel consumed original")
	}
	latencyDrain(t, first)
	if sockets.Load() != 1 || messages.Load() != 1 || p.Stats()["wait_cancelled"] != uint64(1) {
		t.Fatal(p.Stats())
	}
	latencyClean(t, p)
}

func TestLatencyWarmTimeoutAndQueueByteLimit(t *testing.T) {
	s, sockets, messages, _ := successServer(t)
	p := latencyPool(t)
	p.config.perKey = 1
	p.config.wait = 80 * time.Millisecond
	p.config.maxWaiters = 1
	p.config.accountWaiters = 1
	first := latencyGet(t, latencyStart(p, latencyRequest(t, s.URL, validBody, "warm")))
	next := latencyStart(p, latencyRequest(t, s.URL, validBody, "warm"))
	awaitWaiting(t, p, 1)
	r, h, e := p.Try(latencyRequest(t, s.URL, validBody, "warm"), "", "group:37/user:1/key:1797")
	if r != nil || h || e != nil || p.Stats()["wait_queue_full"] != uint64(1) {
		t.Fatal("warm queue bound lost", p.Stats())
	}
	got := latencyGet(t, next)
	if got.r != nil || got.h || got.e != nil || p.Stats()["wait_timeout"] != uint64(1) {
		t.Fatal("warm timeout lost", p.Stats())
	}
	p.config.maxWaitingBytes = 1
	r, h, e = p.Try(latencyRequest(t, s.URL, validBody, "warm"), "", "group:37/user:1/key:1797")
	if r != nil || h || e != nil || p.Stats()["wait_bytes_full"] != uint64(1) {
		t.Fatal("warm byte budget lost", p.Stats())
	}
	latencyDrain(t, first)
	if sockets.Load() != 1 || messages.Load() != 1 {
		t.Fatal("overflow generated")
	}
	latencyClean(t, p)
}

func TestLatencyUnavailableWarmBecomesColdAndLeavesQueue(t *testing.T) {
	s, sockets, messages, _ := successServer(t)
	p := latencyPool(t)
	p.config.perKey = 1
	p.config.dialBurst = 1
	p.config.dialRate = 0.0001
	first := latencyGet(t, latencyStart(p, latencyRequest(t, s.URL, validBody, "warm")))
	next := latencyStart(p, latencyRequest(t, s.URL, validBody, "warm"))
	awaitWaiting(t, p, 1)
	// Closing before consuming the terminal deliberately makes this socket non-reusable.
	before := time.Now()
	first.r.Body.Close()
	got := latencyGet(t, next)
	if got.r != nil || got.h || got.e != nil || time.Since(before) > 250*time.Millisecond {
		t.Fatal("lost warm connection stayed queued", p.Stats())
	}
	if sockets.Load() != 1 || messages.Load() != 1 || p.Stats()["cold_identity_fast_fallback"] != uint64(1) {
		t.Fatal(p.Stats())
	}
	latencyClean(t, p)
}

func TestLatencyNoReliableSessionNeverWaitsForReuse(t *testing.T) {
	s, sockets, messages, _ := successServer(t)
	p := latencyPool(t)
	p.config.dialBurst = 1
	p.config.dialRate = 0.0001
	firstReq := latencyRequest(t, s.URL, validBody, "window")
	firstReq.Header.Del("Session_id")
	first := latencyGet(t, latencyStart(p, firstReq))
	second := latencyRequest(t, s.URL, validBody, "window")
	second.Header.Del("Session_id")
	before := time.Now()
	r, h, e := p.Try(second, "", "group:37/user:1/key:1797")
	if r != nil || h || e != nil || time.Since(before) > 250*time.Millisecond {
		t.Fatal("unreusable identity waited", p.Stats())
	}
	latencyDrain(t, first)
	if sockets.Load() != 1 || messages.Load() != 1 || p.Stats()["cold_identity_fast_fallback"] != uint64(1) {
		t.Fatal(p.Stats())
	}
	latencyClean(t, p)
}

func TestLatencyFallbackRealHTTPBodyHeadersNoDoubleGeneration(t *testing.T) {
	var wsGenerations, httpGenerations atomic.Int64
	seen := make(chan []byte, 1)
	seenHeaders := make(chan http.Header, 1)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if websocket.IsWebSocketUpgrade(r) {
			ws, e := (&websocket.Upgrader{}).Upgrade(w, r, nil)
			if e != nil {
				return
			}
			defer ws.Close()
			for {
				_, _, e = ws.ReadMessage()
				if e != nil {
					return
				}
				id := fmt.Sprint(wsGenerations.Add(1))
				if emit(ws, "response.created", id, "in_progress") != nil {
					return
				}
				if emit(ws, "response.completed", id, "completed") != nil {
					return
				}
			}
		}
		httpGenerations.Add(1)
		raw, e := io.ReadAll(r.Body)
		if e != nil {
			w.WriteHeader(400)
			return
		}
		seen <- raw
		seenHeaders <- r.Header.Clone()
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"http-only\",\"status\":\"completed\"}}\n\n")
	}))
	t.Cleanup(s.Close)
	p := latencyPool(t)
	p.config.dialBurst = 1
	p.config.dialRate = 0.0001
	consume(t, p, latencyRequest(t, s.URL, validBody, "old"), "", "group:37/user:1/key:1797")
	body := "{ \"model\":\"gpt-6-astra\", \"stream\":true, \"reasoning\":{\"effort\":\"xhigh\"}, \"service_tier\":\"priority\", \"input\":[{\"role\":\"user\",\"content\":[{\"type\":\"input_text\",\"text\":\"prefix " + strings.Repeat("context preserved &<> ", 10000) + " suffix\"},{\"type\":\"input_image\",\"image_url\":\"data:image/png;base64,aGVsbG8=\"}]}],\"tools\":[{\"type\":\"function\",\"name\":\"lookup\",\"parameters\":{\"type\":\"object\"}}],\"include\":[\"reasoning.encrypted_content\"],\"prompt_cache_key\":\"same-cache\",\"store\":false }"
	req := latencyRequest(t, s.URL, body, "new")
	req.Header.Set("Content-Type", "application/json")
	wantedHeaders := req.Header.Clone()
	response, handled, err := p.Try(req, "", "group:37/user:1/key:1797")
	if handled || err != nil || response != nil {
		t.Fatal("expected safe fallback")
	}
	if !reflect.DeepEqual(req.Header, wantedHeaders) {
		t.Fatal("request headers mutated")
	}
	client := &http.Client{Timeout: 2 * time.Second}
	response, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || !bytes.Contains(raw, []byte("http-only")) {
		t.Fatal("HTTP completion lost")
	}
	actualBody := <-seen
	actualHeaders := <-seenHeaders
	if string(actualBody) != body {
		t.Fatal("full body changed during fallback")
	}
	for k, v := range wantedHeaders {
		if !reflect.DeepEqual(actualHeaders.Values(k), v) {
			t.Fatalf("header changed %s", k)
		}
	}
	if wsGenerations.Load() != 1 || httpGenerations.Load() != 1 || p.Stats()["dispatched"] != uint64(1) || p.Stats()["cold_identity_fast_fallback"] != uint64(1) {
		t.Fatal("fallback generated twice", p.Stats())
	}
	latencyClean(t, p)
	t.Logf("body_bytes=%d full_body_exact=true headers_exact=true ws_warmup=1 http_generation=1", len(body))
}
