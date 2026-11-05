package service

import (
	"context"
	"encoding/json"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
)

func kiroCachePromptTokens(usage kiro.Usage) int {
	return max(usage.InputTokens, 0) + max(usage.CacheCreationInputTokens, 0) + max(usage.CacheReadInputTokens, 0)
}

// Local response-cache operations use Sub's existing cache pricing buckets.
// These are gateway cache units, not claims about upstream prompt caching.
func kiroCacheRead(out *kiro.Response) {
	out.Usage = kiro.Usage{CacheReadInputTokens: kiroCachePromptTokens(out.Usage), CacheSource: "local"}
	out.UsageSource = "local_cache"
}

func kiroCacheFinalizer(ctx context.Context, cache KiroResponseCache, groupID int64, key string, policy KiroCachePolicy, enabled bool) func(*kiro.Response) {
	if !enabled {
		return nil
	}
	return func(out *kiro.Response) {
		if out.StopReason != "end_turn" || len(out.Content) == 0 {
			return
		}
		for _, block := range out.Content {
			if block.Type != "text" {
				return
			}
		}
		data, err := json.Marshal(out)
		if err != nil || len(data) > 1<<20 {
			return
		}
		if err := cache.PutKiroResponse(ctx, groupID, key, data, policy); err != nil {
			return
		}
		prompt := kiroCachePromptTokens(out.Usage)
		breakdown := &kiro.CacheCreationUsage{}
		if policy.TTLSeconds > 300 {
			breakdown.Ephemeral1hInputTokens = prompt
		} else {
			breakdown.Ephemeral5mInputTokens = prompt
		}
		out.Usage = kiro.Usage{OutputTokens: out.Usage.OutputTokens, CacheCreationInputTokens: prompt, CacheCreation: breakdown, CacheSource: "local"}
		out.UsageSource = "local_cache_create"
	}
}
