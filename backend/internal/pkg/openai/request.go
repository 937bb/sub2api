package openai

import (
	"regexp"
	"strings"

	"golang.org/x/net/http/httpguts"
)

// CodexCLIUserAgentPrefixes matches Codex CLI User-Agent patterns
// Examples: "codex_vscode/1.0.0", "codex_cli_rs/0.1.2"
var CodexCLIUserAgentPrefixes = []string{
	"codex_vscode/",
	"codex_cli_rs/",
}

// Exact Codex client prefixes used by the official-client gate and passthrough.
// The space-bearing Desktop family is handled separately to avoid a broad codex prefix.
var codexOfficialClientUAPrefixes = []string{
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

// Preserve the trailing space in the Codex family prefix. Trimming it would
// incorrectly accept arbitrary client names beginning with codex.
const codexOfficialClientFamilyPrefix = "codex "

// Exact official client originators. app-server uses clientInfo.name;
// do not replace this allowlist with substring matching.
var codexOfficialClientOriginators = map[string]bool{
	"codex_cli_rs":          true, // CLI DEFAULT_ORIGINATOR
	"codex-tui":             true, // Interactive TUI
	"codex_vscode":          true, // VSCode/Cursor extension
	"codex_vscode_copilot":  true, // GitHub Copilot extension integration
	"codex_app":             true, // Legacy identity
	"codex_chatgpt_desktop": true, // is_first_party_chat_originator
	"codex_atlas":           true, // is_first_party_chat_originator
	"codex_exec":            true, // Noninteractive codex exec
	"codex_sdk_ts":          true, // TypeScript SDK
}

// IsCodexCLIRequest checks if the User-Agent indicates a Codex CLI request
func IsCodexCLIRequest(userAgent string) bool {
	ua := normalizeCodexClientHeader(userAgent)
	if ua == "" {
		return false
	}
	return matchCodexClientHeaderPrefixes(ua, CodexCLIUserAgentPrefixes)
}

// IsCodexOfficialClientRequest retains legacy substring matching for passthrough.
func IsCodexOfficialClientRequest(userAgent string) bool {
	return isCodexOfficialClientRequest(userAgent, false)
}

// IsCodexOfficialClientRequestStrict requires a leading prefix, family name,
// or recognized client trailer for the official-client access gate.
func IsCodexOfficialClientRequestStrict(userAgent string) bool {
	return isCodexOfficialClientRequest(userAgent, true)
}

// Match known prefixes, the space-bearing Codex family, then clientInfo.name
// in the final UA group. Originator overrides can change only the leading name.
func isCodexOfficialClientRequest(userAgent string, strict bool) bool {
	ua := normalizeCodexClientHeader(userAgent)
	if ua == "" {
		return false
	}
	if strict {
		if matchCodexClientHeaderStrictPrefixes(ua, codexOfficialClientUAPrefixes) {
			return true
		}
	} else if matchCodexClientHeaderPrefixes(ua, codexOfficialClientUAPrefixes) {
		return true
	}
	if strings.HasPrefix(ua, codexOfficialClientFamilyPrefix) {
		return true
	}
	// Recover an official client name from the final parenthesized group.
	if name := codexUATrailerName(ua); name != "" {
		return IsCodexOfficialClientOriginator(name)
	}
	return false
}

// codexUATrailerName extracts clientInfo.name from the last parenthesized group.
// Originator overrides do not change this trailer. Callers choose whether to
// normalize case; pairing preserves the original case of the Codex family.
func codexUATrailerName(ua string) string {
	last := strings.LastIndex(ua, "(")
	if last < 0 {
		return ""
	}
	rest := ua[last+1:]
	closeIdx := strings.Index(rest, ")")
	if closeIdx < 0 {
		return ""
	}
	inner := strings.TrimSpace(rest[:closeIdx])
	if semi := strings.Index(inner, ";"); semi >= 0 {
		inner = strings.TrimSpace(inner[:semi])
	}
	return inner
}

// IsCodexOfficialClientOriginator matches exact names or the Codex family prefix.
func IsCodexOfficialClientOriginator(originator string) bool {
	v := normalizeCodexClientHeader(originator)
	if v == "" {
		return false
	}
	if codexOfficialClientOriginators[v] {
		return true
	}
	return strings.HasPrefix(v, codexOfficialClientFamilyPrefix)
}

// IsCodexOfficialClientByHeaders checks whether the request headers indicate an
// official Codex client family request.
func IsCodexOfficialClientByHeaders(userAgent, originator string) bool {
	return IsCodexOfficialClientRequest(userAgent) || IsCodexOfficialClientOriginator(originator)
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
		// Keep the legacy substring fallback for composite passthrough headers.
		if strings.HasPrefix(value, normalizedPrefix) || strings.Contains(value, normalizedPrefix) {
			return true
		}
	}
	return false
}

