package openai

import (
	_ "embed"
	"strings"
)

const fallbackCodexSyntheticDefaultInstructions = "You are a helpful coding assistant."

//go:embed instructions_gpt5_5.txt
var codexSyntheticInstructionsGPT55 string

// CodexSyntheticDefaultInstructionsForModel returns the server-side prompt used
// only when the Codex transform must synthesize an empty instructions field.
func CodexSyntheticDefaultInstructionsForModel(model string) string {
	normalized := strings.ToLower(strings.TrimSpace(model))
	if strings.Contains(normalized, "codex") {
		return firstNonEmptyInstructions(DefaultInstructions, fallbackCodexSyntheticDefaultInstructions)
	}
	return latestCodexSyntheticDefaultInstructions()
}

func latestCodexSyntheticDefaultInstructions() string {
	return firstNonEmptyInstructions(
		codexSyntheticInstructionsGPT55,
		DefaultInstructions,
		fallbackCodexSyntheticDefaultInstructions,
	)
}

func firstNonEmptyInstructions(candidates ...string) string {
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate) != "" {
			return candidate
		}
	}
	return fallbackCodexSyntheticDefaultInstructions
}
