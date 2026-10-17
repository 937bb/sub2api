package service

import (
	"encoding/json"
	"errors"
	"strings"
)

const siwcComputerFunctionName = "siwc_computer"

// adaptSIWCInputHistory lowers client-side call history to programmatic tool
// items accepted by subscription sharing. It preserves every call/output pair
// and moves computer screenshots into ordinary image messages.
func adaptSIWCInputHistory(input []any) ([]any, error) {
	adapted := make([]any, 0, len(input))
	for _, raw := range input {
		item, ok := raw.(map[string]any)
		if !ok {
			return nil, errors.New("invalid SIWC input item")
		}
		switch strings.TrimSpace(firstNonEmptyString(item["type"])) {
		case "computer_call":
			call, err := lowerSIWCComputerCall(item)
			if err != nil {
				return nil, err
			}
			adapted = append(adapted, call)
		case "computer_call_output":
			output, imageMessage, err := lowerSIWCComputerCallOutput(item)
			if err != nil {
				return nil, err
			}
			adapted = append(adapted, output)
			if imageMessage != nil {
				adapted = append(adapted, imageMessage)
			}
		case "tool_search_call":
			call, err := lowerSIWCToolSearchCall(item)
			if err != nil {
				return nil, err
			}
			adapted = append(adapted, call)
		case "tool_search_output":
			output, err := lowerSIWCToolSearchOutput(item)
			if err != nil {
				return nil, err
			}
			adapted = append(adapted, output)
		default:
			adapted = append(adapted, raw)
		}
	}
	return adapted, nil
}

func lowerSIWCComputerCall(item map[string]any) (map[string]any, error) {
	callID := strings.TrimSpace(firstNonEmptyString(item["call_id"], item["id"]))
	if callID == "" {
		return nil, errors.New("SIWC computer_call requires call_id")
	}
	arguments := copySIWCItemWithout(item, "type", "id", "call_id", "name")
	encoded, err := json.Marshal(arguments)
	if err != nil {
		return nil, errors.New("encode SIWC computer_call arguments")
	}
	return map[string]any{
		"type": "function_call", "call_id": callID,
		"name": siwcComputerFunctionName, "arguments": string(encoded),
	}, nil
}

func lowerSIWCComputerCallOutput(item map[string]any) (map[string]any, map[string]any, error) {
	callID := strings.TrimSpace(firstNonEmptyString(item["call_id"], item["id"]))
	if callID == "" {
		return nil, nil, errors.New("SIWC computer_call_output requires call_id")
	}
	output := item["output"]
	images := collectSIWCComputerImages(output)
	text, err := stringifySIWCToolOutput(output, len(images) > 0)
	if err != nil {
		return nil, nil, err
	}
	lowered := map[string]any{"type": "function_call_output", "call_id": callID, "output": text}
	if len(images) == 0 {
		return lowered, nil, nil
	}
	return lowered, map[string]any{"type": "message", "role": "user", "content": images}, nil
}

func lowerSIWCToolSearchCall(item map[string]any) (map[string]any, error) {
	callID := strings.TrimSpace(firstNonEmptyString(item["call_id"], item["id"]))
	if callID == "" {
		return nil, errors.New("SIWC tool_search_call requires call_id")
	}
	arguments := item["arguments"]
	if arguments == nil {
		arguments = map[string]any{}
	}
	encoded, err := stringifySIWCJSON(arguments)
	if err != nil {
		return nil, errors.New("encode SIWC tool_search_call arguments")
	}
	return map[string]any{
		"type": "function_call", "call_id": callID,
		"name": "tool_search", "arguments": encoded,
	}, nil
}

func lowerSIWCToolSearchOutput(item map[string]any) (map[string]any, error) {
	callID := strings.TrimSpace(firstNonEmptyString(item["call_id"], item["id"]))
	if callID == "" {
		return nil, errors.New("SIWC tool_search_output requires call_id")
	}
	value, exists := item["output"]
	if !exists {
		value, exists = item["tools"]
	}
	if !exists {
		return nil, errors.New("SIWC tool_search_output requires output or tools")
	}
	encoded, err := stringifySIWCJSON(value)
	if err != nil {
		return nil, errors.New("encode SIWC tool_search_output")
	}
	return map[string]any{"type": "function_call_output", "call_id": callID, "output": encoded}, nil
}

func stringifySIWCToolOutput(value any, hasImages bool) (string, error) {
	if text, ok := value.(string); ok && !hasImages {
		return text, nil
	}
	if hasImages {
		return "Computer output includes the image in the following user message.", nil
	}
	return stringifySIWCJSON(value)
}

func stringifySIWCJSON(value any) (string, error) {
	if text, ok := value.(string); ok {
		return text, nil
	}
	if value == nil {
		return "", nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func collectSIWCComputerImages(value any) []any {
	images := make([]any, 0, 1)
	var visit func(any)
	visit = func(raw any) {
		switch typed := raw.(type) {
		case []any:
			for _, child := range typed {
				visit(child)
			}
		case map[string]any:
			kind := strings.TrimSpace(firstNonEmptyString(typed["type"]))
			if kind == "computer_screenshot" || kind == "input_image" {
				if imageURL := strings.TrimSpace(firstNonEmptyString(typed["image_url"])); imageURL != "" {
					image := map[string]any{"type": "input_image", "image_url": imageURL}
					if detail := strings.TrimSpace(firstNonEmptyString(typed["detail"])); detail != "" {
						image["detail"] = detail
					}
					images = append(images, image)
					return
				}
			}
			for _, child := range typed {
				visit(child)
			}
		}
	}
	visit(value)
	return images
}

func copySIWCItemWithout(item map[string]any, excluded ...string) map[string]any {
	skip := make(map[string]struct{}, len(excluded))
	for _, key := range excluded {
		skip[key] = struct{}{}
	}
	copy := make(map[string]any, len(item))
	for key, value := range item {
		if _, blocked := skip[key]; !blocked {
			copy[key] = value
		}
	}
	return copy
}
