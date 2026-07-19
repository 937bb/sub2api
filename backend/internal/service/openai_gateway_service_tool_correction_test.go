package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestOpenAIGatewayService_ToolCorrection 测试 OpenAIGatewayService 中的工具修正集成
func TestOpenAIGatewayService_ToolCorrection(t *testing.T) {
	// 创建一个简单的 service 实例来测试工具修正
	service := &OpenAIGatewayService{
		toolCorrector: NewCodexToolCorrector(),
	}

	tests := []struct {
		name     string
		input    []byte
		expected string
		changed  bool
	}{
		{
			name: "correct apply_patch in response body",
			input: []byte(`{
				"choices": [{
					"message": {
						"tool_calls": [{
							"function": {"name": "apply_patch"}
						}]
					}
				}]
			}`),
			expected: "edit",
			changed:  true,
		},
		{
			name: "correct update_plan in response body",
			input: []byte(`{
				"tool_calls": [{
					"function": {"name": "update_plan"}
				}]
			}`),
			expected: "todowrite",
			changed:  true,
		},
		{
			name: "no change for correct tool name",
			input: []byte(`{
				"tool_calls": [{
					"function": {"name": "edit"}
				}]
			}`),
			expected: "edit",
			changed:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.correctToolCallsInResponseBody(tt.input)
			resultStr := string(result)

			// 检查是否包含期望的工具名称
			if !strings.Contains(resultStr, tt.expected) {
				t.Errorf("expected result to contain %q, got %q", tt.expected, resultStr)
			}

			// 对于预期有变化的情况，验证结果与输入不同
			if tt.changed && string(result) == string(tt.input) {
				t.Error("expected result to be different from input, but they are the same")
			}

			// 对于预期无变化的情况，验证结果与输入相同
			if !tt.changed && string(result) != string(tt.input) {
				t.Error("expected result to be same as input, but they are different")
			}
		})
	}
}

func TestNormalizeOpenAIResponsesFunctionCallArguments_DedupesOnlySupportedShapes(t *testing.T) {
	doubled := `{\"cmd\":\"pwd\"}{\"cmd\":\"pwd\"}`

	tests := []struct {
		name string
		body string
		path string
	}{
		{
			name: "function_call_arguments_done_top_level",
			body: `{"type":"response.function_call_arguments.done","output_index":0,"item_id":"fc_1","arguments":"` + doubled + `"}`,
			path: "arguments",
		},
		{
			name: "function_call_arguments_done_nested_legacy",
			body: `{"type":"response.function_call_arguments.done","response":{"function_call_arguments":{"done":{"arguments":"` + doubled + `"}}}}`,
			path: "response.function_call_arguments.done.arguments",
		},
		{
			name: "function_call_item",
			body: `{"type":"response.output_item.done","item":{"type":"function_call","arguments":"` + doubled + `"}}`,
			path: "item.arguments",
		},
		{
			name: "custom_tool_call_item",
			body: `{"type":"response.output_item.done","item":{"type":"custom_tool_call","arguments":"` + doubled + `"}}`,
			path: "item.arguments",
		},
		{
			name: "response_output",
			body: `{"response":{"output":[{"type":"function_call","arguments":"` + doubled + `"}]}}`,
			path: "response.output.0.arguments",
		},
		{
			name: "top_level_output",
			body: `{"output":[{"type":"custom_tool_call","arguments":"` + doubled + `"}]}`,
			path: "output.0.arguments",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalized, changed := normalizeOpenAIResponsesFunctionCallArguments([]byte(tt.body))
			require.True(t, changed)
			require.JSONEq(t, `{"cmd":"pwd"}`, gjson.GetBytes(normalized, tt.path).String())
		})
	}
}

