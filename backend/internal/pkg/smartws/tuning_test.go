package smartws

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestTuningValidation(t *testing.T) {
	for _, tt := range []struct {
		tuning  Tuning
		invalid bool
	}{
		{Tuning{}, false}, {Tuning{IdentityCapacity: 160, ColdWaitMillis: 250}, false},
		{Tuning{IdentityCapacity: 257}, true}, {Tuning{IdentityCapacity: 160, AccountCapacity: 128}, true},
		{Tuning{CapacityLimit: 128}, true}, {Tuning{AccountCapacity: 513}, true},
		{Tuning{IdleSeconds: 901}, true}, {Tuning{IdleSeconds: 29}, true},
		{Tuning{ColdWaitMillis: -1}, true}, {Tuning{ColdWaitMillis: 501}, true},
	} {
		p, err := NewPoolWithTuning(nil, tt.tuning)
		if p != nil {
			p.Close()
		}
		if (err != nil) != tt.invalid {
			t.Fatalf("tuning %+v error %v", tt.tuning, err)
		}
	}
	baseline := NewPool()
	defer baseline.Close()
	tuned, err := NewPoolWithTuning(nil, Tuning{})
	if err != nil {
		t.Fatal(err)
	}
	defer tuned.Close()
	for _, key := range []string{"account_capacity", "identity_capacity", "capacity_limit", "idle_timeout_seconds", "connection_lifetime_seconds", "dial_rate_per_second", "dial_burst", "wait_limit_ms"} {
		if baseline.Stats()[key] != tuned.Stats()[key] {
			t.Fatalf("default drift: %s", key)
		}
	}
	_, err = TuningFromEnvironment(func(key string) (string, bool) { return "secret-value", key == "SMART_WS_COLD_WAIT_MS" })
	if err == nil || strings.Contains(err.Error(), "secret-value") {
		t.Fatal("invalid environment not safely rejected", err)
	}
	tuning, err := TuningFromEnvironment(func(key string) (string, bool) { return "160", key == "SMART_WS_IDENTITY_CAPACITY" })
	if err != nil || tuning.IdentityCapacity != 160 {
		t.Fatal(tuning, err)
	}
}

func admissionTestPool(t *testing.T, cold time.Duration) *Pool {
	c := defaultPoolConfig()
	c.coldWait = cold
	c.dialRate = 20
	c.dialBurst = 1
	p := newPool(c)
	t.Cleanup(p.Close)
	p.mu.Lock()
	p.dials["account"] = &dialState{refilled: time.Now(), used: time.Now()}
	p.mu.Unlock()
	return p
}
func finishTestPermit(p *Pool, permit *dialPermit, key string) {
	if permit == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.finishDialLocked(permit, "success", time.Now())
	p.finishOpeningLocked(key, "account")
}
func TestColdWaitUsesExistingDialBudget(t *testing.T) {
	for _, cold := range []time.Duration{0, 250 * time.Millisecond} {
		p := admissionTestPool(t, cold)
		_, permit, err := p.acquire(WithAccountID(context.Background(), 42), "identity", "capability", "account", true, newLease())
		if err != nil {
			t.Fatal(err)
		}
		if (permit != nil) != (cold > 0) {
			t.Fatalf("cold=%v permit=%v", cold, permit)
		}
		finishTestPermit(p, permit, "identity")
		snapshot := p.Stats()["accounts"].([]AccountSnapshot)[0]
		if !reflect.DeepEqual(snapshot.AccountIDs, []int64{42}) || snapshot.Counters["acquire_count"] != 1 {
			t.Fatal(snapshot)
		}
		if cold > 0 && snapshot.Counters["wait_started"] != 1 {
			t.Fatal(snapshot)
		}
		latencyClean(t, p)
	}
}
func TestColdWaitCancellationAndQueueLimit(t *testing.T) {
	p := admissionTestPool(t, 500*time.Millisecond)
	p.mu.Lock()
	p.dials["account"].inFlight = p.config.dialConcurrent
	p.mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, _, err := p.acquire(ctx, "identity", "capability", "account", true, newLease())
		result <- err
	}()
	deadline := time.After(time.Second)
	for p.Stats()["waiting"].(int) != 1 {
		select {
		case <-deadline:
			t.Fatal("not queued")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	latencyClean(t, p)
	p.mu.Lock()
	p.config.accountWaiters = 0
	p.mu.Unlock()
	_, permit, err := p.acquire(context.Background(), "identity", "capability", "account", true, newLease())
	if err != nil || permit != nil || p.Stats()["wait_queue_full"] != uint64(1) {
		t.Fatal(err, p.Stats())
	}
	latencyClean(t, p)
}
func TestColdWaitIsBoundedAndUnscopedRemainsImmediate(t *testing.T) {
	for _, reusable := range []bool{true, false} {
		p := admissionTestPool(t, 40*time.Millisecond)
		p.mu.Lock()
		p.dials["account"].inFlight = p.config.dialConcurrent
		p.mu.Unlock()
		started := time.Now()
		_, permit, err := p.acquire(context.Background(), "identity", "capability", "account", reusable, newLease())
		if err != nil || permit != nil || time.Since(started) > time.Second {
			t.Fatal("unbounded wait", err)
		}
		if reusable && p.Stats()["wait_timeout"] != uint64(1) {
			t.Fatal(p.Stats())
		}
		if !reusable && p.Stats()["wait_started"] != nil {
			t.Fatal("unscoped request queued")
		}
		latencyClean(t, p)
	}
}
func TestAccountSnapshotIsolationAndCopy(t *testing.T) {
	s, sockets, _, _ := successServer(t)
	p := testPool(t, 4)
	for _, id := range []int64{41, 41, 42} {
		req := request(t, s.URL, validBody, "session")
		if id == 42 {
			req.Header.Set("ChatGPT-Account-Id", "second-account")
		}
		req = req.WithContext(WithAccountID(req.Context(), id))
		consume(t, p, req, "", "trusted-scope")
	}
	rows := p.Stats()["accounts"].([]AccountSnapshot)
	if len(rows) != 2 || sockets.Load() != 2 {
		t.Fatal(rows, sockets.Load())
	}
	for _, row := range rows {
		if row.Connections != 1 || row.Idle != 1 || row.Active != 0 || row.Connecting != 0 {
			t.Fatal(row)
		}
		want := uint64(1)
		if row.AccountIDs[0] == 41 {
			want = 2
		}
		if row.Counters["completed"] != want || row.Counters["opened"] != 1 || row.Counters["reused"] != want-1 {
			t.Fatal(row)
		}
		row.Counters["completed"] = 99
		row.AccountIDs[0] = 999
	}
	for _, row := range p.Stats()["accounts"].([]AccountSnapshot) {
		if row.Counters["completed"] == 99 || row.AccountIDs[0] == 999 {
			t.Fatal("snapshot aliases live state")
		}
	}
}
