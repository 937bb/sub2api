package service

import (
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func openAIResponsesInputItemIDPrefix(itemType string) (string, bool) {
	switch strings.TrimSpace(itemType) {
	case "message":
		return "msg", true
	case "reasoning":
		return "rs", true
	case "web_search_call":
		return "ws", true
	case "custom_tool_call":
		return openAIResponsesToolCallIDPrefix(itemType), true
	case "tool_search_call":
		return openAIResponsesToolCallIDPrefix(itemType), true
	case "custom_tool_call_output":
		// Although custom calls use ctc IDs, OpenAI validates replayed custom
		// call output item IDs against the generic fc namespace.
		return "fc", true
	default:
		if isCodexToolCallInputType(itemType) {
			return openAIResponsesToolCallIDPrefix(itemType), true
		}
		return "", false
	}
}

func openAIResponsesToolCallIDPrefix(itemType string) string {
	switch strings.TrimSpace(itemType) {
	case "custom_tool_call", "custom_tool_call_output":
		return "ctc"
	case "tool_search_call", "tool_search_output":
		return "tsc"
	default:
		return "fc"
	}
}

// Invalid replayed IDs are removed rather than rewritten because a fabricated
// ID may point at a different upstream object.
func shouldStripOpenAIResponsesInputItemID(itemType, id string) bool {
	prefix, constrained := openAIResponsesInputItemIDPrefix(itemType)
	if id == "" {
		return constrained
	}
	if strings.TrimSpace(itemType) == "item_reference" {
		return false
	}
	if !isValidOpenAIResponsesItemID(id) {
		return true
	}
	if !constrained {
		return false
	}
	return !strings.HasPrefix(id, prefix)
}

func shouldStripOpenAIResponsesNonPairCallID(itemType string) bool {
	switch strings.TrimSpace(itemType) {
	case "message", "reasoning", "image_generation_call":
		return true
	default:
		return false
	}
}

func isValidOpenAIResponsesItemID(id string) bool {
	for i := 0; i < len(id); i++ {
		c := id[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '_' || c == '-' {
			continue
		}
		return false
	}
	return true
}

// sanitizeOpenAIResponsesInputItems removes replay metadata that is valid in
// response.output items but rejected when the same item is sent back through
// request.input. It preserves the item's semantic payload and pairing fields.
func sanitizeOpenAIResponsesInputItems(body []byte) ([]byte, bool, error) {
	return sanitizeOpenAIResponsesInputItemsWithIDPolicy(body, true)
}

func sanitizeOpenAIResponsesInputItemIDs(body []byte) ([]byte, bool, error) {
	return sanitizeOpenAIResponsesInputItemsWithIDPolicy(body, true)
}

// sanitizeOpenAIResponsesInputItemStatuses is used by native WebSocket ingress.
// That path must follow the same input/output status contract as HTTP while
// preserving client item IDs verbatim.
func sanitizeOpenAIResponsesInputItemStatuses(body []byte) ([]byte, bool, error) {
	return sanitizeOpenAIResponsesInputItemsWithIDPolicy(body, false)
}

func sanitizeOpenAIResponsesInputItemsWithIDPolicy(body []byte, stripInvalidIDs bool) ([]byte, bool, error) {
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return body, false, nil
	}

	items := make([][]byte, 0)
	changed := false
	var sanitizeErr error
	index := 0
	input.ForEach(func(_, item gjson.Result) bool {
		currentIndex := index
		index++
		itemBody := []byte(item.Raw)
		if item.IsObject() {
			itemType := strings.TrimSpace(item.Get("type").String())
			id := item.Get("id")
			if stripInvalidIDs && id.Type == gjson.String && shouldStripOpenAIResponsesInputItemID(itemType, id.String()) {
				itemBody, sanitizeErr = sjson.DeleteBytes(itemBody, "id")
				if sanitizeErr != nil {
					sanitizeErr = fmt.Errorf("delete input.%d.id: %w", currentIndex, sanitizeErr)
					return false
				}
				changed = true
			}
			if stripInvalidIDs && item.Get("call_id").Exists() && shouldStripOpenAIResponsesNonPairCallID(itemType) {
				itemBody, sanitizeErr = sjson.DeleteBytes(itemBody, "call_id")
				if sanitizeErr != nil {
					sanitizeErr = fmt.Errorf("delete input.%d.call_id: %w", currentIndex, sanitizeErr)
					return false
				}
				changed = true
			}
			// status describes server-side output lifecycle (in_progress,
			// completed, and so on). It is not accepted on replayed input items.
			if item.Get("status").Exists() {
				itemBody, sanitizeErr = sjson.DeleteBytes(itemBody, "status")
				if sanitizeErr != nil {
					sanitizeErr = fmt.Errorf("delete input.%d.status: %w", currentIndex, sanitizeErr)
					return false
				}
				changed = true
			}
		}
		items = append(items, itemBody)
		return true
	})
	if sanitizeErr != nil {
		return nil, false, sanitizeErr
	}
	if !changed {
		return body, false, nil
	}

	rebuiltInput := make([]byte, 0, len(input.Raw))
	rebuiltInput = append(rebuiltInput, '[')
	for i, item := range items {
		if i > 0 {
			rebuiltInput = append(rebuiltInput, ',')
		}
		rebuiltInput = append(rebuiltInput, item...)
	}
	rebuiltInput = append(rebuiltInput, ']')

	sanitized, err := sjson.SetRawBytes(body, "input", rebuiltInput)
	if err != nil {
		return nil, false, fmt.Errorf("replace sanitized input: %w", err)
	}
	return sanitized, true, nil
}
