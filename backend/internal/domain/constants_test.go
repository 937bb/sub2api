package domain

import "testing"

func TestDefaultAntigravityModelMapping_ImageCompatibilityAliases(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"gemini-2.5-flash-image":         "gemini-2.5-flash-image",
		"gemini-2.5-flash-image-preview": "gemini-2.5-flash-image",
		"gemini-3.1-flash-image":         "gemini-3.1-flash-image",
		"gemini-3.1-flash-image-preview": "gemini-3.1-flash-image",
		"gemini-3-pro-image":             "gemini-3.1-flash-image",
		"gemini-3-pro-image-preview":     "gemini-3.1-flash-image",
	}

	for from, want := range cases {
		got, ok := DefaultAntigravityModelMapping[from]
		if !ok {
			t.Fatalf("expected mapping for %q to exist", from)
		}
		if got != want {
			t.Fatalf("unexpected mapping for %q: got %q want %q", from, got, want)
		}
	}
}

func TestDefaultAntigravityModelMapping_ContainsNewClaudeModels(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"claude-fable-5":  "claude-fable-5",
		"claude-opus-4-8": "claude-opus-4-8",
	}
	for from, want := range cases {
		got, ok := DefaultAntigravityModelMapping[from]
		if !ok {
			t.Fatalf("expected mapping for %q to exist", from)
		}
		if got != want {
			t.Fatalf("unexpected mapping for %q: got %q want %q", from, got, want)
		}
	}
}

func TestDefaultAntigravityModelMapping_Gemini31ProPublicAliases(t *testing.T) {
	t.Parallel()

	for _, model := range []string{"gemini-3.1-pro", "gemini-3.1-pro-high", "gemini-3.1-pro-preview"} {
		if got := DefaultAntigravityModelMapping[model]; got != AntigravityGemini31ProAgentModel {
			t.Fatalf("mapping for %q = %q, want %q", model, got, AntigravityGemini31ProAgentModel)
		}
	}
	if _, advertised := DefaultAntigravityModelMapping[AntigravityGemini31ProAgentModel]; advertised {
		t.Fatalf("internal route %q must not be a client-facing mapping key", AntigravityGemini31ProAgentModel)
	}
}

func TestDefaultBedrockModelMapping_ContainsNewClaudeModels(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"claude-fable-5":  "anthropic.claude-fable-5",
		"claude-opus-4-8": "us.anthropic.claude-opus-4-8-v1",
	}
	for from, want := range cases {
		got, ok := DefaultBedrockModelMapping[from]
		if !ok {
			t.Fatalf("expected Bedrock mapping for %q to exist", from)
		}
		if got != want {
			t.Fatalf("unexpected Bedrock mapping for %q: got %q want %q", from, got, want)
		}
	}
}
