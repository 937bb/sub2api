package kiro

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClaudeCodeControlsAndCatalogLimits(t *testing.T) {
	for _, limit := range []struct{ requested, want int }{{1, 1024}, {1024, 1024}, {64000, 64000}, {128000, 64000}} {
		raw, _ := json.Marshal(map[string]any{"max_tokens": limit.requested, "output_config": map[string]any{"effort": "high"}, "messages": []any{
			map[string]any{"role": "user", "content": "hello"},
			map[string]any{"role": "system", "content": []any{}, "output_config": map[string]any{"effort": "max"}},
		}})
		body, req, err := Convert(raw, "claude-opus-5-5", "session", map[string]any{"kiro_request_fields": map[string]any{"max_tokens": map[string]any{"minimum": 1024, "maximum": 64000}, "output_config": map[string]any{}}})
		require.NoError(t, err)
		require.Len(t, req.Messages, 1)
		var out struct {
			Fields struct {
				MaxTokens int `json:"max_tokens"`
				Output    struct {
					Effort string `json:"effort"`
				} `json:"output_config"`
			} `json:"additionalModelRequestFields"`
		}
		require.NoError(t, json.Unmarshal(body, &out))
		require.Equal(t, limit.want, out.Fields.MaxTokens)
		require.Equal(t, "max", out.Fields.Output.Effort)
	}
}

func TestControlMessagesNeverDiscardInstructions(t *testing.T) {
	for _, raw := range []string{
		`{"max_tokens":1024,"messages":[{"role":"system","content":"important instructions","output_config":{"effort":"high"}},{"role":"user","content":"hi"}]}`,
		`{"max_tokens":1024,"messages":[{"role":"system","content":[]},{"role":"user","content":"hi"}]}`,
		`{"max_tokens":1024,"messages":[{"role":"system","content":[],"output_config":{"effort":"high"}}]}`,
	} {
		_, _, err := Convert([]byte(raw), "claude-opus-5-5", "session", nil)
		require.Error(t, err)
	}
}
