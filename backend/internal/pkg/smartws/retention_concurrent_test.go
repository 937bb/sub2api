package smartws

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
	"testing"
	"time"
)

func TestRetentionConcurrentHotReuseAtFullPool(t *testing.T) {
	for _, limit := range []int{2048, 4096} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			perAccount := 128
			if limit == 4096 {
				perAccount = 256
			}
			s, opened, received := retentionServer(t)
			p := retentionPool(t, perAccount, limit, 300*time.Second)
			makeReq := func(i int) *http.Request {
				r := request(t, s.URL, validBody, fmt.Sprintf("hot-%d", i))
				r.Header.Set("ChatGPT-Account-Id", fmt.Sprint(i/perAccount))
				return r
			}
			for i := 0; i < limit; i++ {
				consume(t, p, makeReq(i), "", "scope")
			}
			sockets := opened.Load()
			before := received.Load()
			var wg sync.WaitGroup
			durations := make(chan time.Duration, 4096)
			errs := make(chan error, 32)
			start := time.Now()
			for worker := 0; worker < 32; worker++ {
				wg.Add(1)
				go func(w int) {
					defer wg.Done()
					for turn := 0; turn < 128; turn++ {
						i := w*(limit/32) + turn%(limit/32)
						tick := time.Now()
						resp, h, e := p.Try(makeReq(i), "", "scope")
						durations <- time.Since(tick)
						if e != nil || !h || resp == nil {
							errs <- fmt.Errorf("request %d admission %t %v", i, h, e)
							return
						}
						_, e = io.Copy(io.Discard, resp.Body)
						resp.Body.Close()
						if e != nil {
							errs <- e
							return
						}
					}
				}(worker)
			}
			wg.Wait()
			close(errs)
			close(durations)
			for e := range errs {
				t.Fatal(e)
			}
			d := []float64{}
			for elapsed := range durations {
				d = append(d, float64(elapsed)/float64(time.Millisecond))
			}
			sort.Float64s(d)
			st := p.Stats()
			if opened.Load() != sockets || received.Load()-before != 4096 || st["connections"] != limit || st["active"] != 0 {
				t.Fatal("hot reuse opened extra sockets or leaked", st)
			}
			t.Logf("HOT_REUSE full_pool=%d requests=4096 concurrent=32 seconds=%.3f acquire_p50_ms=%.3f p95_ms=%.3f max_ms=%.3f extra_handshakes=%d", limit, time.Since(start).Seconds(), d[len(d)/2], d[len(d)*95/100], d[len(d)-1], opened.Load()-sockets)
			p.Close()
		})
	}
}
