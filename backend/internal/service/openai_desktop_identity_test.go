package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCLIIdentityAcrossOutboundTransports(t *testing.T) {
	SetCodexCanonicalUserAgentResolver(nil)
	t.Cleanup(func() { SetCodexCanonicalUserAgentResolver(nil) })
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeSetupToken} {
		for _, customUA := range []string{"", "Codex Desktop/0.150.0 (Mac OS; arm64) unknown (Codex Desktop; 26.10.1)"} {
			for _, transport := range []string{"http", "compact", "passthrough", "passthrough-compact", "ws-v1", "ws-v2"} {
				t.Run(accountType+"/"+customUA+"/"+transport, func(t *testing.T) {
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
					if strings.Contains(transport, "compact") {
						c.Request.URL.Path += "/compact"
					}
					c.Request.Header.Set("Version", "9.9.9")
					c.Request.Header.Set("OpenAI-Beta", "responses=experimental")
					c.Request.Header.Set("User-Agent", "third-party/1.0")
					c.Request.Header.Set("X-Codex-Installation-ID", "client-installation")
					c.Request.Header.Set("X-Codex-Window-ID", "client-window")
					c.Request.Header.Set("X-Client-Request-ID", "client-request")
					c.Request.Header.Set("Cookie", "must-not-forward")
					svc := &OpenAIGatewayService{}
					account := &Account{ID: 91, Platform: PlatformOpenAI, Type: accountType,
						Credentials: map[string]any{"chatgpt_account_id": "fixture-account", "user_agent": customUA}}
					body := []byte(`{"model":"gpt-5","input":"test","instructions":"test"}`)
					var h http.Header
					wantBeta := ""
					if strings.HasPrefix(transport, "ws-") {
						decision := OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2}
						wantBeta = openAIWSBetaV2Value
						if transport == "ws-v1" {
							decision.Transport = OpenAIUpstreamTransportResponsesWebsocket
							wantBeta = openAIWSBetaV1Value
						}
						var err error
						h, _, err = svc.buildOpenAIWSHeaders(context.Background(), c, account, "fixture-token",
							decision,
							false, "", "", "", "", "")
						require.NoError(t, err)
					} else {
						var req *http.Request
						var err error
						if strings.HasPrefix(transport, "passthrough") {
							req, err = svc.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, account, body, "fixture-token")
						} else {
							req, err = svc.buildUpstreamRequest(context.Background(), c, account, body, "fixture-token", true, "", false)
						}
						require.NoError(t, err)
						h = req.Header
						if strings.Contains(transport, "compact") {
							require.Equal(t, "application/json", h.Get("Accept"))
						}
					}
					wantUA, wantOriginator := codexCLIUserAgent, openai.CodexDefaultOriginator
					if customUA != "" {
						wantUA = openai.SetCodexUserAgentVersion(customUA, codexCLIVersion)
						wantOriginator = "Codex Desktop"
					}
					require.Equal(t, wantOriginator, h.Get("Originator"))
					require.Equal(t, wantUA, h.Get("User-Agent"))
					require.Equal(t, wantBeta, h.Get("OpenAI-Beta"))
					require.Equal(t, "0.166.0-alpha.5.1", h.Get("Version"))
					require.Equal(t, "fixture-account", h.Get("ChatGPT-Account-ID"))
					for _, field := range []struct{ header, kind, value string }{
						{"X-Codex-Installation-ID", "installation", "client-installation"},
						{"X-Codex-Window-ID", "window", "client-window"},
						{"X-Client-Request-ID", "request", "client-request"},
					} {
						require.Equal(t, scopeCodexAccountIdentityValue(account, 0, field.kind, field.value), h.Get(field.header))
					}
					require.Empty(t, h.Get("Cookie"))
				})
			}
		}
	}
}

func TestDesktopIdentityVersionSettingsAndAuth(t *testing.T) {
	svc := NewSettingService(&codexVersionSettingRepoStub{values: map[string]string{
		SettingKeyOpenAICodexClientVersion: "0.200.1",
		SettingKeyOpenAICodexUserAgent:     openai.CodexDesktopOriginator + "/" + openai.CodexDesktopEngineVersion + openai.CodexDesktopUserAgentSuffix,
	}}, nil)
	SetCodexCanonicalUserAgentResolver(func() string { return svc.GetOpenAICodexCanonicalUserAgent(context.Background()) })
	t.Cleanup(func() { SetCodexCanonicalUserAgentResolver(nil) })
	want := "Codex Desktop/0.200.1" + openai.CodexDesktopUserAgentSuffix
	require.Equal(t, want, CodexCanonicalUserAgent())
	h := make(http.Header)
	ApplyCodexCanonicalAuthIdentity(h)
	require.Equal(t, "Codex Desktop", h.Get("Originator"))
	require.Equal(t, want, h.Get("User-Agent"))
	require.Empty(t, h.Get("Version"))
	enforce := make(http.Header)
	ensureCodexIdentityHeaders(enforce)
	enforceCodexIdentityHeaders(enforce)
	require.Equal(t, "0.200.1", enforce.Get("Version"))
	require.Equal(t, want, enforce.Get("User-Agent"))
}
