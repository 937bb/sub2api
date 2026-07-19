package service

import (
	"bytes"
	"encoding/json"
	"strings"
	"unsafe"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	blockTypeServerToolUse       = "server_tool_use"
	blockTypeWebSearchToolResult = "web_search_tool_result"
)

var (
	patternServerToolUse       = []byte(`"server_tool_use"`)
	patternWebSearchToolResult = []byte(`"web_search_tool_result"`)
)

// FilterWebSearchHistoryBlocks removes locally synthesized web-search blocks
// before replay. Passback-required upstreams additionally reject genuine
// Anthropic server-side search blocks, so those are removed on that boundary.
func FilterWebSearchHistoryBlocks(body []byte, mappedModel string) []byte {
	if !bytes.Contains(body, patternServerToolUse) && !bytes.Contains(body, patternWebSearchToolResult) {
		return body
	}

	stripAll := ResolveThinkingProtocol(mappedModel) == ThinkingProtocolPassbackRequired
	jsonStr := *(*string)(unsafe.Pointer(&body))
	messagesResult := gjson.Get(jsonStr, "messages")
	if !messagesResult.Exists() || !messagesResult.IsArray() {
		return body
	}

	var messages []any
	if err := json.Unmarshal(sliceRawFromBody(body, messagesResult), &messages); err != nil {
		return body
	}

	modified := false
	for _, message := range messages {
		messageMap, ok := message.(map[string]any)
		if !ok {
			continue
		}
		content, ok := messageMap["content"].([]any)
		if !ok {
			continue
		}

		var filtered []any
		for i, block := range content {
			blockMap, isMap := block.(map[string]any)
			if isMap && shouldStripWebSearchBlock(blockMap, stripAll) {
				if filtered == nil {
					filtered = make([]any, 0, len(content))
					filtered = append(filtered, content[:i]...)
				}
				continue
			}
			if filtered != nil {
				filtered = append(filtered, block)
			}
		}
		if filtered == nil {
			continue
		}

		modified = true
		if len(filtered) == 0 {
			placeholder := "(content removed)"
			if role, _ := messageMap["role"].(string); role == "assistant" {
				placeholder = "(assistant content removed)"
			}
			filtered = []any{map[string]any{"type": "text", "text": placeholder}}
		}
		messageMap["content"] = filtered
	}

	if !modified {
		return body
	}
	messagesJSON, err := json.Marshal(messages)
	if err != nil {
		return body
	}
	out, err := sjson.SetRawBytes(body, "messages", messagesJSON)
	if err != nil {
		return body
	}
	return out
}

func shouldStripWebSearchBlock(block map[string]any, stripAll bool) bool {
	blockType, _ := block["type"].(string)
	switch blockType {
	case blockTypeServerToolUse:
		if stripAll {
			return true
		}
		id, _ := block["id"].(string)
		return strings.HasPrefix(id, webSearchToolUseIDPrefix)
	case blockTypeWebSearchToolResult:
		if stripAll {
			return true
		}
		id, _ := block["tool_use_id"].(string)
		return strings.HasPrefix(id, webSearchToolUseIDPrefix)
	default:
		return false
	}
}
