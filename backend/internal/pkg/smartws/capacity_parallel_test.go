package smartws

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// This validates local pool mechanics only, not any provider's account quota.
func TestCapacity150ConcurrentLeasesAndReuse(t *testing.T) {
	const concurrency = 150
	arrived := make(chan struct{}, concurrency*2)
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	var sockets, messages atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		sockets.Add(1)
		defer ws.Close()
		for {
			if _, _, err := ws.ReadMessage(); err != nil {
				return
			}
			n := messages.Add(1)
			id := fmt.Sprintf("capacity-%d", n)
			if emit(ws, "response.created", id, "in_progress") != nil {
				return
			}
			arrived <- struct{}{}
			<-release
			if emit(ws, "response.completed", id, "completed") != nil {
				return
			}
		}
	}))
	defer func() { unblock(); server.Close() }()
	c := defaultPoolConfig()
	c.perKey = 160
	c.allowed = func(u *url.URL) bool { return u != nil && u.Scheme == "http" && u.Hostname() == "127.0.0.1" }
	// Remove only the handshake pacing bottleneck in this local capacity test.
	// Production pacing remains unchanged and is exercised in admission tests.
	c.dialConcurrent = concurrency
	c.dialBurst = concurrency
	c.dialRate = 1000
	p := newPool(c)
	defer p.Close()
	reqs := make([]*http.Request, concurrency)
	responses := make([]<-chan latencyResult, concurrency)
	for i := range responses {
		reqs[i] = request(t, server.URL, validBody, "shared-session")
		reqs[i] = reqs[i].WithContext(WithAccountID(reqs[i].Context(), 42))
		responses[i] = latencyStart(p, reqs[i])
	}
	deadline := time.After(15 * time.Second)
	for i := 0; i < concurrency; i++ {
		select {
		case <-arrived:
		case <-deadline:
			unblock()
			t.Fatal("local mock did not receive all generations", p.Stats())
		}
	}
	rows := p.Stats()["accounts"].([]AccountSnapshot)
	if len(rows) != 1 || rows[0].Active != concurrency || rows[0].Waiting != 0 {
		unblock()
		t.Fatal("not actually concurrent", rows)
	}
	unblock()
	for _, ch := range responses {
		latencyDrain(t, latencyGet(t, ch))
	}
	for i := range responses {
		req := request(t, server.URL, validBody, "shared-session")
		req = req.WithContext(WithAccountID(req.Context(), 42))
		responses[i] = latencyStart(p, req)
	}
	deadline = time.After(15 * time.Second)
	for range responses {
		select {
		case <-arrived:
		case <-deadline:
			t.Fatal("local mock did not receive the concurrent reuse round", p.Stats())
		}
	}
	rows = p.Stats()["accounts"].([]AccountSnapshot)
	if len(rows) != 1 || rows[0].Active != concurrency || sockets.Load() != concurrency {
		t.Fatal("second round did not reuse the concurrent pool", rows, sockets.Load())
	}
	for _, ch := range responses {
		latencyDrain(t, latencyGet(t, ch))
	}
	rows = p.Stats()["accounts"].([]AccountSnapshot)
	if sockets.Load() != concurrency || messages.Load() != 2*concurrency || rows[0].Counters["completed"] != 2*concurrency || rows[0].Counters["reused"] != concurrency {
		t.Fatal("extra dials, missing completion, or failed reuse", sockets.Load(), messages.Load(), rows)
	}
	latencyClean(t, p)
	t.Logf("local_mock peak_active=%d opened=%d completed=%d reused=%d", concurrency, sockets.Load(), rows[0].Counters["completed"], rows[0].Counters["reused"])
}
