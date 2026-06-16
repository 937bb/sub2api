package service

import (
	"regexp"
	"strings"
)

var (
	openAISensitiveDiagnosticJSONFieldRe = regexp.MustCompile(`(?i)("(?:x-codex-installation-id|x-codex-window-id|session-id|thread-id|x-client-request-id|installation_id|thread_id|window_id|prompt_cache_key|session_id|conversation_id|raw_user_agent|user_agent|user-agent|authorization|access_token|refresh_token|api_key)"\s*:\s*)"[^"]*"`)
	openAISensitiveDiagnosticKVFieldRe   = regexp.MustCompile(`(?i)\b((?:x-codex-installation-id|x-codex-window-id|session-id|thread-id|x-client-request-id|installation_id|thread_id|window_id|prompt_cache_key|session_id|conversation_id|raw_user_agent|user_agent|user-agent|authorization|access_token|refresh_token|api_key)\s*(?:=|:)\s*)(?:Bearer\s+)?[^\s,;"}]+`)
	openAISensitiveDiagnosticBearerRe    = regexp.MustCompile(`(?i)\bBearer\s+[^\s,;"}]+`)
	openAISensitiveDiagnosticUUIDRe      = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)
	openAISensitiveDiagnosticCodexUARe   = regexp.MustCompile(`(?i)codex-tui/[^\s";]+\s+\([^"\)]*\)\s+[^\s"]+\s+\(codex-tui;\s*[^\)]*\)`)
)

func sanitizeOpenAIUpstreamDiagnosticText(text string) string {
	if text == "" {
		return text
	}
	text = sanitizeUpstreamErrorMessage(text)
	text = openAISensitiveDiagnosticJSONFieldRe.ReplaceAllString(text, `$1"[redacted]"`)
	text = openAISensitiveDiagnosticBearerRe.ReplaceAllString(text, "Bearer [redacted]")
	text = openAISensitiveDiagnosticCodexUARe.ReplaceAllString(text, "[codex-user-agent-redacted]")
	text = openAISensitiveDiagnosticKVFieldRe.ReplaceAllString(text, `$1[redacted]`)
	text = openAISensitiveDiagnosticUUIDRe.ReplaceAllString(text, "[uuid-redacted]")
	return text
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