// matchCodexClientHeaderStrictPrefixes accepts only leading prefixes.
// The input must already be lowercased and trimmed.
func matchCodexClientHeaderStrictPrefixes(value string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if p := normalizeCodexClientHeader(prefix); p != "" && strings.HasPrefix(value, p) {
			return true
		}
	}
	return false
}

// PairCodexClientIdentity pairs the UA leading name with its originator.
// Prefer a recognized leading name; otherwise recover the official trailer
// name and replace only the leading name. Reject unrecognized identities.
// Historical upstream reports associate mismatched pairs with HTTP 404.
func PairCodexClientIdentity(userAgent string) (originator string, pairedUA string, ok bool) {
	// Validate before trimming so control bytes cannot become a valid identity.
	if !validCodexUserAgentValue(userAgent) {
		return "", "", false
	}
	ua := strings.TrimSpace(userAgent)
	slash := strings.IndexByte(ua, '/')
	if slash <= 0 {
		return "", "", false
	}
	if leading := strings.TrimSpace(ua[:slash]); isSaneCodexOriginator(leading) && IsCodexOfficialClientOriginator(leading) {
		leading = canonicalizeCodexOriginator(leading)
		return leading, leading + ua[slash:], true
	}
	// Preserve the original family name case. A slash in the trailer would
	// make the rebuilt leading name and originator inconsistent.
	if trailer := codexUATrailerName(ua); trailer != "" && !strings.ContainsRune(trailer, '/') &&
		isSaneCodexOriginator(trailer) && IsCodexOfficialClientOriginator(trailer) {
		trailer = canonicalizeCodexOriginator(trailer)
		return trailer, trailer + ua[slash:], true
	}
	return "", "", false
}

func validCodexUserAgentValue(value string) bool {
	if !httpguts.ValidHeaderFieldValue(value) {
		return false
	}
	// httpguts follows the legacy field-value grammar and permits obs-fold.
	// User-Agent is not an obs-folded header; reject CR/LF before forwarding.
	return !strings.ContainsAny(value, "\r\n")
}

// codexOriginatorMaxLen bounds the printable ASCII client name.
const codexOriginatorMaxLen = 64

// isSaneCodexOriginator rejects excessive length and non-printable ASCII.
func isSaneCodexOriginator(name string) bool {
	if name == "" || len(name) > codexOriginatorMaxLen {
		return false
	}
	for i := 0; i < len(name); i++ {
		if c := name[i]; c < 0x20 || c > 0x7e {
			return false
		}
	}
	return true
}

// canonicalizeCodexOriginator normalizes exact allowlist names to lowercase.
// Preserve case in the space-bearing Codex family.
func canonicalizeCodexOriginator(name string) string {
	if lower := normalizeCodexClientHeader(name); codexOfficialClientOriginators[lower] {
		return lower
	}
	return name
}

// CodexCLIOriginator identifies the historical Rust CLI client.
const CodexCLIOriginator = "codex_cli_rs"

// CodexTUIOriginator identifies the interactive TUI.
const CodexTUIOriginator = "codex-tui"

// CodexDefaultOriginator uses the locally measured noninteractive CLI identity.
const CodexDefaultOriginator = "codex_exec"

