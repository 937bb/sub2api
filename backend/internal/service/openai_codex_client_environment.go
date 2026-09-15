package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const codexClientTimezone = "America/Los_Angeles"
const codexClientLocale = "en-US"
const codexClientAcceptLanguage = "en-US,en;q=0.9"

// Normalize optional client environment metadata, not message text or routing
// hints. An IANA zone describes daylight saving without a fixed UTC offset.
func normalizeCodexClientEnvironment(metadata map[string]any, depth int) bool {
	if metadata == nil || depth > 4 {
		return false
	}
	changed := false
	for key, value := range metadata {
		var replacement string
		switch key {
		case "timezone", "time_zone", "timeZone", "tz":
			replacement = codexClientTimezone
		case "locale", "language", "language_tag", "languageTag":
			replacement = codexClientLocale
		case "country", "country_code", "countryCode":
			replacement = "US"
		case "client_name", "app_name", "sdk_name":
			if text, ok := value.(string); ok && strings.Contains(strings.ToLower(text), "sub2api") {
				replacement = "api-client"
			}
		case "environment", "client_environment", "runtime_environment":
			if nested, ok := value.(map[string]any); ok && normalizeCodexClientEnvironment(nested, depth+1) {
				changed = true
			}
		case openAIWSTurnMetadataHeader:
			if text, ok := value.(string); ok {
				if next, edited := normalizeCodexClientEnvironmentJSON(text, depth+1); edited {
					metadata[key] = next
					changed = true
				}
			}
		}
		if text, ok := value.(string); ok && replacement != "" && text != replacement {
			metadata[key] = replacement
			changed = true
		}
	}
	return changed
}

func normalizeCodexClientEnvironmentJSON(raw string, depth int) (string, bool) {
	if depth > 4 || !gjson.Valid(raw) || !gjson.Parse(raw).IsObject() {
		return raw, false
	}
	var metadata map[string]any
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&metadata) != nil || !normalizeCodexClientEnvironment(metadata, depth) {
		return raw, false
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return raw, false
	}
	return string(encoded), true
}

func applyCodexClientEnvironmentRaw(body []byte, account *Account) ([]byte, bool, error) {
	if account == nil || !account.IsOpenAIOAuthLike() {
		return body, false, nil
	}
	metadata := gjson.GetBytes(body, "client_metadata")
	if !metadata.IsObject() {
		return body, false, nil
	}
	normalized, changed := normalizeCodexClientEnvironmentJSON(metadata.Raw, 0)
	if !changed {
		return body, false, nil
	}
	next, err := sjson.SetRawBytes(body, "client_metadata", []byte(normalized))
	if err != nil {
		return body, false, fmt.Errorf("normalize Codex client environment: %w", err)
	}
	return next, !bytes.Equal(body, next), nil
}

func applyCodexClientEnvironmentMap(body map[string]any, account *Account) bool {
	if account == nil || !account.IsOpenAIOAuthLike() {
		return false
	}
	metadata, _ := body["client_metadata"].(map[string]any)
	return normalizeCodexClientEnvironment(metadata, 0)
}

func applyCodexClientEnvironmentHeaders(headers http.Header, account *Account) {
	if headers == nil || account == nil || !account.IsOpenAIOAuthLike() {
		return
	}
	headers.Set("Accept-Language", codexClientAcceptLanguage)
	if raw := headers.Get(openAIWSTurnMetadataHeader); raw != "" {
		if next, changed := normalizeCodexClientEnvironmentJSON(raw, 0); changed {
			headers.Set(openAIWSTurnMetadataHeader, next)
		}
	}
}
