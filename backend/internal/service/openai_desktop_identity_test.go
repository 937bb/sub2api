package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestDesktopIdentityAcrossOutboundTransports(t *testing.T) {
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
					c.Request.Header.Set("User-Agent", codexCLIUserAgent)
					c.Request.Header.Set("Originator", openai.CodexDesktopOriginator)
					c.Request.Header.Set("Accept-Language", "zh-CN")
					c.Request.Header.Set("OAI-Product-Sku", "spoofed")
					c.Request.Header.Set("X-Codex-Installation-ID", "client-installation")
					c.Request.Header.Set("X-Codex-Window-ID", "client-window")
					c.Request.Header.Set("X-Client-Request-ID", "client-request")
					c.Request.Header.Set("Session_ID", "client-session-underscore")
					c.Request.Header.Set("Conversation_ID", "client-conversation")
					c.Request.Header.Set("Thread_ID", "client-thread-underscore")
					c.Request.Header.Set("Traceparent", "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01")
					c.Request.Header.Set("Tracestate", "vendor=value")
					c.Request.Header.Add("Baggage", "fixture=one")
					c.Request.Header.Add("Baggage", "fixture2=two")
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
					wantUA, wantOriginator := codexCLIUserAgent, openai.CodexDesktopOriginator
					if customUA != "" {
						wantUA = openai.SetCodexUserAgentVersion(customUA, codexCLIVersion)
						wantOriginator = "Codex Desktop"
					}
					require.Equal(t, wantOriginator, h.Get("Originator"))
					require.Equal(t, wantUA, h.Get("User-Agent"))
					require.Equal(t, wantBeta, h.Get("OpenAI-Beta"))
					if strings.Contains(transport, "compact") {
						require.Equal(t, codexCLIVersion, h.Get("Version"))
						require.NotEmpty(t, h.Get("Session_ID"))
						require.NotEmpty(t, h.Get("Conversation_ID"))
					} else {
						require.Empty(t, h.Get("Version"))
						require.Empty(t, h.Get("Session_ID"))
						require.Empty(t, h.Get("Conversation_ID"))
					}
					require.Empty(t, h.Get("Thread_ID"))
					require.Equal(t, "fixture-account", h.Get("ChatGPT-Account-ID"))
					require.Empty(t, h.Get("X-Codex-Installation-ID"))
					require.Empty(t, h.Get("Accept-Language"))
					require.Empty(t, h.Get("OAI-Product-Sku"))
					if strings.HasPrefix(transport, "ws-") {
						require.Empty(t, h.Get("Traceparent"))
						require.Empty(t, h.Get("Tracestate"))
						require.Empty(t, h.Values("Baggage"))
					} else {
						require.Equal(t, "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01", h.Get("Traceparent"))
						require.Equal(t, "vendor=value", h.Get("Tracestate"))
						require.Equal(t, []string{"fixture=one", "fixture2=two"}, h.Values("Baggage"))
					}
					for _, field := range []struct{ header, kind, value string }{
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

// A real Desktop caller may retain its own app/engine pair when canonical
// enforcement is disabled. This tests compatibility, not proof of client identity.
func TestDesktopCallerProfilePreservedAcrossTransports(t *testing.T) {
	oldEnforcement := codexIdentityEnforcement.Load()
	SetCodexIdentityEnforcementEnabled(false)
	t.Cleanup(func() { SetCodexIdentityEnforcementEnabled(oldEnforcement) })
	const desktopUA = "Codex Desktop/0.167.0-alpha.3.2 (Mac OS; arm64) unknown (Codex Desktop; 26.1103.31037)"
	for _, transport := range []string{"http", "compact", "passthrough", "ws"} {
		t.Run(transport, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			if transport == "compact" {
				c.Request.URL.Path += "/compact"
			}
			c.Request.Header.Set("User-Agent", desktopUA)
			c.Request.Header.Set("Originator", "Codex Desktop")
			c.Request.Header.Set("Version", "0.167.0-alpha.3.2")
			c.Request.Header.Set(openAIWSTurnMetadataHeader, `{"request_kind":"turn","analytics_enabled":false}`)
			svc := &OpenAIGatewayService{}
			account := &Account{ID: 91, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"chatgpt_account_id": "fixture"}}
			body := []byte(`{"model":"gpt-5","input":[]}`)
			var headers http.Header
			if transport == "ws" {
				var err error
				headers, _, err = svc.buildOpenAIWSHeaders(context.Background(), c, account, "fixture", OpenAIWSProtocolDecision{}, true, "", c.GetHeader(openAIWSTurnMetadataHeader), "", "", "")
				require.NoError(t, err)
			} else {
				var req *http.Request
				var err error
				if transport == "passthrough" {
					req, err = svc.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, account, body, "fixture")
				} else {
					req, err = svc.buildUpstreamRequest(context.Background(), c, account, body, "fixture", true, "", true)
				}
				require.NoError(t, err)
				headers = req.Header
			}
			require.Equal(t, desktopUA, headers.Get("User-Agent"))
			require.Equal(t, "Codex Desktop", headers.Get("Originator"))
			if transport == "compact" {
				require.Equal(t, "0.167.0-alpha.3.2", headers.Get("Version"))
			} else {
				require.Empty(t, headers.Get("Version"))
			}
		})
	}
}

