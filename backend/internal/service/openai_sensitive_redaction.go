package service

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
)

var (
	openAISensitiveDiagnosticFieldNames = []string{
		"x-codex-installation-id",
		"x-codex-window-id",
		"session-id",
		"thread-id",
		"x-client-request-id",
		"installation_id",
		"thread_id",
		"window_id",
		"prompt_cache_key",
		"session_id",
		"conversation_id",
		"raw_user_agent",
		"user_agent",
		"user-agent",
		"authorization",
		"access_token",
		"refresh_token",
		"id_token",
		"session_token",
		"api_key",
		"apikey",
		"token",
		"personal_access_token",
		"email",
		"chatgpt_user_id",
		"chatgpt_account_id",
		"chatgpt-account-id",
		"chatgpt_plan_type",
		"chatgpt_account_is_fedramp",
		"x-openai-fedramp",
	}
	openAISensitiveDiagnosticFieldSet         = buildOpenAISensitiveDiagnosticFieldSet(openAISensitiveDiagnosticFieldNames)
	openAISensitiveDiagnosticFieldPattern     = strings.Join(openAISensitiveDiagnosticFieldNames, "|")
	openAISensitiveDiagnosticJSONFieldRe      = regexp.MustCompile(`(?i)("(?:` + openAISensitiveDiagnosticFieldPattern + `)"\s*:\s*)(?:"(?:\\.|[^"\\])*"|true|false|null|-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)(\s*(?:[,}\]]|$))`)
	openAISensitiveDiagnosticJSONKeyRe        = regexp.MustCompile(`(?i)"(?:` + openAISensitiveDiagnosticFieldPattern + `)"\s*:\s*`)
	openAISensitiveDiagnosticEscapedJSONKeyRe = regexp.MustCompile(`(?i)\\"(?:` + openAISensitiveDiagnosticFieldPattern + `)\\"\s*:\s*`)
	openAISensitiveDiagnosticKVFieldRe        = regexp.MustCompile(`(?i)\b((?:` + openAISensitiveDiagnosticFieldPattern + `)\s*(?:=|:)\s*)(?:Bearer\s+)?(?:"(?:\\.|[^"\\])*"|[^\s,;"}]+)`)
	openAISensitiveDiagnosticBearerRe         = regexp.MustCompile(`(?i)\bBearer\s+[^\s,;"}]+`)
	openAISensitiveDiagnosticUUIDRe           = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)
	openAISensitiveDiagnosticCodexUARe        = regexp.MustCompile(`(?i)codex-tui/[^\s";]+\s+\([^"\)]*\)\s+[^\s"]+\s+\(codex-tui;\s*[^\)]*\)`)
)

const (
	openAISensitiveDiagnosticJSONCompositeMaxScan = 8192
	openAISensitiveDiagnosticJSONMaxDepth         = 16
	openAISensitiveDiagnosticJSONBodyMaxParse     = 512 << 10
	openAISensitiveDiagnosticEmbeddedJSONMaxParse = 64 << 10
)

func sanitizeOpenAIUpstreamDiagnosticText(text string) string {
	if text == "" {
		return text
	}
	text = sanitizeUpstreamErrorMessage(text)
	text = redactOpenAISensitiveDiagnosticEscapedJSONFields(text)
	text = openAISensitiveDiagnosticJSONFieldRe.ReplaceAllString(text, `$1"[redacted]"$2`)
	text = redactOpenAISensitiveDiagnosticJSONCompositeFields(text)
	text = redactOpenAISensitiveDiagnosticJSONRemainderFields(text)
	text = redactOpenAISensitiveDiagnosticEscapedJSONFields(text)
	text = openAISensitiveDiagnosticBearerRe.ReplaceAllString(text, "Bearer [redacted]")
	text = openAISensitiveDiagnosticCodexUARe.ReplaceAllString(text, "[codex-user-agent-redacted]")
	text = openAISensitiveDiagnosticKVFieldRe.ReplaceAllString(text, `$1[redacted]`)
	text = openAISensitiveDiagnosticUUIDRe.ReplaceAllString(text, "[uuid-redacted]")
	return text
}

func redactOpenAISensitiveDiagnosticJSONCompositeFields(text string) string {
	matches := openAISensitiveDiagnosticJSONKeyRe.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return text
	}

	var b strings.Builder
	last := 0
	changed := false
	for _, match := range matches {
		if match[0] < last || match[1] >= len(text) {
			continue
		}
		if openAISensitiveDiagnosticIsEscapedJSONQuote(text, match[0]) {
			continue
		}
		if text[match[1]] != '[' && text[match[1]] != '{' {
			continue
		}
		valueEnd, ok := openAISensitiveDiagnosticJSONCompositeEnd(text, match[1])
		b.WriteString(text[last:match[1]])
		b.WriteString(`"[redacted]"`)
		changed = true
		if !ok {
			return b.String()
		}
		last = valueEnd
	}
	if !changed {
		return text
	}
	b.WriteString(text[last:])
	return b.String()
}

