package kiro

import (
	"encoding/json"
	"errors"
	"strings"
)

// Only a terminal Claude suffix is a thinking alias. Custom names containing
// "thinking" and other provider IDs must not be reinterpreted.
func IsThinkingAlias(model string) bool {
	model = strings.TrimPrefix(strings.ToLower(model), "kiro/")
	model = strings.TrimSuffix(model, "[1m]")
	return strings.HasPrefix(model, "claude-") && strings.HasSuffix(model, "-thinking")
}

// The alias supplies a default, never overrides explicit client controls. Use
// rs's 20k budget and let the live catalog translate it to adaptive/effort.
// Retain the original model and unknown fields for response identity and billing.
func ApplyThinkingAlias(raw []byte, mapped string) ([]byte, error) {
	var request map[string]json.RawMessage
	if json.Unmarshal(raw, &request) != nil || request == nil {
		return nil, errors.New("invalid Kiro request JSON")
	}
	var requested string
	_ = json.Unmarshal(request["model"], &requested)
	if !IsThinkingAlias(mapped) && !IsThinkingAlias(requested) {
		return raw, nil
	}
	if thinking := strings.TrimSpace(string(request["thinking"])); thinking != "" && thinking != "null" {
		return raw, nil
	}
	request["thinking"] = json.RawMessage(`{"type":"enabled","budget_tokens":20000}`)
	return json.Marshal(request)
}