func TestFinalizeCodexDesktopInferenceHeadersDoesNotAffectExplicitCLI(t *testing.T) {
	h := http.Header{
		"User-Agent":              {"codex_exec/0.167.0-alpha.3.2 (Mac OS 26.6.2; arm64) unknown (codex_exec; 0.167.0-alpha.3.2)"},
		"Originator":              {"codex_exec"},
		"Version":                 {"0.167.0-alpha.3.2"},
		"Session_id":              {"session"},
		"Conversation_id":         {"conversation"},
		"X-Codex-Installation-Id": {"installation"},
	}
	finalizeCodexDesktopInferenceHeaders(h, false, true)
	require.Equal(t, "0.167.0-alpha.3.2", h.Get("Version"))
	require.Equal(t, "session", h.Get("Session_id"))
	require.Equal(t, "conversation", h.Get("Conversation_id"))
	require.Equal(t, "installation", h.Get("X-Codex-Installation-Id"))
}

func TestDesktopCompatibilityBridgeRetainsCacheIdentityHeaders(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "Go-http-client/2.0")
	c.Request.Header.Set("Originator", openai.CodexDesktopOriginator)
	c.Set("api_key", &APIKey{ID: 1})

	account := accountDeviceFixture(91, "pool-owner")
	body := []byte(`{"model":"gpt-6.1-sol","stream":true,"prompt_cache_key":"bridge-cache","input":[]}`)
	req, err := (&OpenAIGatewayService{}).buildUpstreamRequestOpenAIPassthrough(
		context.Background(), c, account, body, "fixture-token",
	)
	require.NoError(t, err)

	outboundBody, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	want := isolateOpenAIUpstreamSessionID(1, account, gjson.GetBytes(outboundBody, "prompt_cache_key").String())
	require.Equal(t, want, req.Header.Get("Session_ID"))
	require.Equal(t, want, req.Header.Get("Conversation_ID"))
	require.Equal(t, codexCLIUserAgent, req.Header.Get("User-Agent"))
	require.Equal(t, openai.CodexDesktopOriginator, req.Header.Get("Originator"))
}

func TestDesktopCompatibilityBridgeUsesNativeSessionHeadersAfterBodyRecovery(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "Go-http-client/2.0")
	c.Set("api_key", &APIKey{ID: 1})

	const clientSession = "019aa111-1111-7111-8111-111111111111"
	account := newTestOAuthAccount(92, map[string]any{
		codexFingerprintModeExtraKey: "session",
		codexFingerprintSeedExtraKey: "11111111-1111-4111-8111-111111111111",
	})
	body := []byte(`{"model":"gpt-6.1-sol","stream":true,"prompt_cache_key":"` + clientSession + `","client_metadata":{"session_id":"` + clientSession + `","thread_id":"` + clientSession + `"},"input":[]}`)
	svc := &OpenAIGatewayService{}
	_, err := svc.prepareCodexAccountIdentitySource(context.Background(), c, account)
	require.NoError(t, err)
	stageCodexFingerprintIDs(c, resolveCodexFingerprintIDsForAttemptBody(c, account, c.Request.Header, body))
	body, changed, err := applyCodexOutboundIdentityClientMetadataRaw(c, account, body)
	require.NoError(t, err)
	require.True(t, changed)

	req, err := svc.buildUpstreamRequestOpenAIPassthrough(
		context.Background(), c, account, body, "fixture-token",
	)
	require.NoError(t, err)

	outboundBody, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	wantSession := gjson.GetBytes(outboundBody, "client_metadata.session_id").String()
	wantThread := gjson.GetBytes(outboundBody, "client_metadata.thread_id").String()
	require.NotEmpty(t, wantSession)
	require.NotEmpty(t, wantThread)
	require.Equal(t, wantSession, req.Header.Get("Session-Id"))
	require.Equal(t, wantThread, req.Header.Get("Thread-Id"))
	require.Equal(t, wantThread, req.Header.Get("X-Client-Request-Id"))
	require.Empty(t, req.Header.Get("Session_ID"))
	require.Empty(t, req.Header.Get("Conversation_ID"))
	require.Equal(t, codexCLIUserAgent, req.Header.Get("User-Agent"))
	require.Equal(t, openai.CodexDesktopOriginator, req.Header.Get("Originator"))
}
