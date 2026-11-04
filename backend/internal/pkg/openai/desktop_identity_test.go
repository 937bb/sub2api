package openai

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDesktopIdentityPreservesApplicationVersion(t *testing.T) {
	observed := "Codex Desktop/0.166.0-alpha.6 (Mac OS 26.6.2; arm64) unknown (Codex Desktop; 26.1029.61003)"
	require.Equal(t, observed, CodexDesktopOriginator+"/"+CodexDesktopEngineVersion+CodexDesktopUserAgentSuffix)
	originator, paired, ok := PairCodexClientIdentity(observed)
	require.True(t, ok)
	require.Equal(t, "Codex Desktop", originator)
	require.Equal(t, observed, paired)
	require.Equal(t, "0.166.0-alpha.6", CodexUserAgentVersion(observed))
	require.Equal(t, "Codex Desktop/0.200.1 (Mac OS 26.6.2; arm64) unknown (Codex Desktop; 26.1029.61003)", SetCodexUserAgentVersion(observed, "0.200.1"))
}

func TestEngineRewriteDoesNotChangeIndependentClientVersions(t *testing.T) {
	for _, client := range []string{"Codex Desktop", "codex_vscode", "codex_vscode_copilot", "codex_sdk_ts", "codex_chatgpt_desktop"} {
		t.Run(client, func(t *testing.T) {
			// Even coincidentally equal versions belong to independent products.
			ua := client + "/0.166.0 (Mac OS 26.6.2; arm64) unknown (" + client + "; 0.166.0)"
			want := client + "/0.200.1 (Mac OS 26.6.2; arm64) unknown (" + client + "; 0.166.0)"
			require.Equal(t, want, SetCodexUserAgentVersion(ua, "0.200.1"))
		})
	}
	ua := "codex-tui/0.166.0 (Linux; x86_64) dumb (codex-tui; 0.150.0)"
	require.Equal(t, "codex-tui/0.200.1 (Linux; x86_64) dumb (codex-tui; 0.150.0)", SetCodexUserAgentVersion(ua, "0.200.1"))
}
