package service

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestComputeClaudeCodeFingerprintMatchesJSStringIndexing(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "ascii",
			text: "hello world this is enough",
			want: "338",
		},
		{
			name: "bmp unicode",
			text: "abcd界fg中文0123456789tail",
			want: "08b",
		},
		{
			name: "surrogate pair uses js code unit",
			text: "abcd😀fg中文0123456789tail",
			want: "559",
		},
		{
			name: "short text pads zero",
			text: "short",
			want: "fa9",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			textJSON, err := json.Marshal(tt.text)
			if err != nil {
				t.Fatal(err)
			}
			body := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":` + string(textJSON) + `}]}]}`)
			if got := computeClaudeCodeFingerprint(body, "2.1.161"); got != tt.want {
				t.Fatalf("computeClaudeCodeFingerprint() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildBillingAttributionBlockText_DefaultShapeMatchesCurrentCLI(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"billing shape"}]}]}`)

	text, err := buildBillingAttributionBlockText(body, "2.1.181")
	if err != nil {
		t.Fatal(err)
	}

	if want := "x-anthropic-billing-header: cc_version=2.1.181."; !strings.HasPrefix(text, want) {
		t.Fatalf("billing text = %q, want prefix %q", text, want)
	}
	if !strings.Contains(text, "cc_entrypoint=sdk-cli;") {
		t.Fatalf("billing text = %q, want sdk-cli entrypoint", text)
	}
	if strings.Contains(text, "cch=") {
		t.Fatalf("billing text = %q, current CLI shape should not include cch", text)
	}
}