func openAISensitiveDiagnosticJSONCompositeEnd(text string, start int) (int, bool) {
	stack := make([]byte, 0, openAISensitiveDiagnosticJSONMaxDepth)
	inString := false
	escaped := false
	for i := start; i < len(text) && i-start <= openAISensitiveDiagnosticJSONCompositeMaxScan; i++ {
		if inString {
			switch {
			case escaped:
				escaped = false
			case text[i] == '\\':
				escaped = true
			case text[i] == '"':
				inString = false
			}
			continue
		}

		switch text[i] {
		case '"':
			inString = true
		case '{':
			if len(stack) >= openAISensitiveDiagnosticJSONMaxDepth {
				return 0, false
			}
			stack = append(stack, '}')
		case '[':
			if len(stack) >= openAISensitiveDiagnosticJSONMaxDepth {
				return 0, false
			}
			stack = append(stack, ']')
		case '}', ']':
			if len(stack) == 0 || stack[len(stack)-1] != text[i] {
				return 0, false
			}
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				return i + 1, true
			}
		}
	}
	return 0, false
}

func sanitizeOpenAIUpstreamDiagnosticBodyForLog(body []byte, maxBytes int) string {
	if len(body) == 0 {
		return ""
	}
	if maxBytes <= 0 {
		maxBytes = 2048
	}
	text, ok := sanitizeOpenAIUpstreamDiagnosticJSONBodyForLog(body)
	if !ok {
		text = sanitizeOpenAIUpstreamDiagnosticText(string(body))
	}
	// Keep diagnostic bodies single-line for logs/support details after redaction.
	text = strings.ReplaceAll(text, "\r", " ")
	text = strings.ReplaceAll(text, "\n", " ")
	return truncateString(text, maxBytes)
}

func redactOpenAISensitiveDiagnosticJSONRemainderFields(text string) string {
	matches := openAISensitiveDiagnosticJSONKeyRe.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return text
	}

	var b strings.Builder
	last := 0
	changed := false
	for _, match := range matches {
		if match[0] < last {
			continue
		}
		if openAISensitiveDiagnosticIsEscapedJSONQuote(text, match[0]) {
			continue
		}
		valueStart := match[1]
		b.WriteString(text[last:valueStart])
		b.WriteString(`"[redacted]"`)
		changed = true
		valueEnd, ok := openAISensitiveDiagnosticJSONValueEnd(text, valueStart)
		if !ok {
			return b.String()
		}
		last = valueEnd
	}
	if !changed {
		return text
	}
	b.WriteString(text[last:])
	return b.String()
}

func redactOpenAISensitiveDiagnosticEscapedJSONFields(text string) string {
	matches := openAISensitiveDiagnosticEscapedJSONKeyRe.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return text
	}

	var b strings.Builder
	last := 0
	changed := false
	for _, match := range matches {
		if match[0] < last {
			continue
		}
		valueStart := match[1]
		b.WriteString(text[last:valueStart])
		b.WriteString(`\"[redacted]\"`)
		changed = true
		valueEnd, ok := openAISensitiveDiagnosticEscapedJSONValueEnd(text, valueStart)
		if !ok {
			return b.String()
		}
		last = valueEnd
	}
	if !changed {
		return text
	}
	b.WriteString(text[last:])
	return b.String()
}

func openAISensitiveDiagnosticJSONValueEnd(text string, start int) (int, bool) {
	if start >= len(text) {
		return 0, false
	}

	var (
		valueEnd int
		ok       bool
	)
	switch text[start] {
	case '"':
		valueEnd, ok = openAISensitiveDiagnosticQuotedStringEnd(text, start)
	case '\\':
		if start+1 < len(text) && text[start+1] == '"' {
			valueEnd, ok = openAISensitiveDiagnosticEscapedQuotedStringEnd(text, start)
		}
	case '{', '[':
		valueEnd, ok = openAISensitiveDiagnosticJSONCompositeEnd(text, start)
	default:
		valueEnd, ok = openAISensitiveDiagnosticUnknownScalarEnd(text, start)
	}
	if !ok {
		return 0, false
	}
	if !openAISensitiveDiagnosticValueHasJSONDelimiter(text, valueEnd) {
		return 0, false
	}
	return valueEnd, true
}

func openAISensitiveDiagnosticQuotedStringEnd(text string, start int) (int, bool) {
	escaped := false
	for i := start + 1; i < len(text); i++ {
		switch {
		case escaped:
			escaped = false
		case text[i] == '\\':
			escaped = true
		case text[i] == '"':
			return i + 1, true
		}
	}
	return 0, false
}

func openAISensitiveDiagnosticEscapedQuotedStringEnd(text string, start int) (int, bool) {
	for i := start + 2; i < len(text); i++ {
		if text[i] == '"' && openAISensitiveDiagnosticIsEscapedJSONQuote(text, i) {
			return i + 1, true
		}
	}
	return 0, false
}

func openAISensitiveDiagnosticUnknownScalarEnd(text string, start int) (int, bool) {
	for i := start; i < len(text); i++ {
		switch text[i] {
		case ',', '}', ']':
			return i, true
		}
	}
	return len(text), true
}

