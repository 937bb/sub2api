package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func functionCallRequest(t *testing.T, arguments string) *ResponsesRequest {
	t.Helper()
	input, err := json.Marshal([]ResponsesInputItem{
		{Type: "function_call", CallID: "call_wire", Name: "run", Arguments: arguments},
		{Type: "function_call_output", CallID: "call_wire", Output: "ok"},
	})
	require.NoError(t, err)
	return &ResponsesRequest{Model: "claude-test", Input: input}
}

func TestResponsesToAnthropicRequest_FunctionArgumentsInvalidWireValues(t *testing.T) {
	tests := []struct {
		name      string
		arguments string
		want      string
	}{
		{name: "null", arguments: `null`, want: "must be a JSON object, got null"},
		{name: "array", arguments: `[]`, want: "cannot unmarshal array"},
		{name: "string", arguments: `"value"`, want: "cannot unmarshal string"},
		{name: "boolean", arguments: `true`, want: "cannot unmarshal bool"},
		{name: "number", arguments: `42`, want: "cannot unmarshal number"},
		{name: "malformed", arguments: `{"x":`, want: "unexpected end of JSON input"},
		{name: "trailing", arguments: `{"x":1} {}`, want: "invalid character"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ResponsesToAnthropicRequest(functionCallRequest(t, tt.arguments))
			require.Error(t, err)
			require.ErrorContains(t, err, `responses input item 0 function_call "call_wire" arguments:`)
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestResponsesToAnthropicRequest_EmptyFunctionArgumentsWireObject(t *testing.T) {
	req, err := ResponsesToAnthropicRequest(functionCallRequest(t, ""))
	require.NoError(t, err)
	wire, err := json.Marshal(req)
	require.NoError(t, err)
	require.JSONEq(t, `{"model":"claude-test","messages":[{"role":"assistant","content":[{"type":"tool_use","id":"call_wire","name":"run","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_wire","content":"ok"}]}],"max_tokens":8192}`, string(wire))
}

func TestResponsesToAnthropicRequest_PreservesNestedFunctionArgumentsOnWire(t *testing.T) {
	arguments := `{"nested":{"array":[1,true,null,{"text":"value"}]},"large":9007199254740993}`
	req, err := ResponsesToAnthropicRequest(functionCallRequest(t, arguments))
	require.NoError(t, err)
	wire, err := json.Marshal(req)
	require.NoError(t, err)

	var decoded struct {
		Messages []struct {
			Content []struct {
				Input json.RawMessage `json:"input"`
			} `json:"content"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(wire, &decoded))
	require.JSONEq(t, arguments, string(decoded.Messages[0].Content[0].Input))
	require.Contains(t, string(wire), `9007199254740993`)
}

func TestResponsesToAnthropicRequest_RejectsDuplicateFunctionArgumentKeys(t *testing.T) {
	for _, arguments := range []string{
		`{"cmd":"first","cmd":"second"}`,
		`{"outer":{"value":1,"value":2}}`,
		`{"name":1,"n\u0061me":2}`,
		`{"items":[{"id":1,"id":2}]}`,
	} {
		t.Run(arguments, func(t *testing.T) {
			_, err := ResponsesToAnthropicRequest(functionCallRequest(t, arguments))
			require.ErrorContains(t, err, `responses input item 0 function_call "call_wire" arguments:`)
			require.ErrorContains(t, err, "duplicate object key")
		})
	}
}
