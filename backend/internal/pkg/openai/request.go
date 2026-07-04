package openai

import "strings"

// CodexCLIUserAgentPrefixes matches Codex CLI User-Agent patterns
// Examples: "codex_vscode/1.0.0", "codex_cli_rs/0.1.2"
var CodexCLIUserAgentPrefixes = []string{
	"codex_vscode/",
	"codex_cli_rs/",
}

// CodexOfficialClientUserAgentPrefixes matches Codex 官方客户端家族 User-Agent 前缀。
// 该列表仅用于 OpenAI OAuth `codex_cli_only` 访问限制判定。
var CodexOfficialClientUserAgentPrefixes = []string{
	"codex_cli_rs/",
	"codex-tui/",
	"codex_vscode/",
	"codex_app/",
	"codex_chatgpt_desktop/",
	"codex_atlas/",
	"codex_exec/",
	"codex_sdk_ts/",
	"codex ",
}

// CodexOfficialClientStrictUserAgentPrefixes matches official Codex client UA
// prefixes for the OpenAI OAuth codex_cli_only detector. Matching is
// prefix-only; the "Codex " family intentionally keeps its trailing space.
var CodexOfficialClientStrictUserAgentPrefixes = []string{
	"codex_cli_rs/",
	"codex-tui/",
	"codex_vscode/",
	"codex_app/",
	"codex_chatgpt_desktop/",
	"codex_atlas/",
	"codex_exec/",
	"codex_sdk_ts/",
	"Codex ",
}

// CodexOfficialClientOriginatorPrefixes matches Codex 官方客户端家族 originator 前缀。
// 说明：OpenAI 官方 Codex 客户端并不只使用固定的 codex_app 标识。
// 例如 codex_cli_rs、codex_vscode、codex_chatgpt_desktop、codex_atlas、codex_exec、codex_sdk_ts 等。
var CodexOfficialClientOriginatorPrefixes = []string{
	"codex_",
	"codex ",
}

// CodexOfficialClientStrictOriginators matches exact known official Codex
// originators for the OpenAI OAuth codex_cli_only detector. The separate
// "Codex " family is accepted by literal prefix in
// IsCodexOfficialClientOriginatorStrict.
var CodexOfficialClientStrictOriginators = []string{
	"codex_cli_rs",
	"codex-tui",
	"codex_vscode",
	"codex_app",
	"codex_chatgpt_desktop",
	"codex_atlas",
	"codex_exec",
	"codex_sdk_ts",
}

// IsBrowserUserAgent 判断 User-Agent 是否来自浏览器（Chrome/Firefox/Safari/Edge/Opera 等）。
// 所有现代浏览器的 UA 均以 "Mozilla/" 作为前缀，CLI 工具（codex/claude/curl/postman/python-requests 等）不会。
// 该判定用于避免 Cloudflare 对浏览器型 UA 在 OpenAI 上游接口上触发 JS 质询。
func IsBrowserUserAgent(userAgent string) bool {
	ua := strings.TrimSpace(userAgent)
	if ua == "" {
		return false
	}
	return strings.HasPrefix(strings.ToLower(ua), "mozilla/")
}

// IsCodexCLIRequest checks if the User-Agent indicates a Codex CLI request
func IsCodexCLIRequest(userAgent string) bool {
	ua := normalizeCodexClientHeader(userAgent)
	if ua == "" {
		return false
	}
	return matchCodexClientHeaderPrefixes(ua, CodexCLIUserAgentPrefixes)
}

// IsCodexOfficialClientRequest checks if the User-Agent indicates a Codex 官方客户端请求。
// 与 IsCodexCLIRequest 解耦，避免影响历史兼容逻辑。
func IsCodexOfficialClientRequest(userAgent string) bool {
	ua := normalizeCodexClientHeader(userAgent)
	if ua == "" {
		return false
	}
	return matchCodexClientHeaderPrefixes(ua, CodexOfficialClientUserAgentPrefixes)
}

// IsCodexOfficialClientRequestStrict checks official Codex User-Agent identity
// for the codex_cli_only detector. It only matches known prefixes at the start
// of the header and deliberately does not use substring fallback.
func IsCodexOfficialClientRequestStrict(userAgent string) bool {
	ua := strings.TrimSpace(userAgent)
	if ua == "" {
		return false
	}
	for _, prefix := range CodexOfficialClientStrictUserAgentPrefixes {
		if prefix == "" {
			continue
		}
		if prefix == "Codex " {
			if strings.HasPrefix(ua, prefix) {
				return true
			}
			continue
		}
		if hasPrefixFold(ua, prefix) {
			return true
		}
	}
	return false
}

// IsCodexOfficialClientOriginator checks if originator indicates a Codex 官方客户端请求。
func IsCodexOfficialClientOriginator(originator string) bool {
	v := normalizeCodexClientHeader(originator)
	if v == "" {
		return false
	}
	return matchCodexClientHeaderPrefixes(v, CodexOfficialClientOriginatorPrefixes)
}

// IsCodexOfficialClientOriginatorStrict checks official Codex originator
// identity for the codex_cli_only detector. It accepts exact known originators
// and the literal "Codex " family only.
func IsCodexOfficialClientOriginatorStrict(originator string) bool {
	v := strings.TrimSpace(originator)
	if v == "" {
		return false
	}
	if strings.HasPrefix(v, "Codex ") {
		return true
	}
	for _, known := range CodexOfficialClientStrictOriginators {
		if strings.EqualFold(v, known) {
			return true
		}
	}
	return false
}

// IsCodexOfficialClientByHeaders checks whether the request headers indicate an
// official Codex client family request.
func IsCodexOfficialClientByHeaders(userAgent, originator string) bool {
	return IsCodexOfficialClientRequest(userAgent) || IsCodexOfficialClientOriginator(originator)
}

// IsCodexOfficialClientByHeadersStrict checks whether headers indicate an
// official Codex client for codex_cli_only detector use.
func IsCodexOfficialClientByHeadersStrict(userAgent, originator string) bool {
	return IsCodexOfficialClientRequestStrict(userAgent) || IsCodexOfficialClientOriginatorStrict(originator)
}

func normalizeCodexClientHeader(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func matchCodexClientHeaderPrefixes(value string, prefixes []string) bool {
	for _, prefix := range prefixes {
		normalizedPrefix := normalizeCodexClientHeader(prefix)
		if normalizedPrefix == "" {
			continue
		}
		// 优先前缀匹配；若 UA/Originator 被网关拼接为复合字符串时，退化为包含匹配。
		if strings.HasPrefix(value, normalizedPrefix) || strings.Contains(value, normalizedPrefix) {
			return true
		}
	}
	return false
}

func hasPrefixFold(value, prefix string) bool {
	if len(value) < len(prefix) {
		return false
	}
	return strings.EqualFold(value[:len(prefix)], prefix)
}
