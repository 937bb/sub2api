package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/tidwall/gjson"
)

// This policy controls outbound localization, not IP geolocation or billing time.
// Enable it only on the intended deployment; ordinary API-key traffic is untouched.
var codexUSLocaleEnabled atomic.Bool

const codexUSLanguage = "en-US,en;q=0.9"
const codexUSTimezone = "America/New_York"

func usesCodexUSLocale(account *Account) bool {
	return codexUSLocaleEnabled.Load() && account != nil && account.IsOpenAIOAuthLike()
}

var codexLocaleKeySeparators = strings.NewReplacer("-", "", "_", "")

func codexLocaleKey(key string) string {
	return codexLocaleKeySeparators.Replace(strings.ToLower(key))
}

// Only recognized protocol metadata is localized. Unknown values and nested
// application data are preserved, including precision-sensitive numeric IDs.
func localizeCodexMetadata(values map[string]any) bool {
	changed := false
	for key, value := range values {
		var replacement string
		switch codexLocaleKey(key) {
		case "timezone", "timezonename", "clienttimezone":
			replacement = codexUSTimezone
		case "locale", "language", "clientlocale":
			replacement = "en-US"
		case "acceptlanguage":
			replacement = codexUSLanguage
		case "timezoneoffset", "timezoneoffsetminutes", "utcoffset", "currentdate":
			// Offset conventions differ (UTC offset vs JS getTimezoneOffset). Do not
			// invent a sign or keep a date belonging to the previous timezone.
			delete(values, key)
			changed = true
			continue
		default:
			if strings.EqualFold(key, openAIWSTurnMetadataHeader) {
				if raw, ok := value.(string); ok {
					if next, ok := localizeCodexMetadataJSON(raw); ok {
						values[key] = next
						changed = true
					}
				}
			}
			continue
		}
		if value != replacement {
			values[key] = replacement
			changed = true
		}
	}
	return changed
}

func localizeCodexMetadataJSON(raw string) (string, bool) {
	// Embedded metadata is a single object. Avoid recursively following an
	// attacker-controlled chain of JSON strings with the same metadata key.
	values := map[string]any{}
	if decodeCodexIdentityMetadata(raw, &values) != nil || values == nil {
		return raw, false
	}
	embedded := make(map[string]any)
	for key, value := range values {
		if strings.EqualFold(key, openAIWSTurnMetadataHeader) {
			embedded[key] = value
			delete(values, key)
		}
	}
	changed := localizeCodexMetadata(values)
	for key, value := range embedded {
		values[key] = value
	}
	if !changed {
		return raw, false
	}
	encoded, err := json.Marshal(values)
	return string(encoded), err == nil
}

func applyCodexUSLocaleHeaders(h http.Header) {
	if h == nil || !codexUSLocaleEnabled.Load() {
		return
	}
	// No made-up timezone header: omit caller timezone hints instead.
	for key := range h {
		switch codexLocaleKey(key) {
		case "timezone", "xtimezone", "oaitimezone", "oaitimezoneoffset", "xclienttimezone", "xopenaitimezone":
			delete(h, key)
		}
	}
	// Localization changes existing language preferences, not the request's
	// capability/header shape. Authentication and native clients may omit it.
	if codexIdentityHeaderValue(h, "accept-language") != "" {
		setCodexIdentityHeader(h, "accept-language", codexUSLanguage)
	}
	if raw := codexIdentityHeaderValue(h, openAIWSTurnMetadataHeader); raw != "" {
		if next, changed := localizeCodexMetadataJSON(raw); changed {
			raw = next
		}
		// Collapse case aliases even when the preferred value is already localized.
		setCodexIdentityHeader(h, openAIWSTurnMetadataHeader, raw)
	}
}

var codexEnvironmentTimezone = regexp.MustCompile(`(?m)^[\t ]*<timezone>[^<>\r\n]+</timezone>[\t ]*\r?$`)
var codexEnvironmentDate = regexp.MustCompile(`(?m)^[\t ]*<current_date>[^<>\r\n]+</current_date>[\t ]*\r?\n?`)

func localizeCodexEnvironment(text string) string {
	trimmed := strings.TrimSpace(text)
	// Never replace arbitrary prose, code fences, quoted examples or tool output.
	if (!strings.HasPrefix(trimmed, "<environment_context>\n") && !strings.HasPrefix(trimmed, "<environment_context>\r\n")) ||
		!strings.HasSuffix(trimmed, "</environment_context>") ||
		strings.Count(trimmed, "<environment_context>") != 1 ||
		strings.Count(trimmed, "</environment_context>") != 1 ||
		len(codexEnvironmentTimezone.FindAllStringIndex(trimmed, -1)) != 1 {
		return text
	}
	if strings.TrimSpace(codexEnvironmentTimezone.FindString(trimmed)) == "<timezone>"+codexUSTimezone+"</timezone>" {
		return text
	}
	next := codexEnvironmentTimezone.ReplaceAllString(text, "<timezone>"+codexUSTimezone+"</timezone>")
	return codexEnvironmentDate.ReplaceAllString(next, "")
}

type codexLocaleEdit struct {
	start, end int
	value      []byte
}