func openAISensitiveDiagnosticEscapedJSONValueEnd(text string, start int) (int, bool) {
	if start >= len(text) {
		return 0, false
	}

	var (
		valueEnd int
		ok       bool
	)
	switch text[start] {
	case '\\':
		if start+1 < len(text) && text[start+1] == '"' {
			valueEnd, ok = openAISensitiveDiagnosticEscapedQuotedStringEnd(text, start)
		}
	case '{', '[':
		valueEnd, ok = openAISensitiveDiagnosticEscapedJSONCompositeEnd(text, start)
	default:
		valueEnd, ok = openAISensitiveDiagnosticUnknownScalarEnd(text, start)
	}
	if !ok {
		return 0, false
	}
	if !openAISensitiveDiagnosticValueHasJSONDelimiter(text, valueEnd) {
		return 0, false
	}
	return valueEnd, true
}

func openAISensitiveDiagnosticValueHasJSONDelimiter(text string, valueEnd int) bool {
	for i := valueEnd; i < len(text); i++ {
		switch text[i] {
		case ' ', '\t', '\r', '\n':
			continue
		case ',', '}', ']':
			return true
		default:
			return false
		}
	}
	return true
}

func openAISensitiveDiagnosticEscapedJSONCompositeEnd(text string, start int) (int, bool) {
	stack := make([]byte, 0, openAISensitiveDiagnosticJSONMaxDepth)
	inString := false
	for i := start; i < len(text) && i-start <= openAISensitiveDiagnosticJSONCompositeMaxScan; i++ {
		if inString {
			if text[i] == '"' && openAISensitiveDiagnosticIsEscapedJSONQuote(text, i) {
				inString = false
			}
			continue
		}

		if text[i] == '"' && openAISensitiveDiagnosticIsEscapedJSONQuote(text, i) {
			inString = true
			continue
		}

		switch text[i] {
		case '{':
			if len(stack) >= openAISensitiveDiagnosticJSONMaxDepth {
				return 0, false
			}
			stack = append(stack, '}')
		case '[':
			if len(stack) >= openAISensitiveDiagnosticJSONMaxDepth {
				return 0, false
			}
			stack = append(stack, ']')
		case '}', ']':
			if len(stack) == 0 || stack[len(stack)-1] != text[i] {
				return 0, false
			}
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				return i + 1, true
			}
		}
	}
	return 0, false
}

func openAISensitiveDiagnosticIsEscapedJSONQuote(text string, quoteIndex int) bool {
	backslashes := 0
	for i := quoteIndex - 1; i >= 0 && text[i] == '\\'; i-- {
		backslashes++
	}
	return backslashes%4 == 1
}

func sanitizeOpenAIUpstreamDiagnosticJSONBodyForLog(body []byte) (string, bool) {
	if len(body) > openAISensitiveDiagnosticJSONBodyMaxParse {
		return "", false
	}
	if !json.Valid(body) {
		return "", false
	}

	var value any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return "", false
	}

	redacted := redactOpenAISensitiveDiagnosticJSONValue(value, 0)
	encoded, err := json.Marshal(redacted)
	if err != nil {
		return "", false
	}
	return string(encoded), true
}

func redactOpenAISensitiveDiagnosticJSONValue(value any, depth int) any {
	if depth > openAISensitiveDiagnosticJSONMaxDepth {
		return "[redacted]"
	}

	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, val := range v {
			if isOpenAISensitiveDiagnosticField(key) {
				out[key] = "[redacted]"
				continue
			}
			out[key] = redactOpenAISensitiveDiagnosticJSONValue(val, depth+1)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = redactOpenAISensitiveDiagnosticJSONValue(item, depth+1)
		}
		return out
	case string:
		return sanitizeOpenAISensitiveDiagnosticJSONString(v, depth)
	default:
		return value
	}
}

func sanitizeOpenAISensitiveDiagnosticJSONString(value string, depth int) string {
	if depth >= openAISensitiveDiagnosticJSONMaxDepth {
		return sanitizeOpenAIUpstreamDiagnosticText(value)
	}

	trimmed := strings.TrimSpace(value)
	if len(trimmed) > 0 && len(trimmed) <= openAISensitiveDiagnosticEmbeddedJSONMaxParse && (trimmed[0] == '{' || trimmed[0] == '[') && json.Valid([]byte(trimmed)) {
		var embedded any
		decoder := json.NewDecoder(strings.NewReader(trimmed))
		decoder.UseNumber()
		if err := decoder.Decode(&embedded); err == nil {
			redacted := redactOpenAISensitiveDiagnosticJSONValue(embedded, depth+1)
			if encoded, err := json.Marshal(redacted); err == nil {
				return string(encoded)
			}
		}
	}

	return sanitizeOpenAIUpstreamDiagnosticText(value)
}

func isOpenAISensitiveDiagnosticField(key string) bool {
	_, ok := openAISensitiveDiagnosticFieldSet[strings.ToLower(strings.TrimSpace(key))]
	return ok
}

func buildOpenAISensitiveDiagnosticFieldSet(keys []string) map[string]struct{} {
	set := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		normalized := strings.ToLower(strings.TrimSpace(key))
		if normalized == "" {
			continue
		}
		set[normalized] = struct{}{}
	}
	return set
}
