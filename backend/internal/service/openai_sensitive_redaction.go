package service

import (
	"regexp"
	"strings"
)

var (
	openAISensitiveDiagnosticFieldPattern = `x-codex-installation-id|x-codex-window-id|session-id|thread-id|x-client-request-id|installation_id|thread_id|window_id|prompt_cache_key|session_id|conversation_id|raw_user_agent|user_agent|user-agent|authorization|access_token|refresh_token|id_token|session_token|api_key|apikey|token|personal_access_token|email|chatgpt_user_id|chatgpt_account_id|chatgpt-account-id|chatgpt_plan_type|chatgpt_account_is_fedramp|x-openai-fedramp`
	openAISensitiveDiagnosticJSONFieldRe  = regexp.MustCompile(`(?i)("(?:` + openAISensitiveDiagnosticFieldPattern + `)"\s*:\s*)(?:"(?:\\.|[^"\\])*"|true|false|null|-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)`)
	openAISensitiveDiagnosticJSONKeyRe    = regexp.MustCompile(`(?i)"(?:` + openAISensitiveDiagnosticFieldPattern + `)"\s*:\s*`)
	openAISensitiveDiagnosticKVFieldRe    = regexp.MustCompile(`(?i)\b((?:` + openAISensitiveDiagnosticFieldPattern + `)\s*(?:=|:)\s*)(?:Bearer\s+)?(?:"(?:\\.|[^"\\])*"|[^\s,;"}]+)`)
	openAISensitiveDiagnosticBearerRe     = regexp.MustCompile(`(?i)\bBearer\s+[^\s,;"}]+`)
	openAISensitiveDiagnosticUUIDRe       = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)
	openAISensitiveDiagnosticCodexUARe    = regexp.MustCompile(`(?i)codex-tui/[^\s";]+\s+\([^"\)]*\)\s+[^\s"]+\s+\(codex-tui;\s*[^\)]*\)`)
)

const (
	openAISensitiveDiagnosticJSONCompositeMaxScan = 8192
	openAISensitiveDiagnosticJSONMaxDepth         = 16
)

func sanitizeOpenAIUpstreamDiagnosticText(text string) string {
	if text == "" {
		return text
	}
	text = sanitizeUpstreamErrorMessage(text)
	text = openAISensitiveDiagnosticJSONFieldRe.ReplaceAllString(text, `$1"[redacted]"`)
	text = redactOpenAISensitiveDiagnosticJSONCompositeFields(text)
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
	text := sanitizeOpenAIUpstreamDiagnosticText(string(body))
	// Keep diagnostic bodies single-line for logs/support details after redaction.
	text = strings.ReplaceAll(text, "\r", " ")
	text = strings.ReplaceAll(text, "\n", " ")
	return truncateString(text, maxBytes)
}
