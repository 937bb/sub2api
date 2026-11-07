package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type startupSlotCache struct {
	ConcurrencyCache
	slots map[string]bool
	calls int
}

func (c *startupSlotCache) CleanupStaleProcessSlots(_ context.Context, prefix string) error {
	c.calls++
	for owner := range c.slots {
		if owner != prefix {
			delete(c.slots, owner)
		}
	}
	return nil
}

func TestProvideConcurrencyServicePreservesRollingReleaseSlots(t *testing.T) {
	for _, skip := range []bool{false, true} {
		t.Run(map[bool]string{false: "single_process_default", true: "shared_redis_rolling"}[skip], func(t *testing.T) {
			cache := &startupSlotCache{slots: map[string]bool{"old-runtime": true}}
			cfg := &config.Config{}
			cfg.Gateway.Scheduling.SkipStartupSlotCleanup = skip
			svc := ProvideConcurrencyService(cache, nil, cfg)
			require.NotNil(t, svc)
			if skip {
				require.Zero(t, cache.calls)
				require.True(t, cache.slots["old-runtime"])
			} else {
				require.Equal(t, 1, cache.calls)
				require.Empty(t, cache.slots)
			}
		})
	}
}
