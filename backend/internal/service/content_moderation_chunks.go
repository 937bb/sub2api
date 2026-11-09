package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// Overlap keeps short phrases crossing a chunk boundary visible to the auditor.
const moderationChunkOverlapRunes = 256

// A request is allowed only after all chunks pass. Sequential calls bound load;
// a total deadline prevents very large inputs from occupying the gate indefinitely.
func (s *ContentModerationService) callModerationChunks(ctx context.Context, cfg *ContentModerationConfig, content ContentModerationInput, trackLoad bool) (*moderationAPIResult, string, error) {
	ctx, cancel := context.WithTimeout(ctx, maxContentModerationTimeoutMS*time.Millisecond)
	defer cancel()
	runes := []rune(content.Text)
	merged := &moderationAPIResult{CategoryScores: make(map[string]float64)}
	evidence := content.Text
	chunks := 0
	for start := 0; ; {
		end := min(start+maxModerationInputRunes, len(runes))
		chunk := ContentModerationInput{Text: string(runes[start:end])}
		if start == 0 {
			chunk.Images = content.Images
		}
		chunks++
		result, err := s.callModeration(ctx, cfg, chunk.ModerationInput(), trackLoad)
		if err != nil {
			return nil, chunk.Text, fmt.Errorf("moderation chunk %d: %w", chunks, err)
		}
		_, _, previousScore := evaluateModerationScores(merged.CategoryScores, cfg.Thresholds)
		_, _, score := evaluateModerationScores(result.CategoryScores, cfg.Thresholds)
		if chunks == 1 || score > previousScore {
			evidence, merged.EngineMeta = chunk.Text, result.EngineMeta
		}
		merged.Flagged = merged.Flagged || result.Flagged
		for category, score := range result.CategoryScores {
			if old, ok := merged.CategoryScores[category]; !ok || score > old {
				merged.CategoryScores[category] = score
			}
		}
		if flagged, _, _ := evaluateModerationScores(result.CategoryScores, cfg.Thresholds); flagged {
			// A configured threshold already denies this input; no later chunk can allow it.
			evidence, merged.EngineMeta = chunk.Text, result.EngineMeta
			break
		}
		if end == len(runes) {
			break
		}
		start = end - moderationChunkOverlapRunes
	}
	slog.Debug("content_moderation.chunks_complete", "chunks", chunks, "input_runes", len(runes))
	return merged, evidence, nil
}
