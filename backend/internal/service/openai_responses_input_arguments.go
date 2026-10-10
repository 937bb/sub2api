package service

import (
	"fmt"
	"strconv"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Responses function-call history requires a JSON string, but some clients
// replay the decoded object. Quote its raw JSON without decoding through
// float64 or changing tool identity, custom-tool input, or existing strings.
func normalizeOpenAIResponsesInputArguments(body []byte) ([]byte, bool, error) {
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return body, false, nil
	}
	normalized := body
	changed := false
	for i, item := range input.Array() {
		if item.Get("type").String() != "function_call" {
			continue
		}
		arguments := item.Get("arguments")
		if !arguments.IsObject() {
			continue
		}
		if !changed && !gjson.ValidBytes(body) {
			return body, false, fmt.Errorf("normalize Responses function arguments: invalid JSON")
		}
		next, err := sjson.SetBytes(normalized, "input."+strconv.Itoa(i)+".arguments", arguments.Raw)
		if err != nil {
			return body, false, fmt.Errorf("normalize Responses input[%d].arguments: %w", i, err)
		}
		normalized = next
		changed = true
	}
	return normalized, changed, nil
}
