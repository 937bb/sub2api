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
	"codex_vscode_copilot/",
	"codex_app/",
	"codex_chatgpt_desktop/",
	"codex_atlas/",
	"codex_exec/",
	"codex_sdk_ts/",
}

// CodexOfficialClientStrictUserAgentPrefixes matches official Codex client UA
// prefixes for the OpenAI OAuth codex_cli_only detector. Matching is prefix-only.
// The literal "Codex " family is handled separately so the trailing space cannot
// be trimmed into a broad "codex" prefix.
var CodexOfficialClientStrictUserAgentPrefixes = []string{
	"codex_cli_rs/",
	"codex-tui/",
	"codex_vscode/",
	"codex_vscode_copilot/",
	"codex_app/",
	"codex_chatgpt_desktop/",
	"codex_atlas/",
	"codex_exec/",
	"codex_sdk_ts/",
}

// CodexOfficialClientOriginatorPrefixes is retained for compatibility with older
// call sites, but values are exact known official originators rather than broad
// prefixes. Originator classification must not accept arbitrary codex_* strings.
var CodexOfficialClientOriginatorPrefixes = []string{
	"codex_cli_rs",
	"codex-tui",
	"codex_vscode",
	"codex_vscode_copilot",
	"codex_app",
	"codex_chatgpt_desktop",
	"codex_atlas",
	"codex_exec",
	"codex_sdk_ts",
}

// CodexOfficialClientStrictOriginators matches exact known official Codex
// originators for the OpenAI OAuth codex_cli_only detector. The separate
// "Codex " family is accepted by literal prefix in
// IsCodexOfficialClientOriginatorStrict.
var CodexOfficialClientStrictOriginators = []string{
	"codex_cli_rs",
	"codex-tui",
	"codex_vscode",
	"codex_vscode_copilot",
	"codex_app",
	"codex_chatgpt_desktop",
	"codex_atlas",
	"codex_exec",
	"codex_sdk_ts",
}

const (
	codexOfficialClientFamilyPrefix = "Codex "
	codexUATrailerScanLimit         = 256
)

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
	return isCodexOfficialClientRequest(userAgent, false)
}

// IsCodexOfficialClientRequestStrict checks official Codex User-Agent identity
// for the codex_cli_only detector. It only matches known prefixes at the start
// of the header, the literal "Codex " family, or the bounded codex-rs trailer
// fallback. It deliberately does not use substring fallback.
func IsCodexOfficialClientRequestStrict(userAgent string) bool {
	return isCodexOfficialClientRequest(userAgent, true)
}

func isCodexOfficialClientRequest(userAgent string, strict bool) bool {
	ua := strings.TrimSpace(userAgent)
	if ua == "" {
		return false
	}
	if strict {
		if matchCodexClientHeaderStrictPrefixes(ua, CodexOfficialClientStrictUserAgentPrefixes) {
			return true
		}
	} else if matchCodexClientHeaderPrefixes(normalizeCodexClientHeader(ua), CodexOfficialClientUserAgentPrefixes) {
		return true
	}
	if strings.HasPrefix(ua, codexOfficialClientFamilyPrefix) {
		return true
	}
	if strict && IsBrowserUserAgent(ua) {
		return false
	}
	if trailerName := codexUAOfficialTrailerName(ua); trailerName != "" {
		return isCodexOfficialClientOriginator(trailerName)
	}
	return false
}

// IsCodexOfficialClientOriginator checks if originator indicates a Codex 官方客户端请求。
func IsCodexOfficialClientOriginator(originator string) bool {
	return isCodexOfficialClientOriginator(originator)
}

// IsCodexOfficialClientOriginatorStrict checks official Codex originator
// identity for the codex_cli_only detector. It accepts exact known originators
// and the literal "Codex " family only.
func IsCodexOfficialClientOriginatorStrict(originator string) bool {
	return isCodexOfficialClientOriginator(originator)
}

func isCodexOfficialClientOriginator(originator string) bool {
	v := strings.TrimSpace(originator)
	if v == "" {
		return false
	}
	if strings.HasPrefix(v, codexOfficialClientFamilyPrefix) {
		return true
	}
	for _, known := range CodexOfficialClientStrictOriginators {
		if strings.EqualFold(v, known) {
			return true
		}
	}
	return false
}

func codexUAOfficialTrailerName(userAgent string) string {
	ua := strings.TrimSpace(userAgent)
	if ua == "" || !strings.HasSuffix(ua, ")") {
		return ""
	}
	if len(ua) > codexUATrailerScanLimit {
		ua = ua[len(ua)-codexUATrailerScanLimit:]
	}
	closeIdx := strings.LastIndex(ua, ")")
	if closeIdx != len(ua)-1 {
		return ""
	}
	openIdx := strings.LastIndex(ua[:closeIdx], "(")
	if openIdx < 0 {
		return ""
	}
	inner := strings.TrimSpace(ua[openIdx+1 : closeIdx])
	semiIdx := strings.Index(inner, ";")
	if semiIdx <= 0 || strings.Contains(inner[semiIdx+1:], ";") {
		return ""
	}
	name := strings.TrimSpace(inner[:semiIdx])
	version := strings.TrimSpace(inner[semiIdx+1:])
	if name == "" || version == "" || strings.ContainsAny(name, "()\r\n") || strings.ContainsAny(version, "()\r\n") {
		return ""
	}
	return name
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

func matchCodexClientHeaderStrictPrefixes(value string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if prefix == "" {
			continue
		}
		if hasPrefixFold(value, prefix) {
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
