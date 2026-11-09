package kiro

import "encoding/json"

// Kiro signatures are opaque. This explicit compatibility marker is used only
// when the provider omits one; it is never presented as an Anthropic signature
// or forwarded as a continuation credential to either provider.
const thinkingSignaturePlaceholder = "sub2api-kiro-thinking-signature"

type DecodeOptions struct {
	ThinkingEnabled bool
	OmitThinking    bool
}

func DecodeOptionsForRequest(raw []byte) DecodeOptions {
	var request struct {
		Thinking struct {
			Type    string `json:"type"`
			Display string `json:"display"`
		} `json:"thinking"`
	}
	if json.Unmarshal(raw, &request) != nil {
		return DecodeOptions{}
	}
	return DecodeOptions{
		ThinkingEnabled: request.Thinking.Type == "enabled" || request.Thinking.Type == "adaptive" || request.Thinking.Type == "between_tools",
		OmitThinking:    request.Thinking.Display == "omitted",
	}
}

func estimatedOutputTokens(blocks []Block) int {
	tokens := 0
	for _, block := range blocks {
		switch block.Type {
		case "thinking":
			tokens += EstimateTokens(block.Thinking)
		case "redacted_thinking":
			// Match rs's fixed estimate; ciphertext length is not a token count.
			tokens += 8
		default:
			tokens += EstimateTokens(block.Text + string(block.Input))
		}
	}
	return tokens
}
