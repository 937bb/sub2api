package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// A client replays the tool identity returned by this bridge in its next turn.
// Some compatible upstreams supply that identity only in the completed item.
func TestResponsesToChat_LateToolIdentityRoundTrip(t *testing.T) {
	for _, terminalOnly := range []bool{false, true} {
		name := "item_done"
		if terminalOnly {
			name = "response_completed"
		}
		t.Run(name, func(t *testing.T) {
			state := NewResponsesEventToChatState()
			var chunks []ChatCompletionsChunk
			emit := func(event ResponsesStreamEvent) {
				chunks = append(chunks, ResponsesEventToChatChunks(&event, state)...)
			}
			emit(ResponsesStreamEvent{Type: "response.output_item.added", OutputIndex: 0,
				Item: &ResponsesOutput{Type: "function_call", ID: "fc_item"}})
			emit(ResponsesStreamEvent{Type: "response.function_call_arguments.delta", OutputIndex: 0, Delta: `{"path":`})
			completed := ResponsesOutput{Type: "function_call", ID: "fc_item", CallID: "call_actual", Name: "read_file", Arguments: `{"path":"test.txt"}`}
			if !terminalOnly {
				emit(ResponsesStreamEvent{Type: "response.output_item.done", OutputIndex: 0, Item: &completed})
			}
			emit(ResponsesStreamEvent{Type: "response.completed", Response: &ResponsesResponse{Status: "completed", Output: []ResponsesOutput{completed}}})
			calls := collectCallIDTestDeltas(t, chunks)
			require.Len(t, calls, 1)
			require.Equal(t, "call_actual", calls[0].ID)
			require.Equal(t, "read_file", calls[0].Function.Name)
			require.JSONEq(t, completed.Arguments, calls[0].Function.Arguments)

			history := make([]ChatMessage, 10)
			for i := range history {
				history[i] = ChatMessage{Role: "user", Content: json.RawMessage(`"synthetic history"`)}
			}
			history = append(history, ChatMessage{Role: "assistant", ToolCalls: calls},
				ChatMessage{Role: "tool", ToolCallID: calls[0].ID, Content: json.RawMessage(`"synthetic result"`)})
			input, err := convertChatMessagesToResponsesInput(history)
			require.NoError(t, err)
			require.Equal(t, "function_call", input[10].Type)
			require.Equal(t, "call_actual", input[10].CallID)
			require.Equal(t, input[10].CallID, input[11].CallID)
		})
	}
}

func TestResponsesToChat_ToolSnapshotsPreserveParallelIdentities(t *testing.T) {
	state := NewResponsesEventToChatState()
	var chunks []ChatCompletionsChunk
	first := ResponsesOutput{Type: "function_call", CallID: "call_first", Name: "read_file", Arguments: `{"path":"a"}`}
	second := ResponsesOutput{Type: "function_call", CallID: "call_second", Name: "read_file", Arguments: `{"path":"b"}`}
	// Both calls have the same name and finish in reverse order. Pair by index.
	for _, event := range []ResponsesStreamEvent{
		{Type: "response.output_item.added", OutputIndex: 1, Item: &ResponsesOutput{Type: "function_call", CallID: first.CallID, Name: first.Name}},
		{Type: "response.output_item.added", OutputIndex: 2, Item: &ResponsesOutput{Type: "function_call"}},
		{Type: "response.output_item.done", OutputIndex: 2, Item: &second},
		{Type: "response.output_item.done", OutputIndex: 1, Item: &first},
		{Type: "response.completed", Response: &ResponsesResponse{Status: "completed", Output: []ResponsesOutput{{Type: "reasoning"}, first, second}}},
	} {
		chunks = append(chunks, ResponsesEventToChatChunks(&event, state)...)
	}
	calls := collectCallIDTestDeltas(t, chunks)
	require.Len(t, calls, 2)
	require.Equal(t, first.CallID, calls[0].ID)
	require.Equal(t, second.CallID, calls[1].ID)
	require.Equal(t, first.Name, calls[0].Function.Name)
	require.Equal(t, second.Name, calls[1].Function.Name)
	require.JSONEq(t, first.Arguments, calls[0].Function.Arguments)
	require.JSONEq(t, second.Arguments, calls[1].Function.Arguments)
}

func TestBufferedResponseAccumulator_LateToolIdentity(t *testing.T) {
	acc := NewBufferedResponseAccumulator()
	acc.ProcessEvent(&ResponsesStreamEvent{Type: "response.output_item.added", OutputIndex: 1,
		Item: &ResponsesOutput{Type: "function_call"}})
	acc.ProcessEvent(&ResponsesStreamEvent{Type: "response.function_call_arguments.delta", OutputIndex: 1, Delta: `{"path":`})
	acc.ProcessEvent(&ResponsesStreamEvent{Type: "response.output_item.done", OutputIndex: 1,
		Item: &ResponsesOutput{Type: "function_call", CallID: "call_buffered", Name: "read_file", Arguments: `{"path":"test.txt"}`}})
	resp := &ResponsesResponse{Status: "completed"}
	acc.SupplementResponseOutput(resp)
	chat := ResponsesToChatCompletions(resp, "test-model")
	require.Len(t, chat.Choices[0].Message.ToolCalls, 1)
	call := chat.Choices[0].Message.ToolCalls[0]
	require.Equal(t, "call_buffered", call.ID)
	require.Equal(t, "read_file", call.Function.Name)
	require.JSONEq(t, `{"path":"test.txt"}`, call.Function.Arguments)
}

