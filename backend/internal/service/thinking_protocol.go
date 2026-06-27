package service

import "strings"

// ThinkingProtocol describes how an upstream treats historical Anthropic thinking blocks.
type ThinkingProtocol int

const (
	ThinkingProtocolUnknown ThinkingProtocol = iota
	// Anthropic-strict upstreams require valid thinking signatures; invalid blocks may be stripped.
	ThinkingProtocolAnthropicStrict
	// Passback-required upstreams require thinking blocks to round-trip unchanged.
	ThinkingProtocolPassbackRequired
)

// ResolveThinkingProtocol classifies the model ID used as the protocol reference.
// Anthropic gateway callers should pass the mapped upstream model; Gemini messages
// compat passes the original Anthropic model because the body being retried is Anthropic-shaped.
func ResolveThinkingProtocol(modelID string) ThinkingProtocol {
	id := strings.ToLower(strings.TrimSpace(modelID))
	if id == "" {
		return ThinkingProtocolUnknown
	}

	switch {
	case strings.HasPrefix(id, "deepseek-"),
		strings.HasPrefix(id, "kimi-"),
		strings.HasPrefix(id, "moonshot-"),
		strings.HasPrefix(id, "glm-"):
		return ThinkingProtocolPassbackRequired
	}
	if strings.HasPrefix(id, "minimax-m") {
		return ThinkingProtocolPassbackRequired
	}
	if (strings.HasPrefix(id, "qwen-") ||
		strings.HasPrefix(id, "qwen2-") ||
		strings.HasPrefix(id, "qwen3-") ||
		strings.HasPrefix(id, "qwen4-")) && strings.Contains(id, "-thinking") {
		return ThinkingProtocolPassbackRequired
	}

	switch {
	case strings.HasPrefix(id, "claude-"),
		strings.HasPrefix(id, "opus-"),
		strings.HasPrefix(id, "sonnet-"),
		strings.HasPrefix(id, "haiku-"):
		return ThinkingProtocolAnthropicStrict
	}

	return ThinkingProtocolUnknown
}

func ShouldPreFilterThinkingBlocks(modelID string) bool {
	return ResolveThinkingProtocol(modelID) == ThinkingProtocolAnthropicStrict
}

func ShouldRectifyThinkingSignatureError(modelID string) bool {
	return ResolveThinkingProtocol(modelID) == ThinkingProtocolAnthropicStrict
}

func ShouldApplyRetryFilters(modelID string) bool {
	return ResolveThinkingProtocol(modelID) == ThinkingProtocolAnthropicStrict
}
