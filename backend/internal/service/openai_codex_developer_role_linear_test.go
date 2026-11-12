package service

import (
	"bytes"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"strings"
	"testing"
)

func TestCodexDeveloperMessages_LinearEquivalent(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	for _, body := range []string{
		` { "input" : [{"content":"世界", "role" : "user"}, {"role":"u\u0073er","content":"two"}], "unknown":900719925474099312345 } `,
		`{"input":[{"\u0072ole":"user","content":"escaped key"},{"role":"user","role":"assistant","content":"duplicate role"}]}`,
		`{"input":[{"role":"user","content":"first"}],"input":[{"role":"user","content":"second"}]}`,
		`{"input":[{"role":"\u0075\u0073\u0065\u0072","content":[{"type":"input_text","text":"x"}]}]}`,
		`{"input":[{"content":"role user","role":"assistant","role":"user"},null,1,"str",{"role":"user","content":"yes","type":"message"}]}`,
		`{"type":"response.create","input":"string input"}`,
		`{"input":[{"role":"user","content":[{"type":"input_image","image_url":"unchanged"}]}]}`,
	} {
		input := []byte(body)
		original := bytes.Clone(input)
		want, wantChanged, wantErr := normalizeCodexDeveloperMessagesReference(input, account)
		got, changed, err := normalizeCodexDeveloperMessages(input, account)
		require.Equal(t, wantErr, err, body)
		require.Equal(t, wantChanged, changed, body)
		require.Equal(t, string(want), string(got), body)
		require.Equal(t, original, input)
	}
}

func FuzzCodexDeveloperMessages_LinearEquivalent(f *testing.F) {
	for _, s := range []string{`{"input":[{"role":"user","content":"x"}]}`, ` {"input":[{"role":"u\u0073er","content":"世界"}]} `, `{"input":"hello"}`, `{"input":null}`, `{"input":[{"role":"user","tool_call_id":"a","content":"x"}]}`} {
		f.Add(s)
	}
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 1<<20 {
			t.Skip()
		}
		body := []byte(s)
		original := bytes.Clone(body)
		want, wc, we := normalizeCodexDeveloperMessagesReference(body, account)
		got, gc, ge := normalizeCodexDeveloperMessages(body, account)
		require.Equal(t, we == nil, ge == nil)
		require.Equal(t, wc, gc)
		require.Equal(t, want, got)
		require.Equal(t, original, body)
	})
}

func BenchmarkCodexDeveloperMessagesLongHistory(b *testing.B) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	for _, turns := range []int{16, 128, 512} {
		item := `{"role":"user","content":"` + strings.Repeat("safe text ", 400) + `"}`
		body := []byte(`{"model":"example","input":[` + strings.TrimSuffix(strings.Repeat(item+",", turns), ",") + `],"store":false}`)
		for _, old := range []bool{true, false} {
			label := "linear"
			if old {
				label = "previous"
			}
			b.Run(fmt.Sprintf("%d/%s", turns, label), func(b *testing.B) {
				b.SetBytes(int64(len(body)))
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					var err error
					if old {
						_, _, err = normalizeCodexDeveloperMessagesReference(body, account)
					} else {
						_, _, err = normalizeCodexDeveloperMessages(body, account)
					}
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

// Preserve the former algorithm as a differential oracle and benchmark baseline.
func normalizeCodexDeveloperMessagesReference(body []byte, account *Account) ([]byte, bool, error) {
	if !usesCodexDeveloperMessages(account) || len(body) == 0 {
		return body, false, nil
	}
	root := gjson.ParseBytes(body)
	if !root.IsObject() || !gjson.ValidBytes(body) {
		return body, false, fmt.Errorf("normalize Codex developer messages: invalid JSON object")
	}
	if event := root.Get("type"); event.Exists() && event.String() != "response.create" {
		return body, false, nil
	}
	input := root.Get("input")
	if input.Type == gjson.String && strings.TrimSpace(input.String()) != "" {
		next, err := sjson.SetRawBytes(body, "input", []byte(`[{"type":"message","role":"developer","content":`+input.Raw+`}]`))
		return next, err == nil, err
	}
	if !input.IsArray() {
		return body, false, nil
	}
	next := body
	changed := false
	for i, item := range input.Array() {
		kind := item.Get("type")
		if !item.IsObject() || item.Get("role").String() != "user" ||
			(kind.Exists() && kind.String() != "message") ||
			item.Get("tool_call_id").Exists() || item.Get("tool_calls").Exists() ||
			!codexDeveloperTextContent(item.Get("content")) {
			continue
		}
		var err error
		next, err = sjson.SetBytes(next, fmt.Sprintf("input.%d.role", i), "developer")
		if err != nil {
			return body, false, fmt.Errorf("normalize Codex developer message role: %w", err)
		}
		changed = true
	}
	return next, changed, nil
}
