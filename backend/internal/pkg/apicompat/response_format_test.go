package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponseFormatCompatibility(t *testing.T) {
	tests := []struct{ name, direction, input, want string }{
		{"chat json object unchanged", "chat", `{"type":"json_object","x":1e+09}`, `{"type":"json_object","x":1e+09}`},
		{"responses json object unchanged", "responses", `{"type":"json_object","x":1e+09}`, `{"type":"json_object","x":1e+09}`},
		{"chat schema preserves wire values", "chat", `{"type":"json_schema","json_schema":{"name":"n","schema":{"x":1e+09,"x":2.00}}}`, `{"type":"json_schema","name":"n","schema":{"x":1e+09,"x":2.00}}`},
		{"responses schema preserves wire values", "responses", `{"type":"json_schema","name":"n","schema":{"x":1e+09,"x":2.00}}`, `{"type":"json_schema","json_schema":{"name":"n","schema":{"x":1e+09,"x":2.00}}}`},
		{"malformed unchanged", "chat", `{"type":"json_schema",`, `{"type":"json_schema",`},
		{"unknown unchanged", "chat", `{"type":"future","n":1e2}`, `{"type":"future","n":1e2}`},
		{"duplicate type unchanged", "chat", `{"type":"json_schema","type":"json_schema","json_schema":{"name":"n"}}`, `{"type":"json_schema","type":"json_schema","json_schema":{"name":"n"}}`},
		{"duplicate wrapper unchanged", "chat", `{"type":"json_schema","json_schema":{"name":"a"},"json_schema":{"name":"b"}}`, `{"type":"json_schema","json_schema":{"name":"a"},"json_schema":{"name":"b"}}`},
		{"inner type collision unchanged", "chat", `{"type":"json_schema","json_schema":{"type":"object","name":"n"}}`, `{"type":"json_schema","json_schema":{"type":"object","name":"n"}}`},
		{"responses wrapper collision unchanged", "responses", `{"type":"json_schema","json_schema":{"name":"n"},"name":"outer"}`, `{"type":"json_schema","json_schema":{"name":"n"},"name":"outer"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got json.RawMessage
			if tt.direction == "chat" {
				got = chatResponseFormatToResponsesTextFormat(json.RawMessage(tt.input))
			} else {
				got = responsesTextFormatToChatResponseFormat(json.RawMessage(tt.input))
			}
			assert.Equal(t, tt.want, string(got))
		})
	}
	assert.Nil(t, chatResponseFormatToResponsesTextFormat(json.RawMessage(`null`)))
	assert.Nil(t, responsesTextFormatToChatResponseFormat(nil))
}

func TestResponseFormatCompatibilityWiredBothDirections(t *testing.T) {
	chat, err := ChatCompletionsToResponses(&ChatCompletionsRequest{Model: "gpt-4o", Messages: []ChatMessage{{Role: "user", Content: json.RawMessage(`"json"`)}}, ResponseFormat: json.RawMessage(`{"type":"json_schema","json_schema":{"name":"n","schema":{"n":1e+09}}}`)})
	require.NoError(t, err)
	require.NotNil(t, chat.Text)
	assert.Equal(t, `{"type":"json_schema","name":"n","schema":{"n":1e+09}}`, string(chat.Text.Format))

	back, err := ResponsesToChatCompletionsRequest(&ResponsesRequest{Model: "gpt-4o", Input: json.RawMessage(`"json"`), Text: &ResponsesText{Format: chat.Text.Format}})
	require.NoError(t, err)
	assert.Equal(t, `{"type":"json_schema","json_schema":{"name":"n","schema":{"n":1e+09}}}`, string(back.ResponseFormat))
}
