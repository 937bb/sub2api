package openai

// Desktop identity observed in ChatGPT.app on 2026-11-15. The engine and
// application versions belong to separate release streams. See
// docs/chatgpt-desktop-fingerprint.md for artifact hashes and probe results.
const (
	CodexDesktopOriginator      = "Codex Desktop"
	CodexDesktopEngineVersion   = "0.167.0-alpha.3.2"
	CodexDesktopAppVersion      = "26.1103.31037"
	CodexDesktopUserAgentSuffix = " (Mac OS 26.6.2; arm64) unknown (Codex Desktop; " + CodexDesktopAppVersion + ")"
)

// IsCodexDesktopOriginator reports identities emitted by the Desktop host and
// its Work/local executor. Both use the bundled native inference engine.
func IsCodexDesktopOriginator(originator string) bool {
	return originator == CodexDesktopOriginator || originator == CodexWorkDesktopOriginator
}
