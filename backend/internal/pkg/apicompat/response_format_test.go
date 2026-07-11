package apicompat

import (
	"encoding/json"
	"fmt"
	"strings"
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

func TestResponseFormatMalformedJSONPassesThrough(t *testing.T) {
	inputs := []string{
		`{"type":"json_schema","json_schema":{"x":"\q"}}`,
		`{"type":"json_schema","json_schema":{"x":"\u12"}}`,
		`{"type":"json_schema","json_schema":{"x":01}}`,
		`{"type":"json_schema","json_schema":{"x":1.}}`,
		`{"type":"json_schema","json_schema":{"x":1e+}}`,
		`{"type":"json_schema","json_schema":{"x":true false}}`,
		`{"type":"json_schema","json_schema":{"x":[1,]}}`,
		`{"type":"json_schema","json_schema":{"x":{"y":1,}}}`,
		`{"type":"json_schema","json_schema":{},}`,
		`{"type":"json_schema","json_schema":{"x":[1}}}`,
		`{"type":"json_schema","json_schema":{}} trailing`,
	}
	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			assert.Equal(t, input, string(chatResponseFormatToResponsesTextFormat(json.RawMessage(input))))
		})
	}
}

func TestResponseFormatAdversarialPreservation(t *testing.T) {
	input := ` { "type" : "json_schema" , "json_schema" : {"name":"a\\\"b","schema":{"n":-0.00E-09,"dup":1,"dup":2,"nested":[true,false,null,{"x":"\uD800"}]}} } `
	want := `{"type":"json_schema","name":"a\\\"b","schema":{"n":-0.00E-09,"dup":1,"dup":2,"nested":[true,false,null,{"x":"\uD800"}]}}`
	assert.Equal(t, want, string(chatResponseFormatToResponsesTextFormat(json.RawMessage(input))))

	deep := `{"type":"json_schema","json_schema":{"schema":` + strings.Repeat(`[`, 1000) + `0` + strings.Repeat(`]`, 1000) + `}}`
	got := chatResponseFormatToResponsesTextFormat(json.RawMessage(deep))
	assert.Contains(t, string(got), strings.Repeat(`[`, 1000)+`0`)
}

func TestScanJSONValueDepthLimit(t *testing.T) {
	for _, depth := range []int{9999, 10000, 10001} {
		t.Run(fmt.Sprintf("depth_%d", depth), func(t *testing.T) {
			raw := []byte(strings.Repeat(`[`, depth) + `0` + strings.Repeat(`]`, depth))
			end, ok := scanJSONValue(raw, 0, 0)
			if depth <= maxJSONNestingDepth {
				assert.True(t, ok)
				assert.Equal(t, len(raw), end)
			} else {
				assert.False(t, ok)
			}
		})
	}
}

func TestResponseFormatGlobalDepthLimit(t *testing.T) {
	for _, depth := range []int{9998, 9999} {
		t.Run(fmt.Sprintf("schema_depth_%d", depth), func(t *testing.T) {
			input := `{"type":"json_schema","json_schema":{"schema":` + strings.Repeat(`[`, depth) + `0` + strings.Repeat(`]`, depth) + `}}`
			got := string(chatResponseFormatToResponsesTextFormat(json.RawMessage(input)))
			if depth == 9998 {
				assert.NotEqual(t, input, got)
				assert.True(t, json.Valid([]byte(got)))
			} else {
				assert.False(t, json.Valid([]byte(input)))
				assert.Equal(t, input, got)
			}
		})
	}
}

func FuzzScanJSONValueValidity(f *testing.F) {
	for _, seed := range [][]byte{
		[]byte(`null`), []byte(`-0.00E-09`), []byte(`{"x":[true,false,null]}`),
		[]byte(`{"x":"\uD800"}`), []byte(`{"x":"\q"}`), []byte(`[1,]`),
		[]byte(`{"x":01}`), []byte("\xff"), []byte("\"\xff\""),
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		end, ok := scanJSONValue(raw, 0, 0)
		got := ok && skipJSONSpace(raw, end) == len(raw)
		assert.Equal(t, json.Valid(raw), got)
	})
}

func BenchmarkResponseFormatCompatibility(b *testing.B) {
	chat := json.RawMessage(`{"type":"json_schema","json_schema":{"name":"invoice","strict":true,"schema":{"type":"object","properties":{"amount":{"type":"number","examples":[1e+09,2.00,-0.5E-2]},"currency":{"type":"string"}},"required":["amount","currency"],"additionalProperties":false}}}`)
	responses := json.RawMessage(`{"type":"json_schema","name":"invoice","strict":true,"schema":{"type":"object","properties":{"amount":{"type":"number","examples":[1e+09,2.00,-0.5E-2]},"currency":{"type":"string"}},"required":["amount","currency"],"additionalProperties":false}}`)
	b.Run("chat_to_responses", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = chatResponseFormatToResponsesTextFormat(chat)
		}
	})
	b.Run("responses_to_chat", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = responsesTextFormatToChatResponseFormat(responses)
		}
	})
}

func BenchmarkResponseFormatLargeSchema(b *testing.B) {
	for _, properties := range []int{100, 1000, 10000} {
		var schema strings.Builder
		schema.WriteString(`{"type":"object","properties":{`)
		for i := 0; i < properties; i++ {
			if i > 0 {
				schema.WriteByte(',')
			}
			fmt.Fprintf(&schema, `"field_%d":{"type":"string"}`, i)
		}
		schema.WriteString(`}}`)

		chat := json.RawMessage(`{"type":"json_schema","json_schema":{"name":"large","schema":` + schema.String() + `}}`)
		responses := json.RawMessage(`{"type":"json_schema","name":"large","schema":` + schema.String() + `}`)
		b.Run(fmt.Sprintf("properties_%d/chat_to_responses", properties), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(chat)))
			for i := 0; i < b.N; i++ {
				_ = chatResponseFormatToResponsesTextFormat(chat)
			}
		})
		b.Run(fmt.Sprintf("properties_%d/responses_to_chat", properties), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(responses)))
			for i := 0; i < b.N; i++ {
				_ = responsesTextFormatToChatResponseFormat(responses)
			}
		})
	}
}

func TestResponseFormatCompatibilityWiredBothDirections(t *testing.T) {
	chat, err := ChatCompletionsToResponses(&ChatCompletionsRequest{Model: "gpt-4o", Messages: []ChatMessage{{Role: "user", Content: json.RawMessage(`"json"`)}}, ResponseFormat: json.RawMessage(`{"type":"json_schema","json_schema":{"name":"n","schema":{"n":1e+09}}}`)})
	require.NoError(t, err)
	require.NotNil(t, chat.Text)
	assert.Equal(t, `{"type":"json_schema","name":"n","schema":{"n":1e+09}}`, string(chat.Text.Format))

	conversion, err := ResponsesToChatCompletionsRequest(&ResponsesRequest{Model: "gpt-4o", Input: json.RawMessage(`"json"`), Text: &ResponsesText{Format: chat.Text.Format}})
	require.NoError(t, err)
	assert.Equal(t, `{"type":"json_schema","json_schema":{"name":"n","schema":{"n":1e+09}}}`, string(conversion.Request.ResponseFormat))
}
