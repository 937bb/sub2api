package smartws

import (
	"fmt"
	"sort"
	"testing"
	"time"
)

func TestDialBalanceEnvelope(t *testing.T) {
	for _, rate := range []float64{0.8, 0.85, 0.9, 0.95, 1.0, 1.2} {
		t.Run(fmt.Sprint(rate), func(t *testing.T) {
			p := newPool(poolConfig{dialRate: rate, dialBurst: 2, dialConcurrent: 2})
			defer p.Close()
			start := time.Now()
			accepted := []time.Time{}
			rejected := 0
			for i := 0; i < 180000; i++ {
				now := start.Add(time.Duration(i) * time.Millisecond)
				permit, reason := p.admitDialLocked("same-account", now)
				if permit == nil {
					if reason != "dial_rate_limited" {
						t.Fatal(reason)
					}
					rejected++
					continue
				}
				accepted = append(accepted, now)
				p.finishDialLocked(permit, "success", now)
			}
			maxWindow := 0
			left := 0
			for right, at := range accepted {
				for !accepted[left].After(at.Add(-time.Minute)) {
					left++
				}
				if n := right - left + 1; n > maxWindow {
					maxWindow = n
				}
			}
			if rate <= 0.9 && maxWindow > 56 {
				t.Fatalf("safe candidate consumed excessive minute budget: %d", maxWindow)
			}
			if rate >= 1 && maxWindow < 60 {
				t.Fatal("hazard scenario was not saturated")
			}
			t.Logf("SATURATED rate=%.2f accepted=%d rejected=%d max_rolling_60s=%d", rate, len(accepted), rejected, maxWindow)
		})
	}
}
func TestDialBalanceBurstTrace(t *testing.T) {
	for _, rate := range []float64{0.8, 0.85, 0.9, 0.95} {
		p := newPool(poolConfig{dialRate: rate, dialBurst: 2, dialConcurrent: 2})
		start := time.Now()
		accepted, rejected := 0, 0
		duration := []float64{}
		// Deterministic arrivals at 6-second bursts of 6 independent cold identities.
		for i := 0; i < 600; i++ {
			now := start.Add(time.Duration(i/6)*6*time.Second + time.Duration(i%6)*120*time.Millisecond)
			tick := time.Now()
			permit, _ := p.admitDialLocked("stable-account", now)
			duration = append(duration, time.Since(tick).Seconds())
			if permit == nil {
				rejected++
			} else {
				accepted++
				p.finishDialLocked(permit, "success", now)
			}
		}
		sort.Float64s(duration)
		t.Logf("BURST rate=%.2f arrivals=600 admitted=%d fallback=%d", rate, accepted, rejected)
		p.Close()
	}
}
