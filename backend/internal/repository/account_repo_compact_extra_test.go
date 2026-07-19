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

func TestSchedulerNeutralExtraKeysMatchMigrationProjection(t *testing.T) {
	if got := len(schedulerNeutralExtraKeys); got != 24 {
		t.Fatalf("schedulerNeutralExtraKeys length = %d, want 24", got)
	}
	for key := range schedulerNeutralExtraKeys {
		if !isSchedulerNeutralExtraKey(key) {
			t.Fatalf("expected allowlisted key %q to be scheduler-neutral", key)
		}
		if shouldEnqueueSchedulerOutboxForExtraUpdates(map[string]any{key: true}) {
			t.Fatalf("expected allowlisted key %q to avoid lifecycle outbox", key)
		}
		if !shouldSyncSchedulerSnapshotForExtraUpdates(map[string]any{key: true}) {
			t.Fatalf("expected allowlisted key %q to publish runtime snapshot", key)
		}
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
		" codex_5h_used_percent",
		"codex_5h_used_percent ",
	} {
		if !shouldEnqueueSchedulerOutboxForExtraUpdates(map[string]any{key: true}) {
			t.Fatalf("expected unknown key %q to enqueue scheduler outbox", key)
		}
	}
}

func TestExtraUpdateSchedulerEffectsAreIndependent(t *testing.T) {
	tests := []struct {
		name         string
		updates      map[string]any
		wantOutbox   bool
		wantSnapshot bool
	}{
		{
			name:         "lifecycle only",
			updates:      map[string]any{"openai_compact_supported": true},
			wantOutbox:   true,
			wantSnapshot: false,
		},
		{
			name:         "runtime only",
			updates:      map[string]any{"codex_5h_used_percent": 42.5},
			wantOutbox:   false,
			wantSnapshot: true,
		},
		{
			name: "mixed compact probe",
			updates: map[string]any{
				"openai_compact_supported": true,
				"codex_5h_used_percent":    42.5,
			},
			wantOutbox:   true,
			wantSnapshot: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldEnqueueSchedulerOutboxForExtraUpdates(tt.updates); got != tt.wantOutbox {
				t.Fatalf("shouldEnqueueSchedulerOutboxForExtraUpdates() = %v, want %v", got, tt.wantOutbox)
			}
			if got := shouldSyncSchedulerSnapshotForExtraUpdates(tt.updates); got != tt.wantSnapshot {
				t.Fatalf("shouldSyncSchedulerSnapshotForExtraUpdates() = %v, want %v", got, tt.wantSnapshot)
			}
		})
	}
}
