package apicompat

import (
	"encoding/json"
	"strings"
	"testing"
)

func namespaceRequest(tools []ResponsesTool) *ResponsesRequest {
	return &ResponsesRequest{Model: "test", Input: json.RawMessage(`"hi"`), Tools: tools}
}

func TestNamespaceToolsFlattenAndRestoreNonStreaming(t *testing.T) {
	tools := []ResponsesTool{{Type: "namespace", Name: "mcp", Tools: []ResponsesTool{{Type: "function", Name: "read", Parameters: json.RawMessage(`{"type":"object"}`)}}}}
	conversion, err := ResponsesToChatCompletionsRequest(namespaceRequest(tools))
	if err != nil || len(conversion.Request.Tools) != 1 || conversion.Request.Tools[0].Function.Name != "mcp__read" {
		t.Fatalf("conversion = %#v, %v", conversion, err)
	}
	resp := &ChatCompletionsResponse{Choices: []ChatChoice{{Message: ChatMessage{ToolCalls: []ChatToolCall{{ID: "call_1", Function: ChatFunctionCall{Name: "mcp__read", Arguments: `{}`}}}}}}}
	out := ChatCompletionsResponseToResponsesWithNamespace(resp, "test", conversion.NamespaceTools)
	if got := out.Output[0]; got.Name != "read" || got.Namespace != "mcp" {
		t.Fatalf("output = %#v", got)
	}
}

func TestNamespaceToolLongNameRestoresFromMap(t *testing.T) {
	ns, name := strings.Repeat("n", 40), strings.Repeat("x", 40)
	tools := []ResponsesTool{{Type: "namespace", Name: ns, Tools: []ResponsesTool{{Type: "function", Name: name}}}}
	conversion, err := ResponsesToChatCompletionsRequest(namespaceRequest(tools))
	if err != nil {
		t.Fatal(err)
	}
	flat := conversion.Request.Tools[0].Function.Name
	if len(flat) != 64 || flat == ns+"__"+name {
		t.Fatalf("flat name = %q", flat)
	}
	resp := &ChatCompletionsResponse{Choices: []ChatChoice{{Message: ChatMessage{ToolCalls: []ChatToolCall{{Function: ChatFunctionCall{Name: flat}}}}}}}
	got := ChatCompletionsResponseToResponsesWithNamespace(resp, "test", conversion.NamespaceTools).Output[0]
	if got.Name != name || got.Namespace != ns {
		t.Fatalf("output = %#v", got)
	}
}

func TestNamespaceToolCollisionRejected(t *testing.T) {
	_, err := ResponsesToChatCompletionsRequest(namespaceRequest([]ResponsesTool{
		{Type: "function", Name: "mcp__read"},
		{Type: "namespace", Name: "mcp", Tools: []ResponsesTool{{Type: "function", Name: "read"}}},
	}))
	if err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("error = %v", err)
	}
}

func TestNamespaceToolFlattenCollisionRejected(t *testing.T) {
	long := strings.Repeat("a", 70)
	_, err := ResponsesToChatCompletionsRequest(namespaceRequest([]ResponsesTool{
		{Type: "namespace", Name: "ns", Tools: []ResponsesTool{{Type: "function", Name: long}}},
		{Type: "namespace", Name: "ns", Children: []ResponsesTool{{Type: "function", Name: long + "different"}}},
	}))
	// SHA suffixes make these distinct; use an exact duplicate to verify de-duplication instead.
	if err != nil {
		t.Fatal(err)
	}
	_, err = ResponsesToChatCompletionsRequest(namespaceRequest([]ResponsesTool{
		{Type: "namespace", Name: "mcp", Tools: []ResponsesTool{{Type: "function", Name: "fs__read"}}},
		{Type: "namespace", Name: "mcp__fs", Tools: []ResponsesTool{{Type: "function", Name: "read"}}},
	}))
	if err == nil || !strings.Contains(err.Error(), "conflicting") {
		t.Fatalf("error = %v", err)
	}
}

func TestNamespaceToolExactDuplicateAllowedButConflictingDuplicateRejected(t *testing.T) {
	strict := true
	tool := ResponsesTool{Type: "function", Name: "read", Description: "read", Parameters: json.RawMessage(`{"type":"object"}`), Strict: &strict}
	conversion, err := ResponsesToChatCompletionsRequest(namespaceRequest([]ResponsesTool{{Type: "namespace", Name: "mcp", Tools: []ResponsesTool{tool, tool}}}))
	if err != nil || len(conversion.Request.Tools) != 1 {
		t.Fatalf("conversion = %#v, %v", conversion, err)
	}
	conflict := tool
	conflict.Description = "different"
	_, err = ResponsesToChatCompletionsRequest(namespaceRequest([]ResponsesTool{{Type: "namespace", Name: "mcp", Tools: []ResponsesTool{tool, conflict}}}))
	if err == nil || !strings.Contains(err.Error(), "conflicting") {
		t.Fatalf("error = %v", err)
	}
}

