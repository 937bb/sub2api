package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
)

func TestNeutralMarkers_GeneratedContent(t *testing.T) {
	for _, value := range []string{codexPythonToolAlias, codexImageGenerationBridgeText, codexSparkImageUnsupportedText, openAICompatClaudeCodeTodoGuardText} {
		require.NotContains(t, strings.ToLower(value), "sub2api")
	}
	for _, tc := range []struct {
		name, marker, legacy string
		apply                func(map[string]any) bool
	}{
		{"image", codexImageGenerationBridgeMarker, legacyCodexImageGenerationBridgeMarker, applyCodexImageGenerationBridgeInstructions},
		{"spark", codexSparkImageUnsupportedMarker, legacyCodexSparkImageUnsupportedMarker, applyCodexSparkImageUnsupportedInstructions},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const original = "Explain sub2api; preserve the user's wording."
			body := map[string]any{"instructions": original, "model": "gpt-5.5", "tools": []any{map[string]any{"type": "image_generation"}}}
			require.True(t, tc.apply(body))
			text := body["instructions"].(string)
			require.True(t, strings.HasPrefix(text, original+"\n\n"))
			require.Contains(t, text, tc.marker)
			require.Contains(t, text, "</"+strings.TrimPrefix(tc.marker, "<"))
			require.False(t, tc.apply(body))
			require.Equal(t, text, body["instructions"])
			body["instructions"] = original + "\n" + tc.legacy
			require.False(t, tc.apply(body))
			require.Equal(t, original+"\n"+tc.legacy, body["instructions"])
		})
	}
}

func TestNeutralMarkers_TodoGuardAndBridgeRoundTrip(t *testing.T) {
	for _, marker := range []string{openAICompatClaudeCodeTodoGuardMarker, legacyOpenAICompatClaudeCodeTodoGuardMarker} {
		t.Run(marker, func(t *testing.T) {
			input := []any{map[string]any{"type": "message", "role": "developer", "content": []any{map[string]any{"type": "input_text", "text": marker + "\nKeep tasks consistent."}}}}
			body := map[string]any{"input": input}
			raw, err := json.Marshal(body)
			require.NoError(t, err)
			// Go JSON encoding escapes angle brackets; detection must still work.
			require.Contains(t, string(raw), `\u003c`)
			require.True(t, isOpenAICompatMessagesBridgeBody(raw))
			unescaped := strings.NewReplacer(`\u003c`, "<", `\u003e`, ">").Replace(string(raw))
			require.True(t, isOpenAICompatMessagesBridgeBody([]byte(unescaped)))
			require.True(t, isOpenAICompatMessagesBridgeRequestBody(body))
			require.False(t, appendOpenAICompatClaudeCodeTodoGuardToRequestBody(body))
			after, err := json.Marshal(body)
			require.NoError(t, err)
			require.Equal(t, raw, after)
			inputJSON, err := json.Marshal(input)
			require.NoError(t, err)
			req := &apicompat.ResponsesRequest{Input: inputJSON}
			require.False(t, appendOpenAICompatClaudeCodeTodoGuard(req))
			require.Equal(t, json.RawMessage(inputJSON), req.Input)
		})
	}
}

func TestNeutralMarkers_BridgeIgnoresUnrelatedMetadata(t *testing.T) {
	body := map[string]any{"input": []any{map[string]any{"role": "user", "content": "hello"}}, "metadata": map[string]any{"note": openAICompatClaudeCodeTodoGuardMarker}}
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	require.False(t, isOpenAICompatMessagesBridgeBody(raw))
	require.False(t, isOpenAICompatMessagesBridgeRequestBody(body))
	require.False(t, isOpenAICompatMessagesBridgeBody(nil))
	require.False(t, isOpenAICompatMessagesBridgeRequestBody(nil))
}

