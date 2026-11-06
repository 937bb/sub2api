package smartws

import (
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"
)

func TestRetentionProductionBudgetsAndRealCeilings(t *testing.T) {
	defaults := NewPool()
	cfg := defaults.config
	defaults.Close()
	if cfg.perAccount != 256 || cfg.hardCapacity != 4096 || cfg.idle != 300*time.Second || cfg.perKey != 64 || cfg.lifetime != 20*time.Minute || cfg.dialRate != 0.9 || cfg.dialBurst != 2 || cfg.dialConcurrent != 2 {
		t.Fatalf("unexpected production defaults %+v", cfg)
	}
	s, opened, received := retentionServer(t)
	p := retentionPool(t, cfg.perAccount, cfg.hardCapacity, cfg.idle)
	beforeFD, _ := os.ReadDir("/proc/self/fd")
	start := time.Now()
	for a := 0; a < 20; a++ {
		for i := 0; i < 220; i++ {
			r := request(t, s.URL, validBody, fmt.Sprintf("ceiling-%d", i))
			r.Header.Set("ChatGPT-Account-Id", fmt.Sprint(a))
			consume(t, p, r, "", "scope")
		}
	}
	stats := p.Stats()
	if stats["connections"] != 4096 || stats["capacity"] != 4096 || stats["idle_reclaimed_global"] != uint64(304) || stats["active"] != 0 {
		t.Fatal("real global ceiling failed", stats)
	}
	p.mu.Lock()
	for _, st := range p.accounts {
		if st.sockets+st.opening > 256 {
			t.Fatal("account bound exceeded")
		}
	}
	p.mu.Unlock()
	runtime.GC()
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	t.Logf("REAL_BOUND sockets=%v handshakes=%d messages=%d seconds=%.3f heap_inuse_bytes=%d", stats["connections"], opened.Load(), received.Load(), time.Since(start).Seconds(), mem.HeapInuse)
	p.Close()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		fd, _ := os.ReadDir("/proc/self/fd")
		if len(fd) <= len(beforeFD)+8 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	afterFD, _ := os.ReadDir("/proc/self/fd")
	if len(afterFD) > len(beforeFD)+8 {
		t.Fatalf("descriptor leak before=%d after=%d", len(beforeFD), len(afterFD))
	}
}
