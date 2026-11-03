package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

func TestCodexIdentityPairingRepairsVersionsAndAliases(t *testing.T) {
	SetCodexIdentityEnforcementEnabled(false)
	SetCodexCanonicalUserAgentResolver(nil)
	t.Cleanup(func() { SetCodexIdentityEnforcementEnabled(true); SetCodexCanonicalUserAgentResolver(nil) })
	for _, engine := range []string{"0.145.2", "0.120.0", "invalid"} {
		for _, version := range []string{"", "invalid", "0.120.0", "9.9.9"} {
			t.Run(engine+"/"+version, func(t *testing.T) {
				h := http.Header{"user-agent": {"codex-tui/" + engine + " (Linux; x86_64) term (codex-tui; " + engine + ")"}, "originator": {"codex_cli_rs"}, "version": {version}, "VERSION": {"conflict"}}
				enforceCodexIdentityHeaders(h)
				want := engine
				if engine != "0.145.2" {
					want = codexCLIVersion
				}
				require.Equal(t, want, h.Get("Version"))
				require.Equal(t, want, openai.CodexUserAgentVersion(h.Get("User-Agent")))
				require.Contains(t, h.Get("User-Agent"), "(codex-tui; "+want+")")
				require.Equal(t, "codex-tui", h.Get("Originator"))
				require.Len(t, h, 3)
			})
		}
	}
}

func TestCodexIdentityDoesNotSelectTransportBeta(t *testing.T) {
	for _, beta := range []string{"", openAIWSBetaV1Value, openAIWSBetaV2Value, "assistants=v2"} {
		h := make(http.Header)
		if beta != "" {
			h.Set("OpenAI-Beta", beta)
		}
		ensureCodexIdentityHeaders(h)
		require.Equal(t, beta, h.Get("OpenAI-Beta"))
		h.Set("User-Agent", "unrelated/1.0")
		h.Set("Version", "9.9.9")
		applyOpenAICodexProbeHeaders(h)
		require.Equal(t, beta, h.Get("OpenAI-Beta"))
		require.Equal(t, CodexCanonicalUserAgent(), h.Get("User-Agent"))
		require.Equal(t, CodexCanonicalClientVersion(), h.Get("Version"))
	}
	h := http.Header{"version": {"old"}, "Version": {"conflict"}, "originator": {"old"}, "user-agent": {"old"}}
	ApplyCodexCanonicalAuthIdentity(h)
	require.Len(t, h, 2)
	require.Empty(t, h.Get("Version"))
}

func TestOpenAIWSIdentityChangeDoesNotReuseOldHandshake(t *testing.T) {
	for _, field := range []string{"User-Agent", "Originator", "Version", "OpenAI-Beta", "ChatGPT-Account-ID", "X-OpenAI-Fedramp", "url"} {
		t.Run(field, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 2
			cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 2
			pool := newOpenAIWSConnPool(cfg)
			t.Cleanup(pool.Close)
			dialer := &openAIWSCountingDialer{}
			pool.setClientDialerForTest(dialer)
			req := openAIWSAcquireRequest{Account: &Account{ID: 891, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, WSURL: "wss://example.com/responses", Headers: make(http.Header)}
			ensureCodexIdentityHeaders(req.Headers)
			req.Headers.Set("OpenAI-Beta", openAIWSBetaV1Value)
			first, err := pool.Acquire(context.Background(), req)
			require.NoError(t, err)
			firstID := first.ConnID()
			first.Release()
			changed := cloneOpenAIWSAcquireRequest(req)
			if field == "url" {
				changed.WSURL += "/other"
			} else {
				changed.Headers.Set(field, "changed")
			}
			next, err := pool.Acquire(context.Background(), changed)
			require.NoError(t, err)
			require.False(t, next.Reused())
			require.NotEqual(t, firstID, next.ConnID())
			next.Release()
			reused, err := pool.Acquire(context.Background(), changed)
			require.NoError(t, err)
			require.True(t, reused.Reused())
			reused.Release()
			require.Equal(t, 2, dialer.DialCount())
		})
	}
}

