package service

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

var openAIOAuthHTTPAllowlistFields = []string{
	"model",
	"input",
	"instructions",
	"tools",
	"tool_choice",
	"parallel_tool_calls",
	"reasoning",
	"store",
	"stream",
	"include",
	"service_tier",
	"prompt_cache_key",
	"text",
	"client_metadata",
}

var openAIOAuthCompactAllowlistFields = []string{
	"model",
	"input",
	"instructions",
	"tools",
	"parallel_tool_calls",
	"reasoning",
	"service_tier",
	"prompt_cache_key",
	"text",
}

// applyOpenAIOAuthHTTPAllowlist applies the terminal OAuth /responses body policy.
// It copies allowed fields with raw JSON so large numeric literals are not rounded.
func applyOpenAIOAuthHTTPAllowlist(body []byte) ([]byte, bool, error) {
	if len(body) == 0 {
		return body, false, nil
	}
	if !json.Valid(body) {
		return body, false, fmt.Errorf("apply openai oauth http allowlist: invalid JSON")
	}

	normalized, err := copyOpenAIOAuthAllowedRawFields(body, openAIOAuthHTTPAllowlistFields, "http")
	if err != nil {
		return body, false, err
	}

	// Codex CLI fixed values for non-compact /responses.
	if store := gjson.GetBytes(normalized, "store"); !store.Exists() || store.Type != gjson.False {
		next, err := sjson.SetBytes(normalized, "store", false)
		if err != nil {
			return body, false, fmt.Errorf("apply openai oauth http allowlist store=false: %w", err)
		}
		normalized = next
	}
	if stream := gjson.GetBytes(normalized, "stream"); !stream.Exists() || stream.Type != gjson.True {
		next, err := sjson.SetBytes(normalized, "stream", true)
		if err != nil {
			return body, false, fmt.Errorf("apply openai oauth http allowlist stream=true: %w", err)
		}
		normalized = next
	}

	return changedOpenAIOAuthBody(body, normalized)
}

// applyOpenAIOAuthCompactAllowlist applies the terminal OAuth /responses/compact body policy.
// Compact follows Codex CompactionInput fields and intentionally omits store/stream/client_metadata.
func applyOpenAIOAuthCompactAllowlist(body []byte) ([]byte, bool, error) {
	if len(body) == 0 {
		return body, false, nil
	}
	if !json.Valid(body) {
		return body, false, fmt.Errorf("apply openai oauth compact allowlist: invalid JSON")
	}

	normalized, err := copyOpenAIOAuthAllowedRawFields(body, openAIOAuthCompactAllowlistFields, "compact")
	if err != nil {
		return body, false, err
	}
	return changedOpenAIOAuthBody(body, normalized)
}

func copyOpenAIOAuthAllowedRawFields(body []byte, fields []string, policy string) ([]byte, error) {
	normalized := []byte(`{}`)
	for _, field := range fields {
		value := gjson.GetBytes(body, field)
		if !value.Exists() {
			continue
		}
		next, err := sjson.SetRawBytes(normalized, field, []byte(value.Raw))
		if err != nil {
			return nil, fmt.Errorf("apply openai oauth %s allowlist %s: %w", policy, field, err)
		}
		normalized = next
	}
	return normalized, nil
}

func changedOpenAIOAuthBody(original, normalized []byte) ([]byte, bool, error) {
	if bytes.Equal(bytes.TrimSpace(original), bytes.TrimSpace(normalized)) {
		return original, false, nil
	}
	return normalized, true, nil
}
