package service

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
)

// Lookup precedes inference; only completed responses commit checkpoints. Two
// concurrent cold requests cannot manufacture a read from each other's writes.
func (s *GatewayService) kiroPrefixFinalizer(ctx context.Context, groupID int64, scope string, raw []byte, model string, policy KiroCachePolicy) func(*kiro.Response) {
	cache, ok := s.cache.(KiroPrefixCache)
	if !ok || !policy.PrefixEnabled() {
		return nil
	}
	plan := kiro.PlanPrefix(raw, model, policy.AutoPrefix, policy.TTLSeconds)
	if plan == nil {
		return nil
	}
	matched, err := cache.LookupKiroPrefix(ctx, groupID, scope, plan, policy)
	if err != nil {
		return nil
	}
	return func(out *kiro.Response) {
		// Like kiro.rs, authoritative provider usage always wins. Failure,
		// cancellation and refusal never establish billable local checkpoints.
		if ctx.Err() != nil || out == nil || out.UsageSource != "estimated" || len(out.Content) == 0 {
			return
		}
		switch out.StopReason {
		case "end_turn", "tool_use", "max_tokens", "stop_sequence":
		default:
			return
		}
		if err := cache.CommitKiroPrefix(ctx, groupID, scope, plan, matched, policy); err != nil {
			return
		}
		read := 0
		if matched != nil {
			read = matched.Tokens
		}
		input, write5m, write1h, read := plan.Split(kiroCachePromptTokens(out.Usage), read, policy.TargetPercent)
		out.Usage.InputTokens = input
		out.Usage.CacheCreationInputTokens = write5m + write1h
		out.Usage.CacheReadInputTokens = read
		out.Usage.CacheCreation = &kiro.CacheCreationUsage{Ephemeral5mInputTokens: write5m, Ephemeral1hInputTokens: write1h}
		out.Usage.CacheSource = "local_prefix"
		out.UsageSource = "local_prefix_estimated"
	}
}
