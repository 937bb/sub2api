package service

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

const (
	apiKeyDashboardTTL         = 15 * time.Second
	apiKeyDashboardLoadTimeout = 12 * time.Second
	apiKeyDashboardMaxLoads    = 2
	apiKeyDashboardMaxEntries  = 1024
)

var errDashboardBusy = errors.New("usage statistics refresh capacity exhausted")

type dashboardEntry struct {
	stats   *usagestats.UserDashboardStats
	expires time.Time
	day     int64
}

type dashboardLoad struct {
	done  chan struct{}
	stats *usagestats.UserDashboardStats
	err   error
}

// Only historical display statistics are cached. Authoritative balances,
// quotas, authentication and billing never read this cache.
type apiKeyDashboardCache struct {
	mu      sync.Mutex
	entries map[int64]dashboardEntry
	loads   map[int64]*dashboardLoad
}

func cloneDashboardStats(stats *usagestats.UserDashboardStats) *usagestats.UserDashboardStats {
	if stats == nil {
		return nil
	}
	copy := *stats
	copy.ByPlatform = append([]usagestats.PlatformDashboardStats(nil), stats.ByPlatform...)
	return &copy
}

func (c *apiKeyDashboardCache) get(ctx context.Context, id int64, fetch func(context.Context, int64) (*usagestats.UserDashboardStats, error)) (*usagestats.UserDashboardStats, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	now, day := time.Now(), timezone.Today().Unix()
	c.mu.Lock()
	if entry, ok := c.entries[id]; ok && now.Before(entry.expires) && entry.day == day {
		stats := cloneDashboardStats(entry.stats)
		c.mu.Unlock()
		return stats, nil
	}
	load := c.loads[id]
	if load == nil {
		// Do not queue arbitrary keys behind expensive history scans. Optional
		// statistics may be absent while fresh balance/quota remains available.
		if len(c.loads) >= apiKeyDashboardMaxLoads {
			c.mu.Unlock()
			return nil, errDashboardBusy
		}
		if c.loads == nil {
			c.loads = make(map[int64]*dashboardLoad)
		}
		load = &dashboardLoad{done: make(chan struct{})}
		c.loads[id] = load
		// A short-lived caller must not cancel the refresh shared by other
		// callers. Detached work has its own deadline and a hard concurrency cap.
		go c.refresh(context.WithoutCancel(ctx), id, day, load, fetch)
	}
	c.mu.Unlock()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-load.done:
		return cloneDashboardStats(load.stats), load.err
	}
}

func (c *apiKeyDashboardCache) refresh(parent context.Context, id, day int64, load *dashboardLoad, fetch func(context.Context, int64) (*usagestats.UserDashboardStats, error)) {
	ctx, cancel := context.WithTimeout(parent, apiKeyDashboardLoadTimeout)
	defer cancel()
	stats, err := fetch(ctx, id)
	if err == nil {
		err = ctx.Err()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err == nil && stats != nil {
		if c.entries == nil {
			c.entries = make(map[int64]dashboardEntry)
		}
		now := time.Now()
		for key, entry := range c.entries {
			if !now.Before(entry.expires) || entry.day != day {
				delete(c.entries, key)
			}
		}
		if _, exists := c.entries[id]; !exists && len(c.entries) >= apiKeyDashboardMaxEntries {
			var oldestID int64
			var oldest time.Time
			for key, entry := range c.entries {
				if oldest.IsZero() || entry.expires.Before(oldest) {
					oldestID, oldest = key, entry.expires
				}
			}
			delete(c.entries, oldestID)
		}
		c.entries[id] = dashboardEntry{stats: cloneDashboardStats(stats), expires: now.Add(apiKeyDashboardTTL), day: day}
	}
	load.stats, load.err = cloneDashboardStats(stats), err
	delete(c.loads, id)
	close(load.done)
}
