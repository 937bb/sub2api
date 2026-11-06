package smartws

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func retentionServer(t *testing.T) (*httptest.Server, *atomic.Int64, *atomic.Int64) {
	var opened, received atomic.Int64
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, e := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if e != nil {
			return
		}
		opened.Add(1)
		defer ws.Close()
		for {
			_, raw, e := ws.ReadMessage()
			if e != nil {
				return
			}
			var body map[string]any
			if json.Unmarshal(raw, &body) != nil {
				return
			}
			n := received.Add(1)
			id := fmt.Sprintf("retention-%d", n)
			if ws.WriteJSON(map[string]any{"type": "response.created", "response": map[string]any{"id": id, "status": "in_progress"}}) != nil {
				return
			}
			if ws.WriteJSON(map[string]any{"type": "response.output_text.delta", "response_id": id, "delta": body["input"]}) != nil {
				return
			}
			if ws.WriteJSON(map[string]any{"type": "response.completed", "response": map[string]any{"id": id, "status": "completed"}}) != nil {
				return
			}
		}
	}))
	t.Cleanup(s.Close)
	return s, &opened, &received
}
func retentionPool(t *testing.T, account, hard int, idle time.Duration) *Pool {
	p := newPool(poolConfig{capacity: 128, perKey: 64, adaptive: true, perAccount: account, hardCapacity: hard,
		wait: 2500 * time.Millisecond, maxWaiters: 256, accountWaiters: 32, maxWaitingBytes: 64 << 20, idle: idle, lifetime: 20 * time.Minute, handshake: 5 * time.Second,
		allowed: func(u *url.URL) bool { return u != nil && u.Scheme == "http" && u.Hostname() == "127.0.0.1" },
		dialer:  &websocket.Dialer{HandshakeTimeout: 5 * time.Second, ReadBufferSize: 16 << 10, WriteBufferSize: 16 << 10, NetDialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext},
		// Matrix isolates storage/retention under a reproducible arrival trace; production pacing is covered by unchanged admission tests.
		dialConcurrent: 128, dialBurst: 100000, dialRate: 100000})
	t.Cleanup(p.Close)
	return p
}
func TestRetentionCapacityMatrix(t *testing.T) {
	for _, c := range []struct {
		name          string
		account, hard int
	}{{"baseline", 128, 2048}, {"moderate", 192, 3072}, {"bounded_double", 256, 4096}, {"larger_account", 384, 4096}} {
		t.Run(c.name, func(t *testing.T) {
			s, opened, received := retentionServer(t)
			p := retentionPool(t, c.account, c.hard, 300*time.Second)
			const accounts = 12
			const identities = 220
			const rounds = 2
			start := time.Now()
			var maxTry time.Duration
			for round := 0; round < rounds; round++ {
				for a := 0; a < accounts; a++ {
					for i := 0; i < identities; i++ {
						text := fmt.Sprintf("a%d-i%d-r%d", a, i, round)
						raw, _ := json.Marshal(map[string]any{"model": "gpt-6-astra", "stream": true, "input": text, "reasoning": map[string]string{"effort": "xhigh"}, "service_tier": "priority"})
						req := request(t, s.URL, string(raw), fmt.Sprintf("session-%d", i))
						req.Header.Set("ChatGPT-Account-Id", fmt.Sprint(a))
						req.Header.Set("X-Codex-Window-Id", fmt.Sprint(i))
						tick := time.Now()
						resp, h, e := p.Try(req, "", fmt.Sprintf("user:1/key:1797/account:%d", a))
						duration := time.Since(tick)
						if duration > maxTry {
							maxTry = duration
						}
						if !h || e != nil || resp == nil {
							t.Fatalf("admission %v %v", h, e)
						}
						output, e := io.ReadAll(resp.Body)
						resp.Body.Close()
						if e != nil || !bytes.Contains(output, []byte(text)) {
							t.Fatal("cross-identity output or incomplete response", text, e)
						}
						if resp.Header.Get("X-Sub2-Upstream-Transport") != "websocket" {
							t.Fatal("not WS")
						}
					}
				}
			}
			stats := p.Stats()
			var mem runtime.MemStats
			runtime.ReadMemStats(&mem)
			if received.Load() != accounts*identities*rounds || stats["active"] != 0 || stats["waiting"] != 0 {
				t.Fatal(stats)
			}
			if stats["connections"].(int) > c.hard {
				t.Fatal("global bound")
			}
			if c.account >= identities && opened.Load() != accounts*identities {
				t.Fatal("retained identity was needlessly reopened", stats)
			}
			t.Logf("MATRIX name=%s requests=%d handshakes=%d reused=%v idle_reclaimed=%v retained=%v duration_s=%.3f max_acquire_ms=%.3f heap_inuse=%d", c.name, received.Load(), opened.Load(), stats["reused"], stats["idle_reclaimed"], stats["connections"], time.Since(start).Seconds(), float64(maxTry)/float64(time.Millisecond), mem.HeapInuse)
			p.Close()
		})
	}
}
func TestRetentionIdleAndLifetime(t *testing.T) {
	for _, seconds := range []int{120, 300} {
		s, opened, _ := retentionServer(t)
		p := retentionPool(t, 256, 4096, time.Duration(seconds)*time.Second)
		r := func() *http.Request {
			return request(t, s.URL, `{"model":"gpt-6-astra","stream":true,"input":"sentinel"}`, "retained")
		}
		consume(t, p, r(), "", "scope")
		p.mu.Lock()
		for c := range p.connections {
			c.used = time.Now().Add(-180 * time.Second)
		}
		p.mu.Unlock()
		consume(t, p, r(), "", "scope")
		want := int64(1)
		if seconds == 120 {
			want = 2
		}
		if opened.Load() != want {
			t.Fatalf("idle=%d %v", seconds, p.Stats())
		}
		p.mu.Lock()
		for c := range p.connections {
			c.created = time.Now().Add(-21 * time.Minute)
		}
		p.mu.Unlock()
		consume(t, p, r(), "", "scope")
		if opened.Load() != want+1 {
			t.Fatal("maximum lifetime ignored")
		}
		p.Close()
	}
}
func TestRetentionActiveSurvivesCapacityAndExpiry(t *testing.T) {
	s, _, _ := retentionServer(t)
	p := retentionPool(t, 2, 2, 300*time.Second)
	first, h, e := p.Try(request(t, s.URL, `{"model":"gpt-6-astra","stream":true,"input":"active"}`, "active"), "", "scope")
	if !h || e != nil {
		t.Fatal(e)
	}
	p.mu.Lock()
	for c := range p.connections {
		c.used = time.Now().Add(-time.Hour)
		c.created = time.Now().Add(-time.Hour)
	}
	p.mu.Unlock()
	p.expireIdle(time.Now())
	for _, id := range []string{"b", "c"} {
		consume(t, p, request(t, s.URL, `{"model":"gpt-6-astra","stream":true,"input":"idle"}`, id), "", "scope")
	}
	raw, e := io.ReadAll(first.Body)
	first.Body.Close()
	if e != nil || !bytes.Contains(raw, []byte("active")) {
		t.Fatal("active request disturbed", e)
	}
}
func TestRetentionDeadIdleNeverReused(t *testing.T) {
	var opened atomic.Int64
	closeIdle := make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, e := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if e != nil {
			return
		}
		opened.Add(1)
		defer ws.Close()
		_, _, e = ws.ReadMessage()
		if e != nil {
			return
		}
		emit(ws, "response.created", "r", "in_progress")
		emit(ws, "response.completed", "r", "completed")
		<-closeIdle
	}))
	defer s.Close()
	p := retentionPool(t, 256, 4096, 300*time.Second)
	consume(t, p, request(t, s.URL, validBody, "s"), "", "scope")
	close(closeIdle)
	deadline := time.Now().Add(2 * time.Second)
	for p.Stats()["connections"] != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if p.Stats()["connections"] != 0 {
		t.Fatal("dead idle retained")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r := request(t, s.URL, validBody, "s").WithContext(ctx)
	resp, h, e := p.Try(r, "", "scope")
	if e == nil && h && resp != nil {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	if opened.Load() != 2 {
		t.Fatal("dead connection reused")
	}
}
