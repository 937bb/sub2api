package service

import (
	"encoding/json"
	"strings"

	"github.com/google/uuid"
)

func buildOpenAIPassthroughCompletedEvent(responseID string, usage *OpenAIUsage) []byte {
	responseID = strings.TrimSpace(responseID)
	if responseID == "" {
		responseID = "resp_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	}
	inputTokens := 0
	outputTokens := 0
	cacheReadTokens := 0
	if usage != nil {
		inputTokens = usage.InputTokens
		outputTokens = usage.OutputTokens
		cacheReadTokens = usage.CacheReadInputTokens
	}
	payload, _ := json.Marshal(map[string]any{
		"type":            "response.completed",
		"sequence_number": 0,
		"response": map[string]any{
			"id":     responseID,
			"object": "response",
			"status": "completed",
			"output": []any{},
			"error":  nil,
			"usage": map[string]any{
				"input_tokens":  inputTokens,
				"output_tokens": outputTokens,
				"total_tokens":  inputTokens + outputTokens,
				"input_tokens_details": map[string]any{
					"cached_tokens": cacheReadTokens,
				},
			},
		},
	})
	return payload
}
