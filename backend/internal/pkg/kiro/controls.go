package kiro

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Claude Code requests live reasoning updates, while Kiro accepts only
// summarized or omitted reasoning display. Preserve all other thinking fields.
func normalizeThinkingDisplay(raw json.RawMessage) (json.RawMessage, error) {
	var thinking map[string]json.RawMessage
	if err := json.Unmarshal(raw, &thinking); err != nil || thinking == nil {
		return nil, errors.New("thinking must be an object")
	}
	var display string
	if json.Unmarshal(thinking["display"], &display) == nil && display == "updates" {
		thinking["display"] = json.RawMessage(`"summarized"`)
		return json.Marshal(thinking)
	}
	return raw, nil
}

// A CLI can retain xhigh when switching models. Match kiro.rs's compatibility
// mapping, but use the selected account's catalog instead of a model deny-list.
func normalizeOutputEffort(raw json.RawMessage, schema any) (json.RawMessage, error) {
	var config map[string]json.RawMessage
	if json.Unmarshal(raw, &config) != nil || config == nil {
		return nil, errors.New("output_config must be an object")
	}
	value, present := config["effort"]
	if !present {
		return raw, nil
	}
	var effort string
	if json.Unmarshal(value, &effort) != nil {
		return nil, errors.New("output_config.effort must be a string")
	}
	data, err := json.Marshal(schema)
	if err != nil {
		return raw, nil
	}
	var capabilities struct {
		Properties map[string]struct {
			Enum []string `json:"enum"`
		} `json:"properties"`
	}
	if json.Unmarshal(data, &capabilities) != nil || len(capabilities.Properties["effort"].Enum) == 0 {
		return raw, nil
	}
	allowed := capabilities.Properties["effort"].Enum
	for _, candidate := range allowed {
		if candidate == effort {
			return raw, nil
		}
	}
	if effort == "xhigh" {
		for _, candidate := range allowed {
			if candidate == "high" {
				config["effort"] = json.RawMessage(`"high"`)
				return json.Marshal(config)
			}
		}
	}
	return nil, errors.New("output_config.effort is unsupported by the selected Kiro model; choose a supported effort level")
}

// Claude Code can include system instructions and effort updates in history.
// Preserve their contents at the request level before building Kiro history.
func normalizeControlMessages(req *Request) error {
	messages := make([]Message, 0, len(req.Messages))
	instructions := []string{}
	for _, message := range req.Messages {
		if message.Role != "system" {
			messages = append(messages, message)
			continue
		}
		text, err := textContent(message.Content)
		if err != nil {
			return fmt.Errorf("system message: %w", err)
		}
		if text != "" {
			instructions = append(instructions, text)
		}
		if len(message.OutputConfig) != 0 && strings.TrimSpace(string(message.OutputConfig)) != "null" {
			var update map[string]json.RawMessage
			if json.Unmarshal(message.OutputConfig, &update) != nil {
				return errors.New("system message output_config must be an object")
			}
			if len(update) == 0 {
				continue
			}
			config := map[string]json.RawMessage{}
			if len(req.OutputConfig) != 0 && strings.TrimSpace(string(req.OutputConfig)) != "null" {
				if json.Unmarshal(req.OutputConfig, &config) != nil {
					return errors.New("output_config must be an object")
				}
			}
			for key, value := range update {
				config[key] = value
			}
			req.OutputConfig, err = json.Marshal(config)
			if err != nil {
				return fmt.Errorf("system message output_config: %w", err)
			}
		}
	}
	if len(messages) == 0 {
		return errors.New("messages must contain a user message")
	}
	if len(instructions) > 0 {
		system, err := textContent(req.System)
		if err != nil {
			return fmt.Errorf("system: %w", err)
		}
		if system != "" {
			instructions = append([]string{system}, instructions...)
		}
		req.System, err = json.Marshal(strings.Join(instructions, "\n\n"))
		if err != nil {
			return fmt.Errorf("system instructions: %w", err)
		}
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
