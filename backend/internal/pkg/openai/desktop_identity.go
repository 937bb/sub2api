package openai

// Desktop identity observed in ChatGPT.app on 2026-11-04. The engine and
// application versions belong to separate release streams. See
// docs/chatgpt-desktop-fingerprint.md for artifact hashes and probe results.
const (
	CodexDesktopOriginator      = "Codex Desktop"
	CodexDesktopEngineVersion   = "0.166.0-alpha.6"
	CodexDesktopAppVersion      = "26.1029.61003"
	CodexDesktopUserAgentSuffix = " (Mac OS 26.6.2; arm64) unknown (Codex Desktop; " + CodexDesktopAppVersion + ")"
)
