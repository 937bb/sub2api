package openai

import (
	"strings"
	"testing"
)

func TestCodexSyntheticDefaultInstructionsForModel(t *testing.T) {
	latest := strings.TrimSpace(codexSyntheticInstructionsGPT55)
	if latest == "" {
		t.Fatal("embedded GPT-5.5 Codex synthetic instructions must not be empty")
	}
	codexDefault := strings.TrimSpace(DefaultInstructions)
	if codexDefault == "" {
		t.Fatal("embedded default Codex instructions must not be empty")
	}

	tests := []struct {
		name  string
		model string
		want  string
	}{
		{name: "gpt 5.5 uses latest", model: "gpt-5.5", want: latest},
		{name: "gpt 5.4 falls back to latest", model: "gpt-5.4", want: latest},
		{name: "gpt 5.3 falls back to latest", model: "gpt-5.3", want: latest},
		{name: "bare gpt 5 falls back to latest", model: "gpt-5", want: latest},
		{name: "unknown falls back to latest", model: "some-unknown-model", want: latest},
		{name: "empty model falls back to latest", model: "", want: latest},
		{name: "codex model uses existing default", model: "gpt-5.3-codex", want: codexDefault},
		{name: "codex spark uses existing default", model: "gpt-5.3-codex-spark", want: codexDefault},
		{name: "codex alias uses existing default", model: "gpt-5-codex", want: codexDefault},
		{name: "codex auto review uses existing default", model: "codex-auto-review", want: codexDefault},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strings.TrimSpace(CodexSyntheticDefaultInstructionsForModel(tt.model))
			if got == "" {
				t.Fatalf("CodexSyntheticDefaultInstructionsForModel(%q) returned empty instructions", tt.model)
			}
			if got != tt.want {
				t.Fatalf("CodexSyntheticDefaultInstructionsForModel(%q) mismatch:\ngot head:  %q\nwant head: %q", tt.model, firstLine(got), firstLine(tt.want))
			}
		})
	}
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
