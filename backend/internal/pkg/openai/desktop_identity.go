package openai

// Desktop identity observed in ChatGPT.app on 2026-11-03. The engine and
// application versions belong to separate release streams. See
// docs/chatgpt-desktop-fingerprint.md for artifact hashes and probe results.
const (
	CodexDesktopOriginator      = "Codex Desktop"
	CodexDesktopEngineVersion   = "0.166.0-alpha.5.1"
	CodexDesktopAppVersion      = "26.1027.51404"
	CodexDesktopUserAgentSuffix = " (Mac OS 26.6.2; arm64) unknown (Codex Desktop; " + CodexDesktopAppVersion + ")"
)
