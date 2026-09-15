package service

import (
	"encoding/json"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/tidwall/gjson"
)

const (
	openAICompatClaudeCodeTodoGuardMarker       = "<task-tracking-compat>"
	legacyOpenAICompatClaudeCodeTodoGuardMarker = "<sub2api-claude-code-todo-guard>"
	openAICompatClaudeCodeTodoGuardText         = openAICompatClaudeCodeTodoGuardMarker + "\nWhen using Claude Code todo or task tracking tools, keep the visible task list consistent. Do not send final or summary text while any item remains in_progress. Before finishing, asking the user to choose, or reporting a blocker, update the todo list so completed work is completed and deferred work is pending/open; leave an item in_progress only when active work will continue in the same turn.\n</task-tracking-compat>"
)

func appendOpenAICompatClaudeCodeTodoGuard(req *apicompat.ResponsesRequest) bool {
	if req == nil || len(req.Input) == 0 {
		return false
	}

	// Retain opaque item fields: a typed round trip loses custom tool
	// inputs and unknown fields, and can turn multimodal tool outputs into text.
	var items []json.RawMessage
	if err := json.Unmarshal(req.Input, &items); err != nil {
		return false
	}
	if len(items) == 0 || compatRawItemsContainGuard(gjson.ParseBytes(req.Input)) {
		return false
	}

	content, err := json.Marshal([]apicompat.ResponsesContentPart{{
		Type: "input_text",
		Text: openAICompatClaudeCodeTodoGuardText,
	}})
	if err != nil {
		return false
	}

	guard, err := json.Marshal(apicompat.ResponsesInputItem{
		Type:    "message",
		Role:    "developer",
		Content: content,
	})
	if err != nil {
		return false
	}

	insertAt := 0
	for insertAt < len(items) {
		item := gjson.ParseBytes(items[insertAt])
		kind := item.Get("type").String()
		if item.Get("role").String() != "developer" || (kind != "" && kind != "message") {
			break
		}
		insertAt++
	}

	items = append(items, nil)
	copy(items[insertAt+1:], items[insertAt:])
	items[insertAt] = guard

	input, err := json.Marshal(items)
	if err != nil {
		return false
	}
	req.Input = input
	return true
}

func appendOpenAICompatClaudeCodeTodoGuardToRequestBody(reqBody map[string]any) bool {
	if reqBody == nil {
		return false
	}

	input, ok := reqBody["input"].([]any)
	if !ok || len(input) == 0 || inputContainsText(input, openAICompatClaudeCodeTodoGuardMarker) || inputContainsText(input, legacyOpenAICompatClaudeCodeTodoGuardMarker) {
		return false
	}

	guard := map[string]any{
		"type": "message",
		"role": "developer",
		"content": []any{
			map[string]any{
				"type": "input_text",
				"text": openAICompatClaudeCodeTodoGuardText,
			},
		},
	}

	insertAt := 0
	for insertAt < len(input) {
		item, ok := input[insertAt].(map[string]any)
		kind := strings.TrimSpace(firstNonEmptyString(item["type"]))
		if !ok || (kind != "" && kind != "message") || strings.TrimSpace(firstNonEmptyString(item["role"])) != "developer" {
			break
		}
		insertAt++
	}

	input = append(input, nil)
	copy(input[insertAt+1:], input[insertAt:])
	input[insertAt] = guard
	reqBody["input"] = input
	return true
}

func inputContainsText(input []any, needle string) bool {
	needle = strings.TrimSpace(needle)
	if needle == "" {
		return false
	}
	for _, item := range input {
		message, ok := item.(map[string]any)
		if !ok || message["role"] != "developer" || (message["type"] != nil && message["type"] != "" && message["type"] != "message") {
			continue
		}
		if compatContentContainsText(message["content"], needle) {
			return true
		}
	}
	return false
}

// Only inspect developer text, never quoted user text or tool arguments.
func compatContentContainsText(value any, needle string) bool {
	switch v := value.(type) {
	case string:
		return strings.Contains(v, needle)
	case []any:
		for _, item := range v {
			part, ok := item.(map[string]any)
			if !ok || (part["type"] != "input_text" && part["type"] != "text") {
				continue
			}
			if text, ok := part["text"].(string); ok && strings.Contains(text, needle) {
				return true
			}
		}
	}
	return false
}

// Read individual JSON text fields without decoding the entire input array.
// This keeps image-heavy requests off the full-allocation path.
func compatRawInputContainsGuard(body []byte) bool {
	return compatRawItemsContainGuard(gjson.GetBytes(body, "input"))
}

func compatRawItemsContainGuard(input gjson.Result) bool {
	if !input.IsArray() {
		return false
	}
	found := false
	input.ForEach(func(_, item gjson.Result) bool {
		if item.Get("role").String() != "developer" {
			return true
		}
		if kind := item.Get("type").String(); kind != "" && kind != "message" {
			return true
		}
		matches := func(text string) bool {
			return strings.Contains(text, openAICompatClaudeCodeTodoGuardMarker) || strings.Contains(text, legacyOpenAICompatClaudeCodeTodoGuardMarker)
		}
		content := item.Get("content")
		if content.Type == gjson.String {
			found = matches(content.String())
		} else if content.IsArray() {
			content.ForEach(func(_, part gjson.Result) bool {
				if kind := part.Get("type").String(); kind == "input_text" || kind == "text" {
					found = matches(codexStringField(part.Get("text")))
				}
				return !found
			})
		}
		return !found
	})
	return found
}