func TestNeutralMarkers_UserQuotesAndToolArgumentsAreNotBridgeSignals(t *testing.T) {
	for _, marker := range []string{openAICompatClaudeCodeTodoGuardMarker, legacyOpenAICompatClaudeCodeTodoGuardMarker} {
		for _, item := range []map[string]any{
			{"type": "message", "role": "user", "content": marker},
			{"type": "message", "role": "assistant", "content": []any{map[string]any{"type": "text", "text": marker}}},
			{"type": "function_call", "name": "read_file", "arguments": marker},
			{"type": "message", "role": "developer", "content": []any{map[string]any{"type": "input_image", "image_url": marker}}},
		} {
			body := map[string]any{"input": []any{item}}
			raw, err := json.Marshal(body)
			require.NoError(t, err)
			require.False(t, isOpenAICompatMessagesBridgeBody(raw))
			require.False(t, isOpenAICompatMessagesBridgeRequestBody(body))
			require.True(t, appendOpenAICompatClaudeCodeTodoGuardToRequestBody(body))
			require.Len(t, body["input"], 2)
		}
	}
}

func TestNeutralMarkers_NewTodoGuardIsIdempotent(t *testing.T) {
	input := json.RawMessage(`[{"type":"message","role":"user","content":[{"type":"input_text","text":"Explain sub2api"}]}]`)
	req := &apicompat.ResponsesRequest{Input: input}
	require.True(t, appendOpenAICompatClaudeCodeTodoGuard(req))
	first := append(json.RawMessage(nil), req.Input...)
	require.False(t, appendOpenAICompatClaudeCodeTodoGuard(req))
	require.Equal(t, first, req.Input)
	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{"input":`+string(input)+`}`), &body))
	require.False(t, isOpenAICompatMessagesBridgeRequestBody(body))
	require.True(t, appendOpenAICompatClaudeCodeTodoGuardToRequestBody(body))
	require.False(t, appendOpenAICompatClaudeCodeTodoGuardToRequestBody(body))
	require.True(t, isOpenAICompatMessagesBridgeRequestBody(body))
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	require.Contains(t, string(raw), "Explain sub2api")
	require.NotContains(t, string(raw), "sub2api-claude")
}

func TestNeutralMarkers_LegacyNamedUserToolIsNotRewritten(t *testing.T) {
	const original = `{"tools":[{"type":"function","name":"python__sub2api"}],"input":[{"type":"function_call","name":"python__sub2api","call_id":"call_1"}]}`
	body, reverse, changed, err := aliasOpenAIOAuthReservedToolNamesBody([]byte(original))
	require.NoError(t, err)
	require.False(t, changed)
	require.Empty(t, reverse)
	require.JSONEq(t, original, string(body))
}

func TestNeutralMarkers_GuardPreservesOpaqueInputFields(t *testing.T) {
	input := json.RawMessage(`[{"role":"developer","content":"Keep the tool contract."},{"type":"custom_tool_call","id":"ctc_1","call_id":"call_1","name":"patch","input":"original bytes","extension":{"large":9007199254740993}},{"type":"function_call_output","call_id":"call_2","output":[{"type":"input_image","image_url":"data:image/png;base64,AAAA"}]},{"type":"reasoning","encrypted_content":"opaque","summary":[],"status":"completed"}]`)
	var before []json.RawMessage
	require.NoError(t, json.Unmarshal(input, &before))
	req := &apicompat.ResponsesRequest{Input: input}
	require.True(t, appendOpenAICompatClaudeCodeTodoGuard(req))
	var after []json.RawMessage
	require.NoError(t, json.Unmarshal(req.Input, &after))
	require.Len(t, after, len(before)+1)
	require.JSONEq(t, string(before[0]), string(after[0]), "existing developer prefix must stay first")
	for i := 1; i < len(before); i++ {
		// Raw equality also catches rounding of opaque large integer fields.
		require.Equal(t, string(before[i]), string(after[i+1]))
	}
	require.False(t, appendOpenAICompatClaudeCodeTodoGuard(req))
}
