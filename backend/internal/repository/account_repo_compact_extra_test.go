package repository

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestShouldEnqueueSchedulerOutboxForExtraUpdates_CompactCapabilityKeysAreRelevant(t *testing.T) {
	updates := map[string]any{
		"openai_compact_supported":  true,
		"openai_compact_checked_at": "2026-04-10T10:00:00Z",
	}

	if !shouldEnqueueSchedulerOutboxForExtraUpdates(updates) {
		t.Fatalf("expected compact capability updates to enqueue scheduler outbox")
	}
}

func TestShouldEnqueueSchedulerOutboxForExtraUpdates_OpenAIResponsesCapabilityKeysAreRelevant(t *testing.T) {
	updates := map[string]any{
		"openai_responses_mode":      "force_chat_completions",
		"openai_responses_supported": false,
	}

	if !shouldEnqueueSchedulerOutboxForExtraUpdates(updates) {
		t.Fatalf("expected responses capability updates to enqueue scheduler outbox")
	}
}

func TestShouldEnqueueSchedulerOutboxForExtraUpdates_OpenAICodexFingerprintIsNeutral(t *testing.T) {
	updates := map[string]any{
		service.OpenAICodexFingerprintExtraKey: map[string]any{"present": true},
	}

	if shouldEnqueueSchedulerOutboxForExtraUpdates(updates) {
		t.Fatalf("expected codex fingerprint update to avoid scheduler outbox")
	}
}

func TestShouldEnqueueSchedulerOutboxForExtraUpdates_ModelRateLimitsAreNeutral(t *testing.T) {
	updates := map[string]any{
		"model_rate_limits": map[string]any{"gpt-5": map[string]any{"rate_limit_reset_at": "2026-07-17T01:00:00Z"}},
	}

	if shouldEnqueueSchedulerOutboxForExtraUpdates(updates) {
		t.Fatalf("expected model rate limit update to avoid scheduler outbox")
	}
}

func TestShouldEnqueueSchedulerOutboxForExtraUpdates_KnownUsageKeysAreNeutral(t *testing.T) {
	updates := map[string]any{
		"codex_primary_used_percent":   12.5,
		"codex_7d_reset_at":            "2026-07-18T01:00:00Z",
		"passive_usage_sampled_at":     "2026-07-17T01:00:00Z",
		"passive_usage_7d_utilization": 0.42,
	}

	if shouldEnqueueSchedulerOutboxForExtraUpdates(updates) {
		t.Fatalf("expected known usage updates to avoid scheduler outbox")
	}
}

func TestShouldEnqueueSchedulerOutboxForExtraUpdates_UnknownUsagePrefixedKeysAreRelevant(t *testing.T) {
	for _, key := range []string{
		"codex_primary_future_policy",
		"codex_5h_future_policy",
		"passive_usage_future_policy",
	} {
		if !shouldEnqueueSchedulerOutboxForExtraUpdates(map[string]any{key: true}) {
			t.Fatalf("expected unknown prefixed key %q to enqueue scheduler outbox", key)
		}
	}
}