// NormalizeCodexClientIdentityToCLI preserves production compatibility for TUI
// profiles by changing the originator and UA prefix together. Engine/platform
// fields are preserved; the TUI trailer is removed from the historical CLI UA.
func NormalizeCodexClientIdentityToCLI(originator, userAgent string) (string, string, bool) {
	if !strings.EqualFold(strings.TrimSpace(originator), CodexTUIOriginator) {
		return originator, userAgent, false
	}

	ua := strings.TrimSpace(userAgent)
	slash := strings.IndexByte(ua, '/')
	if slash <= 0 {
		return CodexCLIOriginator, ua, true
	}

	rest := ua[slash:]
	if trailer := codexUATrailerName(ua); trailer != "" && IsCodexOfficialClientOriginator(trailer) {
		if open := strings.LastIndex(rest, "("); open > 0 {
			rest = strings.TrimRight(rest[:open], " ")
		}
	}
	return CodexCLIOriginator, CodexCLIOriginator + rest, true
}

// CodexUserAgentVersion extracts the complete leading engine version, including
// prerelease suffixes. ParseCodexEngineVersion returns only the numeric triplet.
func CodexUserAgentVersion(userAgent string) string {
	ua := strings.TrimSpace(userAgent)
	slash := strings.IndexByte(ua, '/')
	if slash <= 0 {
		return ""
	}
	rest := ua[slash+1:]
	if space := strings.IndexByte(rest, ' '); space >= 0 {
		rest = rest[:space]
	}
	return strings.TrimSpace(rest)
}

// SetCodexUserAgentVersion changes the leading engine version while preserving
// the platform and independent app-server client version. Native CLI trailers
// that repeat the old engine version are updated together with the prefix.
func SetCodexUserAgentVersion(userAgent, version string) string {
	ua := strings.TrimSpace(userAgent)
	version = strings.TrimSpace(version)
	if version == "" {
		return ""
	}
	slash := strings.IndexByte(ua, '/')
	if slash <= 0 {
		return ""
	}
	client := strings.TrimSpace(ua[:slash])
	if client == "" {
		return ""
	}
	rest := ua[slash+1:]
	tail := ""
	if space := strings.IndexByte(rest, ' '); space >= 0 {
		tail = rest[space:]
	} else if strings.TrimSpace(rest) == "" {
		// A missing version cannot be reconstructed as a valid client identity.
		return ""
	}
	return rewriteCodexUATrailerVersion(client+"/"+version+tail, CodexUserAgentVersion(ua), version)
}

// rewriteCodexUATrailerVersion updates only a native CLI engine-owned trailer.
func rewriteCodexUATrailerVersion(ua, previousVersion, version string) string {
	open := strings.LastIndex(ua, "(")
	if open < 0 {
		return ua
	}
	closeIdx := strings.Index(ua[open+1:], ")")
	if closeIdx < 0 {
		return ua
	}
	inner := ua[open+1 : open+1+closeIdx]
	semi := strings.Index(inner, ";")
	if semi < 0 {
		return ua
	}
	name := strings.TrimSpace(inner[:semi])
	// app-server appends clientInfo.version, which is independent of the Rust
	// engine version for Desktop, extensions, and SDKs. Only native CLI clients
	// with a matching old version have an engine-owned trailer to update.
	switch normalizeCodexClientHeader(name) {
	case "codex_cli_rs", "codex-tui", "codex_exec":
	default:
		return ua
	}
	if strings.TrimSpace(inner[semi+1:]) != previousVersion {
		return ua
	}
	return ua[:open+1] + name + "; " + version + ua[open+1+closeIdx:]
}

// codexEngineVersionPattern extracts the numeric triplet before a prerelease suffix.
var codexEngineVersionPattern = regexp.MustCompile(`^(\d+\.\d+\.\d+)`)

// ParseCodexEngineVersion extracts the Rust engine version from the UA prefix.
func ParseCodexEngineVersion(ua string) (string, bool) {
	ua = strings.TrimSpace(ua)
	slash := strings.IndexByte(ua, '/')
	if slash < 0 {
		return "", false
	}
	rest := ua[slash+1:]
	end := len(rest)
	for i := 0; i < len(rest); i++ {
		if rest[i] == ' ' || rest[i] == '(' {
			end = i
			break
		}
	}
	m := codexEngineVersionPattern.FindString(strings.TrimSpace(rest[:end]))
	if m == "" {
		return "", false
	}
	return m, true
}
