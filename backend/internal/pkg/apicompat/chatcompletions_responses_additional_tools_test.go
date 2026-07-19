package apicompat

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestResponsesToChatCompletionsRequestAdditionalTools(t *testing.T) {
	topLevelParameters := json.RawMessage(`{"type":"object","properties":{"top":{"type":"string"}}}`)
	req := &ResponsesRequest{
		Model: "test",
		Input: json.RawMessage(`[
			{"type":"additional_tools","role":"developer","tools":[
				{"type":"function","name":"wait","parameters": { "type": "object", "properties": { "seconds": { "type": "integer" } } }},
				{"type":"namespace","name":"mcp","tools":[
					{"type":"function","name":"read","parameters":{"type":"object"}}
				]}
			]},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"read it"}]}
		]`),
		Tools: []ResponsesTool{{Type: "function", Name: "top", Parameters: topLevelParameters}},
		ToolChoice: json.RawMessage(
			`{"type":"function","namespace":"mcp","name":"read"}`,
		),
	}

	conversion, err := ResponsesToChatCompletionsRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(conversion.Request.Tools); got != 3 {
		t.Fatalf("tools length = %d, want 3", got)
	}
	wantNames := []string{"top", "wait", "mcp__read"}
	for i, want := range wantNames {
		if got := conversion.Request.Tools[i].Function.Name; got != want {
			t.Fatalf("tool %d name = %q, want %q", i, got, want)
		}
	}
	if got := string(conversion.Request.Tools[1].Function.Parameters); got != `{ "type": "object", "properties": { "seconds": { "type": "integer" } } }` {
		t.Fatalf("additional tool parameters bytes changed: %s", got)
	}
	if got := string(req.Tools[0].Parameters); got != string(topLevelParameters) {
		t.Fatalf("top-level tool parameters changed: %s", got)
	}
	if !bytes.Contains(conversion.Request.ToolChoice, []byte(`"mcp__read"`)) {
		t.Fatalf("tool choice was not flattened: %s", conversion.Request.ToolChoice)
	}
	if got := conversion.NamespaceTools["mcp__read"]; got != (NamespacedToolName{Namespace: "mcp", Name: "read"}) {
		t.Fatalf("namespace mapping = %#v", got)
	}
	if got := len(conversion.Request.Messages); got != 1 || conversion.Request.Messages[0].Role != "user" {
		t.Fatalf("messages = %#v", conversion.Request.Messages)
	}
}

