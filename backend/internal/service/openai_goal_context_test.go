package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGoalContextRecognition(t *testing.T) {
	for _, tt := range []struct {
		name  string
		patch map[string]any
		want  bool
	}{
		{"missing_call_id", nil, true},
		{"null_call_id", map[string]any{"call_id": nil}, true},
		{"empty_call_id", map[string]any{"call_id": ""}, true},
		{"blank_call_id", map[string]any{"call_id": " "}, true},
		{"real_call_id", map[string]any{"call_id": "call_goal"}, false},
		{"numeric_call_id", map[string]any{"call_id": 0}, false},
		{"boolean_call_id", map[string]any{"call_id": false}, false},
		{"object_call_id", map[string]any{"call_id": map[string]any{}}, false},
		{"wrong_namespace", map[string]any{"namespace": "functions"}, false},
		{"missing_namespace", map[string]any{"namespace": nil}, false},
		{"dynamic_context_name", map[string]any{"name": "browser_info"}, true},
		{"browser_page", map[string]any{"name": "codex_apps_open_page"}, true},
		{"browser_selection", map[string]any{"name": "codex_apps_selected_page_part_1_of_1"}, true},
		{"future_context_name", map[string]any{"name": "future_context"}, true},
		{"null_name", map[string]any{"name": nil}, false},
		{"blank_name", map[string]any{"name": " "}, false},
		{"numeric_name", map[string]any{"name": 42}, false},
		{"wrong_type", map[string]any{"type": "custom_tool_call_output"}, false},
		{"null_output", map[string]any{"output": nil}, false},
		{"structured_output", map[string]any{"output": []any{}}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			item := map[string]any{"type": "function_call_output", "namespace": "agent", "name": "goal_objective", "output": "Continue the original objective."}
			for k, v := range tt.patch {
				item[k] = v
			}
			raw, err := json.Marshal(item)
			require.NoError(t, err)
			require.Equal(t, tt.want, isCodexAgentContextOutput(item))
			require.Equal(t, tt.want, isCodexAgentContextOutputJSON(gjson.ParseBytes(raw)))
			if !tt.want {
				return
			}
			req := map[string]any{"input": []any{item}}
			body, err := json.Marshal(req)
			require.NoError(t, err)
			require.False(t, NeedsToolContinuation(req))
			require.Empty(t, AnalyzeToolContinuationSignals(req))
			require.Empty(t, ValidateFunctionCallOutputContext(req))
			require.Empty(t, ValidateFunctionCallOutputContextBytes(body))
			require.Empty(t, AnalyzeToolCallOutputContextCoverageBytes(body))
			require.False(t, openAIWSRawItemsHasFunctionCallOutput([]json.RawMessage{raw}))
			require.False(t, openAIWSRawPayloadHasToolCallOutput(body))
			objectBody, err := json.Marshal(map[string]any{"input": item})
			require.NoError(t, err)
			require.False(t, openAIWSRawPayloadHasToolCallOutput(objectBody))
		})
	}
}

func TestGoalContextMixedToolOutputs(t *testing.T) {
	goal := map[string]any{"type": "function_call_output", "namespace": "agent", "name": "goal_objective", "output": "Continue", "call_id": nil}
	for _, tt := range []struct {
		name             string
		items            []any
		missing, covered bool
	}{
		{"ordinary_missing_id", []any{map[string]any{"type": "function_call_output", "output": "bad"}}, true, false},
		{"ordinary_unreferenced", []any{map[string]any{"type": "function_call_output", "call_id": "call_a", "output": "result"}}, false, false},
		{"ordinary_referenced", []any{map[string]any{"type": "item_reference", "id": "call_a"}, map[string]any{"type": "function_call_output", "call_id": "call_a", "output": "result"}}, false, true},
		{"ordinary_paired", []any{map[string]any{"type": "function_call", "call_id": "call_a", "name": "lookup", "arguments": "{}"}, map[string]any{"type": "function_call_output", "call_id": "call_a", "output": "result"}}, false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, goalFirst := range []bool{false, true} {
				input := append([]any{}, tt.items...)
				if goalFirst {
					input = append([]any{goal}, input...)
				} else {
					input = append(input, goal)
				}
				req := map[string]any{"input": input}
				body, err := json.Marshal(req)
				require.NoError(t, err)
				require.True(t, NeedsToolContinuation(req))
				signals := AnalyzeToolContinuationSignals(req)
				require.True(t, signals.HasFunctionCallOutput)
				require.Equal(t, tt.missing, signals.HasFunctionCallOutputMissingCallID)
				validation := ValidateFunctionCallOutputContextBytes(body)
				require.Equal(t, ValidateFunctionCallOutputContext(req), validation)
				require.Equal(t, tt.missing, validation.HasFunctionCallOutputMissingCallID)
				coverage := AnalyzeToolCallOutputContextCoverageBytes(body)
				require.True(t, coverage.HasFunctionCallOutput)
				require.Equal(t, tt.covered, coverage.ContextCoversAllCallIDs)
				var items []json.RawMessage
				require.NoError(t, json.Unmarshal([]byte(gjson.GetBytes(body, "input").Raw), &items))
				require.True(t, openAIWSRawItemsHasFunctionCallOutput(items))
				require.True(t, openAIWSRawPayloadHasToolCallOutput(body))
				require.Equal(t, tt.name == "ordinary_paired", openAIWSRawItemsHaveToolCallContextForOutputs(items))
			}
		})
	}
}

