package service

import (
	"fmt"

	"github.com/gin-gonic/gin"
)

const openAILegacyCompactNativeV2BridgeKey = "openai_legacy_compact_native_v2_bridge"

// shouldBridgeOpenAILegacyCompactToNativeV2 reports whether a legacy client
// request must use the current ChatGPT remote-compaction wire upstream. The
// legacy /responses/compact endpoint has been removed from the Codex backend,
// while API-key relays may still implement that endpoint themselves.
func shouldBridgeOpenAILegacyCompactToNativeV2(c *gin.Context, account *Account) bool {
	return account != nil && account.UsesOpenAICodexProtocol() && isOpenAIResponsesCompactPath(c)
}

func markOpenAILegacyCompactNativeV2Bridge(c *gin.Context) {
	if c == nil {
		return
	}
	c.Set(openAILegacyCompactNativeV2BridgeKey, true)
	MarkOpenAINativeCompactionV2(c)
}

func isOpenAILegacyCompactNativeV2Bridge(c *gin.Context, account *Account) bool {
	return c != nil && account != nil && account.UsesOpenAICodexProtocol() &&
		c.GetBool(openAILegacyCompactNativeV2BridgeKey)
}

// prepareOpenAIUpstreamResponsesBody converts only the outbound body. The Gin
// request path and the caller-visible stream flag stay unchanged so legacy
// clients continue receiving their unary JSON response.
func prepareOpenAIUpstreamResponsesBody(c *gin.Context, account *Account, body []byte, stream bool) ([]byte, bool, error) {
	if account.IsOpenAISiwc() {
		normalized, err := normalizeSIWCResponsesBody(body)
		return normalized, true, err
	}
	if !shouldBridgeOpenAILegacyCompactToNativeV2(c, account) {
		return body, stream, nil
	}

	var payload map[string]any
	if err := decodeOpenAIJSONUseNumber(body, &payload); err != nil {
		return nil, stream, fmt.Errorf("decode legacy compact bridge body: %w", err)
	}

	input, ok := payload["input"].([]any)
	if !ok {
		if text, textOK := payload["input"].(string); textOK && text != "" {
			input = []any{map[string]any{
				"type":    "message",
				"role":    "user",
				"content": text,
			}}
		} else {
			input = []any{}
		}
	}
	input = append(input, map[string]any{"type": "compaction_trigger"})
	payload["input"] = input
	payload["stream"] = true
	payload["store"] = false

	bridged, err := marshalOpenAIUpstreamJSON(payload)
	if err != nil {
		return nil, stream, fmt.Errorf("encode legacy compact bridge body: %w", err)
	}
	bridged, _, err = NormalizeCompactionTriggerInputOrder(bridged)
	if err != nil {
		return nil, stream, fmt.Errorf("normalize legacy compact bridge trigger: %w", err)
	}
	markOpenAILegacyCompactNativeV2Bridge(c)
	return bridged, true, nil
}

// openAIResponsesUpstreamRequestPathSuffix keeps the public legacy route local
// and sends bridged Codex traffic to the native /responses endpoint.
func openAIResponsesUpstreamRequestPathSuffix(c *gin.Context, account *Account) string {
	if isOpenAILegacyCompactNativeV2Bridge(c, account) {
		return ""
	}
	return openAIResponsesRequestPathSuffix(c)
}