func TestResponsesToChatCompletionsRequestAdditionalToolsValidation(t *testing.T) {
	t.Run("malformed tools", func(t *testing.T) {
		_, err := ResponsesToChatCompletionsRequest(&ResponsesRequest{
			Input: json.RawMessage(`[{"type":"additional_tools","tools":{"type":"function"}}]`),
		})
		if err == nil || !strings.Contains(err.Error(), "parse responses additional tools") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("cross-field conflict", func(t *testing.T) {
		_, err := ResponsesToChatCompletionsRequest(&ResponsesRequest{
			Input: json.RawMessage(`[{"type":"additional_tools","tools":[{"type":"function","name":"run","description":"additional"}]}]`),
			Tools: []ResponsesTool{{Type: "function", Name: "run", Description: "top-level"}},
		})
		if err == nil || !strings.Contains(err.Error(), "conflicting function tool declarations") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("string items remain messages", func(t *testing.T) {
		conversion, err := ResponsesToChatCompletionsRequest(&ResponsesRequest{
			Input: json.RawMessage(`["plain input",{"type":"additional_tools","tools":[{"type":"function","name":"run"}]}]`),
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(conversion.Request.Messages) != 1 || len(conversion.Request.Tools) != 1 {
			t.Fatalf("conversion = %#v", conversion)
		}
	})

	t.Run("unsupported additional tools do not leak tool choice", func(t *testing.T) {
		conversion, err := ResponsesToChatCompletionsRequest(&ResponsesRequest{
			Input:      json.RawMessage(`[{"type":"additional_tools","tools":[{"type":"web_search"},{"type":"custom","name":"exec"},{"type":"tool_search"}]}]`),
			ToolChoice: json.RawMessage(`"auto"`),
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(conversion.Request.Tools) != 0 || len(conversion.Request.ToolChoice) != 0 {
			t.Fatalf("unsupported tools leaked into fallback: %#v", conversion.Request)
		}
	})

	for _, tc := range []struct {
		name            string
		topLevelTools   []ResponsesTool
		additionalTools string
		toolChoice      string
	}{
		{
			name:            "forced additional custom with top-level function",
			topLevelTools:   []ResponsesTool{{Type: "function", Name: "lookup"}},
			additionalTools: `[{"type":"custom","name":"exec"}]`,
			toolChoice:      `{"type":"custom","name":"exec"}`,
		},
		{
			name:            "forced top-level tool search with additional function",
			topLevelTools:   []ResponsesTool{{Type: "tool_search", Name: "search"}},
			additionalTools: `[{"type":"function","name":"lookup"}]`,
			toolChoice:      `{"type":"tool_search","name":"search"}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conversion, err := ResponsesToChatCompletionsRequest(&ResponsesRequest{
				Input:      json.RawMessage(`[{"type":"additional_tools","tools":` + tc.additionalTools + `}]`),
				Tools:      tc.topLevelTools,
				ToolChoice: json.RawMessage(tc.toolChoice),
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(conversion.Request.Tools) != 1 || conversion.Request.Tools[0].Function.Name != "lookup" {
				t.Fatalf("converted tools = %#v", conversion.Request.Tools)
			}
			if len(conversion.Request.ToolChoice) != 0 {
				t.Fatalf("excluded forced tool choice leaked: %s", conversion.Request.ToolChoice)
			}
		})
	}

	t.Run("supported forced function remains valid with additional tools", func(t *testing.T) {
		conversion, err := ResponsesToChatCompletionsRequest(&ResponsesRequest{
			Input:      json.RawMessage(`[{"type":"additional_tools","tools":[{"type":"custom","name":"exec"}]}]`),
			Tools:      []ResponsesTool{{Type: "function", Name: "lookup"}},
			ToolChoice: json.RawMessage(`{"type":"function","name":"lookup"}`),
		})
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(conversion.Request.ToolChoice, []byte(`"lookup"`)) {
			t.Fatalf("supported forced function was dropped: %s", conversion.Request.ToolChoice)
		}
	})

	t.Run("empty additional tools still scopes choice validation", func(t *testing.T) {
		conversion, err := ResponsesToChatCompletionsRequest(&ResponsesRequest{
			Input:      json.RawMessage(`[{"type":"additional_tools","tools":[]}]`),
			ToolChoice: json.RawMessage(`{"type":"custom","name":"exec"}`),
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(conversion.Request.ToolChoice) != 0 {
			t.Fatalf("excluded forced tool choice leaked: %s", conversion.Request.ToolChoice)
		}
	})
}

func TestResponsesToChatCompletionsRequestDecodesInputArrayOnce(t *testing.T) {
	for _, tc := range []struct {
		name           string
		additionalTool bool
	}{
		{name: "ordinary fallback"},
		{name: "additional tools", additionalTool: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := largeResponsesInput(256, tc.additionalTool)
			req := &ResponsesRequest{Model: "test", Input: input}
			original := append(json.RawMessage(nil), input...)
			decodeCalls := 0

			conversion, err := responsesToChatCompletionsRequest(req, func(raw json.RawMessage) ([]json.RawMessage, error) {
				decodeCalls++
				if decodeCalls > 1 {
					t.Fatalf("input array decoded %d times", decodeCalls)
				}
				if !bytes.Equal(raw, original) {
					t.Fatalf("decoder received changed input")
				}
				items, decodeErr := decodeResponsesInputItems(raw)
				// Any later attempt to decode req.Input must fail. RawMessage items
				// returned above own their bytes and remain valid.
				for i := range raw {
					raw[i] = '!'
				}
				return items, decodeErr
			})
			if err != nil {
				t.Fatal(err)
			}
			if decodeCalls != 1 {
				t.Fatalf("input array decode calls = %d, want 1", decodeCalls)
			}
			if got := len(conversion.Request.Messages); got != 256 {
				t.Fatalf("messages length = %d, want 256", got)
			}
			wantTools := 0
			if tc.additionalTool {
				wantTools = 1
			}
			if got := len(conversion.Request.Tools); got != wantTools {
				t.Fatalf("tools length = %d, want %d", got, wantTools)
			}
		})
	}
}

func largeResponsesInput(messageCount int, additionalTool bool) json.RawMessage {
	var input strings.Builder
	input.WriteByte('[')
	if additionalTool {
		input.WriteString(`{"type":"additional_tools","tools":[{"type":"function","name":"wait","parameters":{"type":"object"}}]},`)
	}
	for i := 0; i < messageCount; i++ {
		if i > 0 {
			input.WriteByte(',')
		}
		input.WriteString(`{"type":"message","role":"user","content":[{"type":"input_text","text":"payload"}]}`)
	}
	input.WriteByte(']')
	return json.RawMessage(input.String())
}
