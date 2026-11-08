package apicompat

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAnthropicNativeTerminalParity(t *testing.T) {
	for _, tc := range []struct{ stop, status, reason, finish string }{
		{"refusal", "incomplete", "content_filter", "content_filter"},
		{"max_tokens", "incomplete", "max_output_tokens", "length"},
		{"model_context_window_exceeded", "incomplete", "max_output_tokens", "length"},
		{"stop_sequence", "completed", "", "tool_calls"},
		{"tool_use", "completed", "", "tool_calls"},
	} {
		t.Run(tc.stop, func(t *testing.T) {
			original := &AnthropicResponse{ID: "msg_test", Model: "claude-haiku-4.5", Role: "assistant", Type: "message", StopReason: AnthropicStopReasonPtr(tc.stop), Content: []AnthropicContentBlock{{Type: "tool_use", ID: "call1", Name: "read_probe", Input: json.RawMessage(`{}`)}}}
			converted := AnthropicToResponsesResponse(original)
			require.Equal(t, tc.status, converted.Status)
			if tc.reason != "" {
				require.Equal(t, tc.reason, converted.IncompleteDetails.Reason)
			} else {
				require.Nil(t, converted.IncompleteDetails)
			}
			chat := ResponsesToChatCompletions(converted, original.Model)
			require.Equal(t, tc.finish, chat.Choices[0].FinishReason)
			state := NewAnthropicEventToResponsesState()
			for _, raw := range []string{
				`{"type":"message_start","message":{"id":"msg_test","model":"claude-haiku-4.5","role":"assistant","content":[],"usage":{"input_tokens":0,"output_tokens":0}}}`,
				`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"call1","name":"read_probe","input":{}}}`,
				`{"type":"content_block_stop","index":0}`,
				fmt.Sprintf(`{"type":"message_delta","delta":{"stop_reason":%q},"usage":{"input_tokens":0,"output_tokens":0}}`, tc.stop),
			} {
				var event AnthropicStreamEvent
				require.NoError(t, json.Unmarshal([]byte(raw), &event))
				AnthropicEventToResponsesEvents(&event, state)
			}
			events := AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: "message_stop"}, state)
			require.NotEmpty(t, events)
			terminal := events[len(events)-1]
			require.Equal(t, "response."+tc.status, terminal.Type)
			require.Equal(t, converted.IncompleteDetails, terminal.Response.IncompleteDetails)
			require.Empty(t, AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: "message_stop"}, state), "one terminal only")
		})
	}
}
