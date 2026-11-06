package kiro

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInteractiveClaudeThinkingDisplay(t *testing.T) {
	for _, display := range []string{"updates", "summarized", "omitted"} {
		t.Run(display, func(t *testing.T) {
			raw := `{"max_tokens":4096,"thinking":{"type":"adaptive","display":"` + display + `"},"messages":[{"role":"user","content":"hi"}]}`
			body, parsed, err := Convert([]byte(raw), "claude-opus-5-5", "session", map[string]any{"kiro_request_fields": map[string]any{"thinking": map[string]any{}}})
			require.NoError(t, err)
			var out struct {
				Fields struct {
					Thinking json.RawMessage `json:"thinking"`
				} `json:"additionalModelRequestFields"`
			}
			require.NoError(t, json.Unmarshal(body, &out))
			want := display
			if want == "updates" {
				want = "summarized"
			}
			require.JSONEq(t, `{"type":"adaptive","display":"`+want+`"}`, string(out.Fields.Thinking))
			require.JSONEq(t, `{"type":"adaptive","display":"`+display+`"}`, string(parsed.Thinking))
		})
	}
}

func TestThinkingDisplayPreservesOtherControls(t *testing.T) {
	for _, raw := range []string{`{"type":"adaptive"}`, `{"type":"enabled","budget_tokens":1024}`, `{"type":"disabled"}`, `{"type":"adaptive","display":"unknown"}`} {
		out, err := normalizeThinkingDisplay(json.RawMessage(raw))
		require.NoError(t, err)
		require.JSONEq(t, raw, string(out))
	}
	for _, raw := range []string{`[]`, `"adaptive"`, `null`, `true`} {
		_, err := normalizeThinkingDisplay(json.RawMessage(raw))
		require.Error(t, err)
	}
}

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
	raw := `{"max_tokens":1024,"system":[{"type":"text","text":"Top-level instructions"}],"output_config":{"effort":"low","format":{"type":"json_schema"}},"messages":[
		{"role":"system","content":"First instruction"},
		{"role":"user","content":"hi"},
		{"role":"system","content":[{"type":"text","text":"Second instruction"}],"output_config":{"effort":"high"}},
		{"role":"system","content":[],"output_config":{"effort":"max"}}
	]}`
	body, req, err := Convert([]byte(raw), "claude-opus-5-5", "session", map[string]any{"kiro_request_fields": map[string]any{"output_config": map[string]any{}}})
	require.NoError(t, err)
	require.Len(t, req.Messages, 1)
	require.Equal(t, "user", req.Messages[0].Role)
	require.JSONEq(t, `{"effort":"max","format":{"type":"json_schema"}}`, string(req.OutputConfig))
	var out struct {
		State struct {
			Current struct {
				User struct {
					Content string `json:"content"`
				} `json:"userInputMessage"`
			} `json:"currentMessage"`
			History []any `json:"history"`
		} `json:"conversationState"`
	}
	require.NoError(t, json.Unmarshal(body, &out))
	require.Equal(t, "Top-level instructions\n\nFirst instruction\n\nSecond instruction\n\nhi", out.State.Current.User.Content)
	require.Empty(t, out.State.History)
}

func TestEmptySystemMessagesAreNoOps(t *testing.T) {
	for _, message := range []string{
		`{"role":"system"}`, `{"role":"system","content":null}`, `{"role":"system","content":""}`,
		`{"role":"system","content":[]}`, `{"role":"system","content":[{"type":"text","text":""}]}`,
		`{"role":"system","content":[],"output_config":{}}`, `{"role":"system","content":[],"output_config":null}`,
	} {
		t.Run(message, func(t *testing.T) {
			raw := `{"max_tokens":1024,"system":[{"type":"text","text":"Retain me","cache_control":{"type":"ephemeral"}}],"messages":[{"role":"user","content":"hi"},` + message + `]}`
			_, req, err := Convert([]byte(raw), "claude-haiku-4.5", "session", nil)
			require.NoError(t, err)
			require.Len(t, req.Messages, 1)
			require.Empty(t, req.OutputConfig)
			require.JSONEq(t, `[{"type":"text","text":"Retain me","cache_control":{"type":"ephemeral"}}]`, string(req.System))
		})
	}
}

func TestSystemNormalizationPreservesToolHistory(t *testing.T) {
	raw := `{"max_tokens":1024,"messages":[
		{"role":"user","content":"Read the marker"},
		{"role":"assistant","content":[{"type":"tool_use","id":"tool_1","name":"Read","input":{"path":"marker.txt"}}]},
		{"role":"system","content":"Return the marker exactly"},
		{"role":"user","content":[{"type":"tool_result","tool_use_id":"tool_1","content":"MARKER731"}]},
		{"role":"system","content":[]}
	],"tools":[{"name":"Read","input_schema":{"type":"object"}}]}`
	body, req, err := Convert([]byte(raw), "claude-haiku-4.5", "session", nil)
	require.NoError(t, err)
	require.Len(t, req.Messages, 3)
	var out map[string]any
	require.NoError(t, json.Unmarshal(body, &out))
	state := out["conversationState"].(map[string]any)
	history := state["history"].([]any)
	require.Len(t, history, 2)
	require.Equal(t, "Return the marker exactly\n\nRead the marker", history[0].(map[string]any)["userInputMessage"].(map[string]any)["content"])
	require.Contains(t, string(body), `"toolUseId":"tool_1"`)
	require.Contains(t, string(body), "MARKER731")
}

func TestControlMessagesRejectUnsupportedContent(t *testing.T) {
	for _, raw := range []string{
		`{"max_tokens":1024,"messages":[{"role":"system","content":[{"type":"image"}]},{"role":"user","content":"hi"}]}`,
		`{"max_tokens":1024,"messages":[{"role":"system","content":{},"output_config":{"effort":"high"}},{"role":"user","content":"hi"}]}`,
		`{"max_tokens":1024,"messages":[{"role":"system","content":[],"output_config":[]},{"role":"user","content":"hi"}]}`,
		`{"max_tokens":1024,"messages":[{"role":"system","content":[],"output_config":"high"},{"role":"user","content":"hi"}]}`,
		`{"max_tokens":1024,"messages":[{"role":"system","content":[],"output_config":{"effort":"high"}}]}`,
		`{"max_tokens":1024,"messages":[{"role":"system","content":"Instructions without a user"}]}`,
	} {
		_, _, err := Convert([]byte(raw), "claude-opus-5-5", "session", nil)
		require.Error(t, err)
	}
}
