package service

import (
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Invalid replayed IDs are removed rather than rewritten because a fabricated
// item ID may point at a different upstream object. call_id is intentionally
// untouched: it carries tool-call/output pairing and has separate validation.
func shouldStripOpenAIResponsesInputItemID(itemType, id string) bool {
	if id == "" {
		return false
	}
	// item_reference.id is the reference itself rather than optional replay
	// metadata. Removing it would turn one precise validation error into a
	// missing-reference error and silently lose the requested continuation.
	if itemType == "item_reference" {
		return false
	}
	if !isValidOpenAIResponsesItemID(id) {
		return true
	}

	requiredPrefix := ""
	switch itemType {
	case "message":
		requiredPrefix = "msg"
	case "reasoning":
		requiredPrefix = "rs"
	case "custom_tool_call":
		// ChatGPT Codex validates custom tool-call item IDs separately from
		// function calls: fc_* is rejected with "Expected ... begins with ctc".
		requiredPrefix = "ctc"
	default:
		if isCodexFunctionCallInputType(itemType) {
			requiredPrefix = "fc"
		}
	}
	if requiredPrefix != "" {
		return !strings.HasPrefix(id, requiredPrefix)
	}
	return false
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

func sanitizeOpenAIResponsesInputItemIDs(body []byte) ([]byte, bool, error) {
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
			itemType := item.Get("type")
			id := item.Get("id")
			if itemType.Type == gjson.String && id.Type == gjson.String &&
				shouldStripOpenAIResponsesInputItemID(itemType.String(), id.String()) {
				itemBody, sanitizeErr = sjson.DeleteBytes(itemBody, "id")
				if sanitizeErr != nil {
					sanitizeErr = fmt.Errorf("delete input.%d.id: %w", currentIndex, sanitizeErr)
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