func TestWebSocketCompatibilityPreservesGoalContext(t *testing.T) {
	for _, suffix := range []string{"", ",\"call_id\":null", ",\"call_id\":\"\""} {
		body := []byte(`{"type":"response.create","model":"gpt-5.5","input":[{"type":"function_call_output","namespace":"agent","name":"goal_objective","output":"Continue the original goal."` + suffix + `}]}`)
		account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
		normalized, _, err := normalizeOpenAIResponsesWebSocketCompatibilityBody(body, account, false)
		require.NoError(t, err)
		require.JSONEq(t, gjson.GetBytes(body, "input").Raw, gjson.GetBytes(normalized, "input").Raw)
		again, changed, err := normalizeOpenAIResponsesWebSocketCompatibilityBody(normalized, account, false)
		require.NoError(t, err)
		require.False(t, changed)
		require.JSONEq(t, string(normalized), string(again))
	}
}

func TestHTTPNamespaceCleanupPreservesGoalContext(t *testing.T) {
	for _, keepToolCalls := range []bool{false, true} {
		body := []byte(`{"input":[{"type":"function_call_output","namespace":"agent","name":"goal_objective","output":"Continue","call_id":null},{"type":"message","role":"user","namespace":"obsolete","content":"Continue"}]}`)
		normalized, err := stripOpenAIResponsesInputNamespaces(body, keepToolCalls)
		require.NoError(t, err)
		require.JSONEq(t, gjson.GetBytes(body, "input.0").Raw, gjson.GetBytes(normalized, "input.0").Raw)
		require.False(t, gjson.GetBytes(normalized, "input.1.namespace").Exists())
		require.False(t, ValidateFunctionCallOutputContextBytes(normalized).HasFunctionCallOutput)
		again, err := stripOpenAIResponsesInputNamespaces(normalized, keepToolCalls)
		require.NoError(t, err)
		require.Equal(t, normalized, again)
	}
}

func TestAgentContextSurvivesHTTPAndWebSocketHops(t *testing.T) {
	for _, name := range []string{"goal_objective", "codex_apps_open_page", "codex_apps_selected_page_part_1_of_1", "browser_info", "future_context"} {
		t.Run(name, func(t *testing.T) {
			contextItem := map[string]any{"type": "function_call_output", "namespace": "agent", "name": name, "output": "Context"}
			body, err := json.Marshal(map[string]any{"model": "gpt-6-astra", "input": []any{
				contextItem, map[string]any{"type": "message", "role": "user", "namespace": "obsolete", "content": "Continue"},
			}})
			require.NoError(t, err)
			original := gjson.GetBytes(body, "input.0").Raw
			for _, keepCalls := range []bool{false, true} {
				forwarded := body
				for hop := 0; hop < 2; hop++ {
					forwarded, err = stripOpenAIResponsesInputNamespaces(forwarded, keepCalls)
					require.NoError(t, err)
					require.JSONEq(t, original, gjson.GetBytes(forwarded, "input.0").Raw)
					require.Empty(t, ValidateFunctionCallOutputContextBytes(forwarded))
					require.False(t, gjson.GetBytes(forwarded, "input.1.namespace").Exists())
				}
			}
			account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
			forwarded, _, err := normalizeOpenAIResponsesWebSocketCompatibilityBody(body, account, false)
			require.NoError(t, err)
			require.JSONEq(t, original, gjson.GetBytes(forwarded, "input.0").Raw)
			require.False(t, openAIWSRawPayloadHasToolCallOutput(forwarded))

			for _, paired := range []bool{false, true} {
				ordinary := map[string]any{"type": "function_call_output", "output": "result"}
				if paired {
					ordinary["call_id"] = "call_real"
				}
				mixed, err := json.Marshal(map[string]any{"input": []any{contextItem, ordinary}})
				require.NoError(t, err)
				validation := ValidateFunctionCallOutputContextBytes(mixed)
				require.True(t, validation.HasFunctionCallOutput)
				require.Equal(t, !paired, validation.HasFunctionCallOutputMissingCallID)
				require.False(t, validation.HasItemReferenceForAllCallIDs)
			}
		})
	}
}
