//go:build unit

package service

import "testing"

func TestResolveThinkingProtocol(t *testing.T) {
	tests := []struct {
		name    string
		modelID string
		want    ThinkingProtocol
	}{
		{"claude", "claude-sonnet-4-5", ThinkingProtocolAnthropicStrict},
		{"opus", "opus-4-5", ThinkingProtocolAnthropicStrict},
		{"haiku", "haiku-4-5", ThinkingProtocolAnthropicStrict},
		{"deepseek", "deepseek-v4-pro", ThinkingProtocolPassbackRequired},
		{"kimi", "kimi-coding-v2", ThinkingProtocolPassbackRequired},
		{"moonshot", "moonshot-v1-32k", ThinkingProtocolPassbackRequired},
		{"glm", "glm-5.1", ThinkingProtocolPassbackRequired},
		{"minimax", "MiniMax-M2", ThinkingProtocolPassbackRequired},
		{"qwen thinking", "qwen3-235b-a22b-thinking-2507", ThinkingProtocolPassbackRequired},
		{"qwen non-thinking", "qwen3-32b", ThinkingProtocolUnknown},
		{"unknown", "gpt-5.1", ThinkingProtocolUnknown},
		{"empty", "", ThinkingProtocolUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveThinkingProtocol(tt.modelID); got != tt.want {
				t.Fatalf("ResolveThinkingProtocol(%q)=%v, want %v", tt.modelID, got, tt.want)
			}
		})
	}
}

func TestThinkingProtocolGuards(t *testing.T) {
	if !ShouldPreFilterThinkingBlocks("claude-sonnet-4-5") {
		t.Fatal("anthropic strict should pre-filter")
	}
	if ShouldPreFilterThinkingBlocks("deepseek-v4-pro") {
		t.Fatal("passback-required should not pre-filter")
	}
	if ShouldRectifyThinkingSignatureError("gpt-5.1") {
		t.Fatal("unknown model should not rectify")
	}
	if ShouldApplyRetryFilters("kimi-coding") {
		t.Fatal("passback-required should not apply retry filters")
	}
}