func TestNormalizeOpenAIResponsesFunctionCallArguments_DoesNotRewriteUnsupportedJSON(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "single_arguments_json",
			body: `{"item":{"type":"function_call","arguments":"{\"cmd\":\"pwd\"}"}}`,
		},
		{
			name: "different_repeated_json",
			body: `{"item":{"type":"function_call","arguments":"{\"cmd\":\"pwd\"}{\"cmd\":\"ls\"}"}}`,
		},
		{
			name: "unsupported_item_type",
			body: `{"item":{"type":"message","arguments":"{\"cmd\":\"pwd\"}{\"cmd\":\"pwd\"}"}}`,
		},
		{
			name: "arbitrary_top_level_arguments",
			body: `{"type":"response.completed","arguments":"{\"cmd\":\"pwd\"}{\"cmd\":\"pwd\"}"}`,
		},
		{
			name: "nested_done_arguments_without_done_event_type",
			body: `{"type":"response.completed","response":{"function_call_arguments":{"done":{"arguments":"{\"cmd\":\"pwd\"}{\"cmd\":\"pwd\"}"}}}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalized, changed := normalizeOpenAIResponsesFunctionCallArguments([]byte(tt.body))
			require.False(t, changed)
			require.JSONEq(t, tt.body, string(normalized))
		})
	}
}

func TestCorrectToolCallsInResponseBodyDeduplicatesResponsesArguments(t *testing.T) {
	service := &OpenAIGatewayService{toolCorrector: NewCodexToolCorrector()}
	input := []byte(`{"output":[{"type":"function_call","arguments":"{\"cmd\":\"pwd\"}{\"cmd\":\"pwd\"}"}]}`)

	result := service.correctToolCallsInResponseBody(input)

	require.JSONEq(t, `{"cmd":"pwd"}`, gjson.GetBytes(result, "output.0.arguments").String())
}

func TestNormalizeOpenAIResponsesFunctionCallOutputArguments_DedupesOnlyFunctionCallOutputs(t *testing.T) {
	input := []byte(`{"output":[{"type":"function_call","arguments":"{\"cmd\":\"pwd\"}{\"cmd\":\"pwd\"}"},{"type":"message","arguments":"{\"cmd\":\"pwd\"}{\"cmd\":\"pwd\"}"},{"type":"custom_tool_call","arguments":"[1][1]"}],"arguments":"{\"cmd\":\"pwd\"}{\"cmd\":\"pwd\"}"}`)

	normalized, changed := normalizeOpenAIResponsesFunctionCallOutputArguments(input)

	require.True(t, changed)
	require.JSONEq(t, `{"cmd":"pwd"}`, gjson.GetBytes(normalized, "output.0.arguments").String())
	require.Equal(t, `{"cmd":"pwd"}{"cmd":"pwd"}`, gjson.GetBytes(normalized, "output.1.arguments").String())
	require.JSONEq(t, `[1]`, gjson.GetBytes(normalized, "output.2.arguments").String())
	require.Equal(t, `{"cmd":"pwd"}{"cmd":"pwd"}`, gjson.GetBytes(normalized, "arguments").String())
}

func TestNormalizeOpenAIResponsesFunctionCallOutputArguments_DoesNotRewriteTextOrDifferentJSON(t *testing.T) {
	input := []byte(`{"output":[{"type":"function_call","arguments":"plain text plain text"},{"type":"custom_tool_call","arguments":"{\"cmd\":\"pwd\"}{\"cmd\":\"ls\"}"}]}`)

	normalized, changed := normalizeOpenAIResponsesFunctionCallOutputArguments(input)

	require.False(t, changed)
	require.JSONEq(t, string(input), string(normalized))
}

func TestDedupeRepeatedJSONArgumentString(t *testing.T) {
	deduped, changed := dedupeRepeatedJSONArgumentString(`{"cmd":"pwd"}{"cmd":"pwd"}`)
	require.True(t, changed)
	require.JSONEq(t, `{"cmd":"pwd"}`, deduped)

	deduped, changed = dedupeRepeatedJSONArgumentString(`[{"cmd":"pwd"}][{"cmd":"pwd"}]`)
	require.True(t, changed)
	var parsed []map[string]string
	require.NoError(t, json.Unmarshal([]byte(deduped), &parsed))
	require.Equal(t, []map[string]string{{"cmd": "pwd"}}, parsed)

	unchanged, changed := dedupeRepeatedJSONArgumentString(`"abc""abc"`)
	require.False(t, changed)
	require.Equal(t, `"abc""abc"`, unchanged)
}

// TestOpenAIGatewayService_ToolCorrectorInitialization 测试工具修正器是否正确初始化
func TestOpenAIGatewayService_ToolCorrectorInitialization(t *testing.T) {
	service := &OpenAIGatewayService{
		toolCorrector: NewCodexToolCorrector(),
	}

	if service.toolCorrector == nil {
		t.Fatal("toolCorrector should not be nil")
	}

	// 测试修正器可以正常工作
	data := `{"tool_calls":[{"function":{"name":"apply_patch"}}]}`
	corrected, changed := service.toolCorrector.CorrectToolCallsInSSEData(data)

	if !changed {
		t.Error("expected tool call to be corrected")
	}

	if !strings.Contains(corrected, "edit") {
		t.Errorf("expected corrected data to contain 'edit', got %q", corrected)
	}
}

// TestToolCorrectionStats 测试工具修正统计功能
func TestToolCorrectionStats(t *testing.T) {
	service := &OpenAIGatewayService{
		toolCorrector: NewCodexToolCorrector(),
	}

	// 执行几次修正
	testData := []string{
		`{"tool_calls":[{"function":{"name":"apply_patch"}}]}`,
		`{"tool_calls":[{"function":{"name":"update_plan"}}]}`,
		`{"tool_calls":[{"function":{"name":"apply_patch"}}]}`,
	}

	for _, data := range testData {
		service.toolCorrector.CorrectToolCallsInSSEData(data)
	}

	stats := service.toolCorrector.GetStats()

	if stats.TotalCorrected != 3 {
		t.Errorf("expected 3 corrections, got %d", stats.TotalCorrected)
	}

	if stats.CorrectionsByTool["apply_patch->edit"] != 2 {
		t.Errorf("expected 2 apply_patch->edit corrections, got %d", stats.CorrectionsByTool["apply_patch->edit"])
	}

	if stats.CorrectionsByTool["update_plan->todowrite"] != 1 {
		t.Errorf("expected 1 update_plan->todowrite correction, got %d", stats.CorrectionsByTool["update_plan->todowrite"])
	}
}