func TestTopLevelToolExactDuplicateAllowedButConflictingDuplicateRejected(t *testing.T) {
	tool := ResponsesTool{Type: "function", Name: "read", Description: "read"}
	conversion, err := ResponsesToChatCompletionsRequest(namespaceRequest([]ResponsesTool{tool, tool}))
	if err != nil || len(conversion.Request.Tools) != 1 {
		t.Fatalf("conversion = %#v, %v", conversion, err)
	}
	conflict := tool
	conflict.Description = "different"
	if _, err = ResponsesToChatCompletionsRequest(namespaceRequest([]ResponsesTool{tool, conflict})); err == nil {
		t.Fatal("conflicting duplicate accepted")
	}
}

func TestNamespaceToolChoiceFlattenedAndUnresolvedRejected(t *testing.T) {
	req := namespaceRequest([]ResponsesTool{{Type: "namespace", Name: "mcp", Tools: []ResponsesTool{{Type: "function", Name: "read"}}}})
	req.ToolChoice = json.RawMessage(`{"type":"function","namespace":"mcp","name":"read"}`)
	conversion, err := ResponsesToChatCompletionsRequest(req)
	if err != nil || !strings.Contains(string(conversion.Request.ToolChoice), "mcp__read") {
		t.Fatalf("choice = %s, %v", conversion.Request.ToolChoice, err)
	}
	req.ToolChoice = json.RawMessage(`{"type":"function","namespace":"mcp","name":"missing"}`)
	if _, err = ResponsesToChatCompletionsRequest(req); err == nil {
		t.Fatal("unresolved namespace choice accepted")
	}
}

func TestNamespaceToolUTF8TruncationIsValid(t *testing.T) {
	tools := []ResponsesTool{{Type: "namespace", Name: strings.Repeat("界", 20), Tools: []ResponsesTool{{Type: "function", Name: strings.Repeat("名", 20)}}}}
	conversion, err := ResponsesToChatCompletionsRequest(namespaceRequest(tools))
	if err != nil {
		t.Fatal(err)
	}
	name := conversion.Request.Tools[0].Function.Name
	if len(name) > 64 || !json.Valid([]byte(`"`+name+`"`)) {
		t.Fatalf("invalid flattened name %q", name)
	}
}

func TestNamespaceToolStreamingWire(t *testing.T) {
	state := NewChatCompletionsToResponsesStreamState("test")
	state.NamespaceTools = map[string]NamespacedToolName{"mcp__read": {Namespace: "mcp", Name: "read"}}
	idx := 0
	finish := "tool_calls"
	events := ChatCompletionsChunkToResponsesEvents(&ChatCompletionsChunk{Choices: []ChatChunkChoice{{Delta: ChatDelta{ToolCalls: []ChatToolCall{{Index: &idx, ID: "call_1", Function: ChatFunctionCall{Name: "mcp__read", Arguments: `{}`}}}}, FinishReason: &finish}}}, state)
	var found bool
	for _, event := range append(events, FinalizeChatCompletionsResponsesStream(state)...) {
		if event.Item != nil && event.Item.Type == "function_call" {
			wire := responsesItemWire(event.Item)
			if wire["name"] == "read" && wire["namespace"] == "mcp" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("stream did not emit restored namespace call")
	}
}

func TestNamespaceToolStreamingLateNameSparseInterleaved(t *testing.T) {
	state := NewChatCompletionsToResponsesStreamState("test")
	state.NamespaceTools = map[string]NamespacedToolName{"mcp__read": {Namespace: "mcp", Name: "read"}}
	i2, i7 := 2, 7
	first := ChatCompletionsChunkToResponsesEvents(&ChatCompletionsChunk{Choices: []ChatChunkChoice{{Delta: ChatDelta{ToolCalls: []ChatToolCall{{Index: &i7, ID: "c7", Function: ChatFunctionCall{Arguments: `{"a":`}}, {Index: &i2, ID: "c2", Function: ChatFunctionCall{Name: "plain", Arguments: `{}`}}}}}}}, state)
	for _, event := range first {
		if event.Item != nil && event.Item.CallID == "c7" {
			t.Fatal("late-name call announced before its name")
		}
	}
	second := ChatCompletionsChunkToResponsesEvents(&ChatCompletionsChunk{Choices: []ChatChunkChoice{{Delta: ChatDelta{ToolCalls: []ChatToolCall{{Index: &i7, Function: ChatFunctionCall{Name: "mcp__read", Arguments: `1}`}}}}}}}, state)
	all := append(append(first, second...), FinalizeChatCompletionsResponsesStream(state)...)
	seen := map[string]bool{}
	for _, event := range all {
		if event.Item != nil && event.Item.Type == "function_call" {
			seen[event.Item.CallID+":"+event.Item.Name] = true
		}
	}
	if !seen["c2:plain"] || !seen["c7:read"] {
		t.Fatalf("calls = %#v", seen)
	}
}
