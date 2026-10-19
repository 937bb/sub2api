package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDefaultDistillationMonitorConfigIsDisabled(t *testing.T) {
	cfg := defaultDistillationMonitorConfig()

	require.False(t, cfg.Enabled)
	require.Equal(t, DistillationMonitorModeObserve, cfg.Mode)
	require.Empty(t, cfg.GroupIDs)
}

func TestScoreDistillationCandidateNonStreamAloneCannotDisable(t *testing.T) {
	cfg := defaultDistillationMonitorConfig()
	now := time.Now()
	candidate := DistillationCandidate{
		RequestCount:       cfg.MinRequests,
		InputTokens:        300_000,
		OutputTokens:       cfg.MinOutputTokens,
		CacheReadTokens:    300_000,
		NonStreamCount:     cfg.MinRequests,
		DistinctSessions:   1,
		DistinctAPIKeys:    1,
		DistinctIPs:        1,
		DistinctUserAgents: 1,
		FirstRequestAt:     now,
		LastRequestAt:      now.Add(time.Minute),
	}

	score := scoreDistillationCandidate(candidate, nil, cfg, time.Duration(cfg.WindowMinutes)*time.Minute)

	require.Less(t, score.Total, cfg.RiskThreshold)
	require.Less(t, score.EvidenceClasses, cfg.MinEvidenceClasses)
	require.Equal(t, 8, score.Automation-3)
}

func TestScoreDistillationCandidateMultipleIndependentSignalsReachesRiskThreshold(t *testing.T) {
	cfg := defaultDistillationMonitorConfig()
	now := time.Now()
	candidate := DistillationCandidate{
		RequestCount:       cfg.MinRequests * 4,
		InputTokens:        1_500_000,
		OutputTokens:       cfg.MinOutputTokens * 3,
		CacheReadTokens:    0,
		NonStreamCount:     cfg.MinRequests * 4,
		MissingSessions:    cfg.MinRequests * 4,
		DistinctAPIKeys:    4,
		DistinctIPs:        3,
		DistinctUserAgents: 3,
		ExplicitSignals:    cfg.MinRequests,
		FirstRequestAt:     now,
		LastRequestAt:      now.Add(14 * time.Minute),
	}
	hashes := make([]int64, 64)
	for index := range hashes {
		hashes[index] = 42
	}

	score := scoreDistillationCandidate(candidate, hashes, cfg, 15*time.Minute)

	require.GreaterOrEqual(t, score.Total, cfg.RiskThreshold)
	require.GreaterOrEqual(t, score.EvidenceClasses, cfg.MinEvidenceClasses)
}

func TestBuildDistillationRequestFeatureStoresOnlyDerivedMetadata(t *testing.T) {
	secretPrompt := "private prompt used to generate a synthetic dataset"
	feature := buildDistillationRequestFeature(ContentModerationCheckInput{
		RequestID: "req-1",
		UserID:    7,
		APIKeyID:  8,
		GroupID:   int64Pointer(9),
		Model:     "gpt-test",
		Protocol:  ContentModerationProtocolOpenAIResponses,
		Body:      []byte(`{"input":[{"role":"user","content":"` + secretPrompt + `"}]}`),
	})

	require.NotZero(t, feature.PromptSimHash)
	require.Positive(t, feature.PromptLength)
	require.True(t, feature.ExplicitSignal)
	require.NotContains(t, feature.Model, secretPrompt)
}

func TestDisableDistillationUserSkipsAdmin(t *testing.T) {
	userRepo := &contentModerationTestUserRepo{user: &User{ID: 11, Role: RoleAdmin, Status: StatusActive}}
	svc := &ContentModerationService{userRepo: userRepo}

	disabled := svc.disableDistillationUser(context.Background(), 11, 100, 22)

	require.False(t, disabled)
	require.Equal(t, StatusActive, userRepo.user.Status)
	require.Empty(t, userRepo.updated)
}

func int64Pointer(value int64) *int64 {
	return &value
}