func TestCodexQuotaUsesConfiguredIdentity(t *testing.T) {
	SetCodexCanonicalUserAgentResolver(func() string { return "codex_cli_rs/0.200.1 (Linux; x86_64) unknown" })
	t.Cleanup(func() { SetCodexCanonicalUserAgentResolver(nil) })
	h := buildCodexCommonHeaders("fixture", "account", true)
	require.Equal(t, "codex_cli_rs", h["originator"])
	require.Equal(t, CodexCanonicalUserAgent(), h["user-agent"])
	require.Equal(t, "codex-1", h["openai-beta"])
	require.Equal(t, "true", h["x-openai-fedramp"])
	require.NotContains(t, h, "sec-fetch-mode")
	require.NotContains(t, h, "version")
}

func TestCodexLivePreservesAccountProfile(t *testing.T) {
	const desktop = "Codex Desktop/0.150.0 (Mac OS; arm64) unknown (Codex Desktop; 26.10.1)"
	h := make(http.Header)
	applyLiveUpstreamIdentityHeaders(h, desktop)
	require.Equal(t, "Codex Desktop", h.Get("Originator"))
	require.Equal(t, "quicksilver=v2", h.Get("OpenAI-Alpha"))
	require.Empty(t, h.Get("OpenAI-Beta"))
}

// Exercise serialization and an actual local upgrade, not just header maps.
func TestCodexIdentityOnHTTPAndWebSocketWire(t *testing.T) {
	for _, websocket := range []bool{false, true} {
		t.Run(map[bool]string{false: "http", true: "ws"}[websocket], func(t *testing.T) {
			captured := make(chan http.Header, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				captured <- r.Header.Clone()
				if websocket {
					conn, err := coderws.Accept(w, r, nil)
					if err != nil {
						return
					}
					_ = conn.CloseNow()
				} else {
					w.WriteHeader(http.StatusNoContent)
				}
			}))
			defer server.Close()
			headers := http.Header{"user-agent": {"stale"}, "originator": {"stale"}, "version": {"stale"}}
			enforceCodexIdentityHeaders(headers)
			if websocket {
				headers.Set("OpenAI-Beta", openAIWSBetaV2Value)
				conn, _, _, err := newDefaultOpenAIWSClientDialer().Dial(context.Background(), "ws"+strings.TrimPrefix(server.URL, "http"), headers, "")
				require.NoError(t, err)
				_ = conn.Close()
			} else {
				req, err := http.NewRequest(http.MethodGet, server.URL, nil)
				require.NoError(t, err)
				req.Header = headers
				resp, err := server.Client().Do(req)
				require.NoError(t, err)
				_ = resp.Body.Close()
			}
			got := <-captured
			require.Equal(t, []string{CodexCanonicalUserAgent()}, got.Values("User-Agent"))
			require.Equal(t, []string{openai.CodexDefaultOriginator}, got.Values("Originator"))
			require.Equal(t, []string{CodexCanonicalClientVersion()}, got.Values("Version"))
			if websocket {
				require.Equal(t, openAIWSBetaV2Value, got.Get("OpenAI-Beta"))
			} else {
				require.Empty(t, got.Get("OpenAI-Beta"))
			}
		})
	}
}

func TestCodexIdentityMissingEngineFallsBack(t *testing.T) {
	for _, enforce := range []bool{true, false} {
		SetCodexIdentityEnforcementEnabled(enforce)
		t.Cleanup(func() { SetCodexIdentityEnforcementEnabled(true); SetCodexCanonicalUserAgentResolver(nil) })
		SetCodexCanonicalUserAgentResolver(func() string { return "codex-tui/0.200.1 (Linux; arm64) unknown" })
		for _, bad := range []string{"codex-tui/", "codex_cli_rs/0.100.0"} {
			h := make(http.Header)
			h.Set("User-Agent", bad)
			h.Set("Originator", "codex-tui")
			enforceCodexIdentityHeadersWithUA(h, bad)
			require.Equal(t, "0.200.1", h.Get("Version"))
			require.Equal(t, "0.200.1", openai.CodexUserAgentVersion(h.Get("User-Agent")))
		}
	}
	SetCodexCanonicalUserAgentResolver(func() string { return "codex-tui/" })
	require.Equal(t, codexCLIUserAgent, CodexCanonicalUserAgent())
}