// Edits use original JSON spans and one output allocation. Large histories,
// tool results, encrypted reasoning and unknown top-level fields are not decoded
// or reserialized. Inbound bytes remain intact for retries and auditing.
func normalizeCodexUSLocale(body []byte, account *Account) ([]byte, bool, error) {
	if !usesCodexUSLocale(account) || len(body) == 0 {
		return body, false, nil
	}
	if !gjson.ValidBytes(body) {
		return body, false, fmt.Errorf("normalize Codex locale: invalid JSON")
	}
	root := gjson.ParseBytes(body)
	if !root.IsObject() {
		return body, false, fmt.Errorf("normalize Codex locale: expected JSON object")
	}
	if event := root.Get("type"); event.Exists() && event.String() != "response.create" {
		return body, false, nil
	}
	var edits []codexLocaleEdit
	addString := func(value gjson.Result) {
		if value.Type != gjson.String {
			return
		}
		next := localizeCodexEnvironment(value.String())
		if next == value.String() {
			return
		}
		encoded, _ := json.Marshal(next)
		edits = append(edits, codexLocaleEdit{value.Index, value.Index + len(value.Raw), encoded})
	}
	if cm := root.Get("client_metadata"); cm.IsObject() {
		values := map[string]any{}
		if err := decodeCodexIdentityMetadata(cm.Raw, &values); err != nil {
			return body, false, err
		}
		if localizeCodexMetadata(values) {
			encoded, err := json.Marshal(values)
			if err != nil {
				return body, false, err
			}
			edits = append(edits, codexLocaleEdit{cm.Index, cm.Index + len(cm.Raw), encoded})
		}
	}
	input := root.Get("input")
	if input.Type == gjson.String {
		addString(input)
	}
	if input.IsArray() {
		input.ForEach(func(_, item gjson.Result) bool {
			role := item.Get("role").String()
			kind := item.Get("type")
			if (role != "user" && role != "developer" && role != "system") ||
				(kind.Exists() && kind.String() != "message") || item.Get("tool_call_id").Exists() || item.Get("tool_calls").Exists() {
				return true
			}
			content := item.Get("content")
			if content.Type == gjson.String {
				addString(content)
			}
			if content.IsArray() {
				content.ForEach(func(_, part gjson.Result) bool {
					if t := part.Get("type").String(); t == "input_text" || t == "text" {
						addString(part.Get("text"))
					}
					return true
				})
			}
			return true
		})
	}
	if len(edits) == 0 {
		return body, false, nil
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].start < edits[j].start })
	var out bytes.Buffer
	out.Grow(len(body))
	previous := 0
	for _, edit := range edits {
		if edit.start < previous || edit.end > len(body) {
			return body, false, fmt.Errorf("normalize Codex locale: invalid JSON span")
		}
		_, _ = out.Write(body[previous:edit.start]) // bytes.Buffer.Write never returns an error.
		_, _ = out.Write(edit.value)
		previous = edit.end
	}
	_, _ = out.Write(body[previous:])
	return out.Bytes(), true, nil
}

func normalizeCodexOutboundRequest(body []byte, account *Account) ([]byte, bool, error) {
	localized, localeChanged, err := normalizeCodexUSLocale(body, account)
	if err != nil {
		return body, false, err
	}
	cacheAligned, cacheChanged, err := normalizeGPT62InstructionsForCache(localized, account, time.Now())
	if err != nil {
		return body, false, err
	}
	next, roleChanged, err := normalizeCodexDeveloperMessages(cacheAligned, account)
	return next, localeChanged || cacheChanged || roleChanged, err
}

// Clone only changed maps/slices: pooled WS payloads may share nested history
// with replay/prewarm state owned by the caller.
func normalizeCodexUSLocaleMap(body map[string]any, account *Account) {
	if !usesCodexUSLocale(account) || body == nil {
		return
	}
	if event, ok := body["type"]; ok && event != "response.create" {
		return
	}
	normalizeCodexUSLocaleMetadataMap(body, account)
	if text, ok := body["input"].(string); ok {
		body["input"] = localizeCodexEnvironment(text)
		return
	}
	input, ok := body["input"].([]any)
	if !ok {
		return
	}
	var next []any
	for i, value := range input {
		item, ok := value.(map[string]any)
		if !ok {
			continue
		}
		role, _ := item["role"].(string)
		if role != "user" && role != "developer" && role != "system" {
			continue
		}
		if kind, ok := item["type"]; ok && kind != "message" {
			continue
		}
		if _, ok := item["tool_call_id"]; ok {
			continue
		}
		if _, ok := item["tool_calls"]; ok {
			continue
		}
		content, changed := localizeCodexContent(item["content"])
		if !changed {
			continue
		}
		if next == nil {
			next = append([]any(nil), input...)
		}
		clone := make(map[string]any, len(item))
		for k, v := range item {
			clone[k] = v
		}
		clone["content"] = content
		next[i] = clone
	}
	if next != nil {
		body["input"] = next
	}
}

func localizeCodexContent(content any) (any, bool) {
	if text, ok := content.(string); ok {
		next := localizeCodexEnvironment(text)
		return next, next != text
	}
	parts, ok := content.([]any)
	if !ok {
		return content, false
	}
	var next []any
	for i, value := range parts {
		part, ok := value.(map[string]any)
		if !ok {
			continue
		}
		if part["type"] != "text" && part["type"] != "input_text" {
			continue
		}
		text, ok := part["text"].(string)
		if !ok {
			continue
		}
		localized := localizeCodexEnvironment(text)
		if localized == text {
			continue
		}
		if next == nil {
			next = append([]any(nil), parts...)
		}
		clone := make(map[string]any, len(part))
		for k, v := range part {
			clone[k] = v
		}
		clone["text"] = localized
		next[i] = clone
	}
	if next == nil {
		return content, false
	}
	return next, true
}

func normalizeCodexUSLocaleMetadataMap(body map[string]any, account *Account) {
	if !usesCodexUSLocale(account) {
		return
	}
	if cm, ok := body["client_metadata"].(map[string]any); ok {
		clone := make(map[string]any, len(cm))
		for k, v := range cm {
			clone[k] = v
		}
		if localizeCodexMetadata(clone) {
			body["client_metadata"] = clone
		}
	}
}
