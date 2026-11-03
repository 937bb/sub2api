package openai

// CodexLocalCLIPlatform was measured from the bundled CLI on 2026-11-03.
const CodexLocalCLIPlatform = " (Mac OS 26.6.2; arm64) unknown"

// IsLocalCLIUserAgent limits the measured transport to the observed client family.
// Explicit Linux, Desktop, browser and SDK identities retain their own transport.
func IsLocalCLIUserAgent(ua string) bool {
	version := CodexUserAgentVersion(ua)
	return version != "" && ua == CodexDefaultOriginator+"/"+version+CodexLocalCLIPlatform+" (codex_exec; "+version+")"
}
