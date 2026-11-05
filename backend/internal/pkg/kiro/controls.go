package kiro

import (
	"encoding/json"
	"errors"
	"strings"
)

// Claude Code sends empty system messages to change effort mid-conversation.
// Kiro accepts the effective setting at the request level, never as history.
func normalizeControlMessages(req *Request) error {
	messages := make([]Message, 0, len(req.Messages))
	for _, message := range req.Messages {
		if message.Role != "system" {
			messages = append(messages, message)
			continue
		}
		text, err := textContent(message.Content)
		var config map[string]json.RawMessage
		if err != nil || strings.TrimSpace(text) != "" || json.Unmarshal(message.OutputConfig, &config) != nil || len(config) == 0 {
			return errors.New("Kiro only supports empty system messages carrying output_config; put system instructions in the top-level system field")
		}
		req.OutputConfig = message.OutputConfig
	}
	if len(messages) == 0 {
		return errors.New("messages must contain a user message")
	}
	req.Messages = messages
	return nil
}

// Use the selected account's advertised limits, not a hard-coded model table.
func catalogTokenLimit(requested int, raw any) int {
	data, err := json.Marshal(raw)
	if err != nil {
		return requested
	}
	var bounds struct{ Minimum, Maximum int }
	if json.Unmarshal(data, &bounds) != nil {
		return requested
	}
	if bounds.Minimum > 0 && requested < bounds.Minimum {
		requested = bounds.Minimum
	}
	if bounds.Maximum > 0 && requested > bounds.Maximum {
		requested = bounds.Maximum
	}
	return requested
}
