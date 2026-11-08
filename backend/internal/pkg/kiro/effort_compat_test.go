package kiro

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestModelSwitchEffortUsesSelectedCatalog(t *testing.T) {
	for _, model := range []string{"claude-opus-4.6", "claude-opus-4-6", "claude-opus-5.5"} {
		t.Run(model, func(t *testing.T) {
			allowed := []string{"low", "medium", "high", "max"}
			want := "high"
			if model == "claude-opus-5.5" {
				allowed = append(allowed, "xhigh")
				want = "xhigh"
			}
			credentials := map[string]any{"endpoint": "cli", "kiro_request_fields": map[string]any{
				"thinking":      map[string]any{},
				"output_config": map[string]any{"properties": map[string]any{"effort": map[string]any{"enum": allowed}}},
			}}
			for _, update := range []bool{false, true} {
				raw := map[string]any{"max_tokens": 1024, "thinking": map[string]any{"type": "adaptive"}, "output_config": map[string]any{"effort": "xhigh"}, "messages": []any{
					map[string]any{"role": "user", "content": "hello"},
					map[string]any{"role": "assistant", "content": "hello"},
					map[string]any{"role": "user", "content": "hello again"},
				}}
				if update {
					raw["output_config"] = map[string]any{"effort": "low"}
					raw["messages"] = append(raw["messages"].([]any), map[string]any{"role": "system", "content": []any{}, "output_config": map[string]any{"effort": "xhigh"}})
				}
				encoded, err := json.Marshal(raw)
				require.NoError(t, err)
				body, parsed, err := Convert(encoded, model, "session", credentials)
				require.NoError(t, err)
				var out map[string]any
				require.NoError(t, json.Unmarshal(body, &out))
				fields := out["additionalModelRequestFields"].(map[string]any)
				require.Equal(t, want, fields["output_config"].(map[string]any)["effort"])
				require.JSONEq(t, `{"effort":"xhigh"}`, string(parsed.OutputConfig))
				state := out["conversationState"].(map[string]any)
				require.Len(t, state["history"], 2)
				user := state["currentMessage"].(map[string]any)["userInputMessage"].(map[string]any)
				require.Equal(t, ModelID(model), user["modelId"])
				require.Equal(t, "hello again", user["content"])
			}
		})
	}
}

func TestEffortCompatibilityPreservesOtherOutputControls(t *testing.T) {
	schema := map[string]any{"properties": map[string]any{"effort": map[string]any{"enum": []string{"high", "max"}}}}
	out, err := normalizeOutputEffort(json.RawMessage(`{"effort":"xhigh","format":{"type":"json_schema","schema":{"type":"object"}}}`), schema)
	require.NoError(t, err)
	require.JSONEq(t, `{"effort":"high","format":{"type":"json_schema","schema":{"type":"object"}}}`, string(out))
	for _, effort := range []string{"high", "max"} {
		raw := json.RawMessage(`{"effort":"` + effort + `"}`)
		out, err := normalizeOutputEffort(raw, schema)
		require.NoError(t, err)
		require.JSONEq(t, string(raw), string(out))
	}
	for _, raw := range []string{`[]`, `null`, `{"effort":null}`, `{"effort":42}`, `{"effort":"invalid"}`} {
		_, err := normalizeOutputEffort(json.RawMessage(raw), schema)
		require.Error(t, err)
	}
	// No advertised enum means no capability inference or model-name guessing.
	out, err = normalizeOutputEffort(json.RawMessage(`{"effort":"xhigh"}`), map[string]any{})
	require.NoError(t, err)
	require.JSONEq(t, `{"effort":"xhigh"}`, string(out))
}

func TestGenerationValidationDiagnosticsDoNotExposeUpstreamText(t *testing.T) {
	for _, tc := range []struct{ raw, reason string }{
		{`{"reason":"REQUEST_BODY_INVALID","message":"Invalid additionalModelRequestFields: secret request text"}`, "invalid_model_fields"},
		{`{"reason":"REQUEST_BODY_INVALID","message":"secret request text"}`, "invalid_request_body"},
		{`{"reason":"CONTENT_FILTERED","message":"secret request text"}`, ""},
		{`<html>secret request text</html>`, ""},
	} {
		err := GenerationResponseError(400, []byte(tc.raw))
		require.Equal(t, tc.reason, err.Reason)
		require.NotContains(t, err.Error(), "secret")
		require.Contains(t, err.Error(), "HTTP 400")
	}
}
