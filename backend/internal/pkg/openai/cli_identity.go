package openai

// CodexLocalCLIPlatform was measured from the bundled CLI on 2026-11-03.
const CodexLocalCLIPlatform = " (Mac OS 26.6.2; arm64) unknown"

// IsLocalCLIUserAgent selects the native Rust transport captured from the bundled
// engine. The same engine services both codex_exec and Codex Desktop; only the
// application identity differs.
func IsLocalCLIUserAgent(ua string) bool {
	version := CodexUserAgentVersion(ua)
	if version == "" {
		return false
	}
	return ua == CodexExecOriginator+"/"+version+CodexLocalCLIPlatform+" (codex_exec; "+version+")" ||
		ua == CodexDesktopOriginator+"/"+version+CodexDesktopUserAgentSuffix ||
		ua == CodexWorkDesktopOriginator+"/"+version+CodexDesktopUserAgentSuffix
}