func TestResponsesToChat_CustomToolIdentityAndInputFromDone(t *testing.T) {
	state := NewResponsesEventToChatState()
	completed := ResponsesOutput{Type: "custom_tool_call", CallID: "call_custom", Name: "apply_patch", Input: "synthetic patch"}
	var chunks []ChatCompletionsChunk
	for _, event := range []ResponsesStreamEvent{
		{Type: "response.output_item.added", OutputIndex: 0, Item: &ResponsesOutput{Type: "custom_tool_call"}},
		{Type: "response.custom_tool_call_input.delta", OutputIndex: 0, Delta: "synthetic "},
		{Type: "response.output_item.done", OutputIndex: 0, Item: &completed},
		{Type: "response.completed", Response: &ResponsesResponse{Status: "completed", Output: []ResponsesOutput{completed}}},
	} {
		chunks = append(chunks, ResponsesEventToChatChunks(&event, state)...)
	}
	calls := collectCallIDTestDeltas(t, chunks)
	require.Len(t, calls, 1)
	require.Equal(t, completed.CallID, calls[0].ID)
	require.Equal(t, completed.Name, calls[0].Function.Name)
	require.Equal(t, completed.Input, calls[0].Function.Arguments)

	// Repeated terminal events must not append a second identity or finish.
	require.Empty(t, ResponsesEventToChatChunks(&ResponsesStreamEvent{Type: "response.done",
		Response: &ResponsesResponse{Status: "completed", Output: []ResponsesOutput{completed}}}, state))
}

func TestResponsesToChat_DoesNotInventMissingCallID(t *testing.T) {
	state := NewResponsesEventToChatState()
	chunks := ResponsesEventToChatChunks(&ResponsesStreamEvent{Type: "response.output_item.done", OutputIndex: 0,
		Item: &ResponsesOutput{Type: "function_call", ID: "fc_not_a_call_id", Name: "read_file", Arguments: `{}`}}, state)
	calls := collectCallIDTestDeltas(t, chunks)
	require.Len(t, calls, 1)
	require.Empty(t, calls[0].ID, "an item ID is not a substitute for an upstream call ID")
}

func TestResponsesToChat_ConflictingSnapshotCannotMixToolCalls(t *testing.T) {
	state := NewResponsesEventToChatState()
	first := ResponsesOutput{Type: "function_call", CallID: "call_a", Name: "read_file", Arguments: `{"path":"a"}`}
	chunks := ResponsesEventToChatChunks(&ResponsesStreamEvent{Type: "response.output_item.added", OutputIndex: 0, Item: &first}, state)
	conflict := ResponsesOutput{Type: "function_call", CallID: "call_b", Name: "read_file", Arguments: `{"path":"b"}`}
	chunks = append(chunks, ResponsesEventToChatChunks(&ResponsesStreamEvent{Type: "response.output_item.done", OutputIndex: 0, Item: &conflict}, state)...)
	calls := collectCallIDTestDeltas(t, chunks)
	require.Len(t, calls, 1)
	require.Equal(t, first.CallID, calls[0].ID)
	require.Equal(t, first.Arguments, calls[0].Function.Arguments)
}

func TestBufferedResponseAccumulator_RepairsOnlyMatchingTerminalItem(t *testing.T) {
	for _, callID := range []string{"", "call_original", "call_different"} {
		t.Run("terminal_"+callID, func(t *testing.T) {
			acc := NewBufferedResponseAccumulator()
			acc.ProcessEvent(&ResponsesStreamEvent{Type: "response.output_item.added", OutputIndex: 0,
				Item: &ResponsesOutput{Type: "function_call", CallID: "call_original", Name: "read_file", Arguments: `{}`}})
			resp := &ResponsesResponse{Status: "completed", Output: []ResponsesOutput{{Type: "function_call", CallID: callID}}}
			acc.SupplementResponseOutput(resp)
			if callID == "call_different" {
				require.Equal(t, callID, resp.Output[0].CallID)
				require.Empty(t, resp.Output[0].Name)
				require.Empty(t, resp.Output[0].Arguments)
				return
			}
			require.Equal(t, "call_original", resp.Output[0].CallID)
			require.Equal(t, "read_file", resp.Output[0].Name)
			require.Equal(t, `{}`, resp.Output[0].Arguments)
		})
	}
}

func collectCallIDTestDeltas(t *testing.T, chunks []ChatCompletionsChunk) []ChatToolCall {
	t.Helper()
	var calls []ChatToolCall
	for _, chunk := range chunks {
		for _, choice := range chunk.Choices {
			for _, delta := range choice.Delta.ToolCalls {
				require.NotNil(t, delta.Index)
				index := *delta.Index
				for len(calls) <= index {
					calls = append(calls, ChatToolCall{Type: "function"})
				}
				// Chat clients concatenate delta fields, so duplicate IDs/names fail.
				calls[index].ID += delta.ID
				calls[index].Function.Name += delta.Function.Name
				calls[index].Function.Arguments += delta.Function.Arguments
			}
		}
	}
	return calls
}
