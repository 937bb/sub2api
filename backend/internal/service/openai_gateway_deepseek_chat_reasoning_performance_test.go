package service

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Keep the previous algorithm as a compatibility oracle and benchmark baseline.
func legacyDeepSeekReasoningPlaceholders(account *Account, body []byte) []byte {
	if !requiresDeepSeekChatReasoning(account, body) {
		return body
	}
	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() {
		return body
	}
	updated := body
	for i, msg := range messages.Array() {
		if strings.TrimSpace(msg.Get("role").String()) != "assistant" || msg.Get("reasoning_content").String() != "" {
			continue
		}
		next, err := sjson.SetBytes(updated, "messages."+strconv.Itoa(i)+".reasoning_content", deepSeekChatReasoningPlaceholderText)
		if err != nil {
			return body
		}
		updated = next
	}
	return updated
}

func TestDeepSeekReasoningPreservesRawRequest(t *testing.T) {
	account := &Account{Platform: PlatformDeepseek, Type: AccountTypeAPIKey}
	messageCases := []string{
		`{"role":"assistant","content":"hello"}`,
		`{ "role" : "assistant", "reasoning_content" : "", "content": "escaped \\" }`,
		`{"role":"assistant","reasoning_content":null,"unknown":9007199254740993}`,
		`{"role":" assistant ","content":null,"tool_calls":[{"id":"call_1","function":{"name":"exec","arguments":"{\"x\":1}"}}]}`,
		`{"role":"assistant","reasoning_content":"real thinking","content":"ok"}`,
		`{"role":"assistant","reasoning_content":" ","content":"ok"}`,
		`{"role":"assistant","reasoning_content":0,"content":"ok"}`,
		`{"role":"assistant","reasoning_content":false,"content":"ok"}`,
		`{"role":"assistant","reasoning_content":[],"content":"ok"}`,
		`{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,aGVsbG8="}}]}`,
		`{"role":"tool","tool_call_id":"call_1","content":"你好 } ] \""}`,
		`{"nested":{"role":"assistant"},"content":"keep"}`,
		`null`, `5`, `"assistant"`, `[]`,
	}
	for i, message := range messageCases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			body := []byte(" \n{\"unknown\":9007199254740993,\"messages\" : [\n" + message + ",\n{\"role\":\"assistant\",\"content\":\"second\"}\n], \"model\":\"deepseek-v4.1-flash\",\"metadata\":{\"raw\":1.2300e+10}} \n")
			require.True(t, gjson.ValidBytes(body))
			original := bytes.Clone(body)
			want := legacyDeepSeekReasoningPlaceholders(account, body)
			got := ensureDeepSeekChatReasoningPlaceholders(account, body)
			require.Equal(t, string(want), string(got))
			require.Equal(t, original, body, "the caller's request must not be mutated")
			require.Equal(t, got, ensureDeepSeekChatReasoningPlaceholders(account, got))
		})
	}
	for _, body := range []string{`{}`, `{"messages":null}`, `{"messages":{}}`, `{"messages":[]}`, `{"messages":[{"role":"assistant","reasoning_content":"cached"}]}`} {
		input := []byte(body)
		got := ensureDeepSeekChatReasoningPlaceholders(account, input)
		require.Equal(t, body, string(got))
		require.True(t, &input[0] == &got[0], "unchanged requests should reuse the original buffer")
	}
}

func deepSeekBenchmarkHistory(turns, contentBytes int) []byte {
	var body strings.Builder
	_, _ = body.WriteString(`{"model":"deepseek-v4.1-flash","messages":[{"role":"system","content":"system"}`)
	content := strings.Repeat("x", contentBytes)
	for range turns {
		fmt.Fprintf(&body, `,{"role":"user","content":"next"},{"role":"assistant","content":"%s"}`, content)
	}
	_, _ = body.WriteString(`],"stream":true,"unknown":9007199254740993}`)
	return []byte(body.String())
}

func TestDeepSeekReasoningLongHistoryMatchesLegacy(t *testing.T) {
	account := &Account{Platform: PlatformDeepseek, Type: AccountTypeAPIKey}
	for _, turns := range []int{1, 16, 128, 512} {
		body := deepSeekBenchmarkHistory(turns, 1024)
		require.Equal(t, legacyDeepSeekReasoningPlaceholders(account, body), ensureDeepSeekChatReasoningPlaceholders(account, body))
	}
}

func BenchmarkDeepSeekReasoningPlaceholders(b *testing.B) {
	account := &Account{Platform: PlatformDeepseek, Type: AccountTypeAPIKey}
	for _, turns := range []int{16, 128, 512} {
		body := deepSeekBenchmarkHistory(turns, 1024)
		for _, implementation := range []struct {
			name string
			run  func(*Account, []byte) []byte
		}{{"legacy", legacyDeepSeekReasoningPlaceholders}, {"linear", ensureDeepSeekChatReasoningPlaceholders}} {
			b.Run(fmt.Sprintf("%d_turns/%s", turns, implementation.name), func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(body)))
				for b.Loop() {
					if len(implementation.run(account, body)) <= len(body) {
						b.Fatal("missing placeholders")
					}
				}
			})
		}
	}
}
