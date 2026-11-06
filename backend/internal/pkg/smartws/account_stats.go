package smartws

import (
	"context"
	"sort"
	"time"
)

type localAccountIDKey struct{}

// WithAccountID adds an internal database identifier for diagnostics only.
// It does not affect affinity, admission, upstream headers or credentials.
func WithAccountID(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, localAccountIDKey{}, id)
}

// AccountSnapshot is process-local and retained with the bounded account state.
// Admission buckets are cumulative counts; they do not measure model latency.
type AccountSnapshot struct {
	AccountHash  string            `json:"account_hash"`
	AccountIDs   []int64           `json:"account_ids"`
	Connections  int               `json:"connections"`
	Active       int               `json:"active"`
	Idle         int               `json:"idle"`
	Connecting   int               `json:"connecting"`
	Waiting      int               `json:"waiting"`
	WaitingBytes int               `json:"waiting_bytes"`
	Counters     map[string]uint64 `json:"counters"`
}

func (p *Pool) accountCount(key, name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.accountCountLocked(key, name)
}

func (p *Pool) accountCountLocked(key, name string) {
	if st := p.accounts[key]; st != nil {
		if st.counters == nil {
			st.counters = make(map[string]uint64)
		}
		st.counters[name]++
	}
}
func (p *Pool) observeAdmission(key string, started time.Time) {
	elapsed := time.Since(started)
	p.mu.Lock()
	defer p.mu.Unlock()
	st := p.accounts[key]
	if st == nil {
		return
	}
	p.accountCountLocked(key, "acquire_count")
	st.counters["acquire_total_us"] += uint64(elapsed.Microseconds())
	for _, bucket := range []struct {
		name  string
		limit time.Duration
	}{
		{"acquire_le_10ms", 10 * time.Millisecond},
		{"acquire_le_50ms", 50 * time.Millisecond},
		{"acquire_le_100ms", 100 * time.Millisecond},
		{"acquire_le_250ms", 250 * time.Millisecond},
		{"acquire_le_500ms", 500 * time.Millisecond},
		{"acquire_le_2500ms", 2500 * time.Millisecond},
	} {
		if elapsed <= bucket.limit {
			st.counters[bucket.name]++
		}
	}
}
func (p *Pool) accountSnapshotsLocked() []AccountSnapshot {
	snapshots := make(map[string]*AccountSnapshot, len(p.accounts))
	for key, st := range p.accounts {
		row := &AccountSnapshot{AccountHash: key, AccountIDs: []int64{}, Connecting: st.opening, Waiting: len(st.waiters), Counters: make(map[string]uint64, len(st.counters))}
		for id := range st.accountIDs {
			row.AccountIDs = append(row.AccountIDs, id)
		}
		sort.Slice(row.AccountIDs, func(i, j int) bool { return row.AccountIDs[i] < row.AccountIDs[j] })
		for name, n := range st.counters {
			row.Counters[name] = n
		}
		for _, waiter := range st.waiters {
			row.WaitingBytes += waiter.bytes
		}
		snapshots[key] = row
	}
	for c := range p.connections {
		if row := snapshots[c.accountKey]; row != nil {
			row.Connections++
			if c.active != nil {
				row.Active++
			} else {
				row.Idle++
			}
		}
	}
	result := make([]AccountSnapshot, 0, len(snapshots))
	for _, row := range snapshots {
		result = append(result, *row)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].AccountHash < result[j].AccountHash })
	return result
}
