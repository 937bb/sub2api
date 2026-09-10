package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestCodexPanelEnvironmentOverrideReachesAccountTransports(t *testing.T) {
	const suppliedUA = "codex-tui/0.153.3 (Mac OS 26.5.1; arm64) iTerm.app/3.6.11 (codex-tui; 0.153.3)"
	const effectiveUA = "codex-tui/0.154.0 (Mac OS 26.5.1; arm64) iTerm.app/3.6.11 (codex-tui; 0.154.0)"
	settings := NewSettingService(&codexVersionSettingRepoStub{values: map[string]string{
		SettingKeyOpenAICodexUserAgent:           suppliedUA,
		SettingKeyOpenAICodexClientVersionSynced: "0.154.0",
	}}, nil)
	SetCodexConfiguredUserAgentResolver(func() string { return settings.GetOpenAICodexUserAgentOverride(context.Background()) })
	SetCodexCanonicalUserAgentResolver(func() string { return settings.GetOpenAICodexCanonicalUserAgent(context.Background()) })
	t.Cleanup(func() {
		SetCodexConfiguredUserAgentResolver(nil)
		SetCodexCanonicalUserAgentResolver(nil)
	})

	for _, accountType := range []string{AccountTypeOAuth, AccountTypeSetupToken} {
		t.Run(accountType, func(t *testing.T) {
			account := &Account{ID: 70, Platform: PlatformOpenAI, Type: accountType,
				Extra: map[string]any{codexFingerprintSeedExtraKey: testCodexFingerprintSeed}}
			require.Equal(t, suppliedUA, codexAccountUserAgent(account))
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			svc := &OpenAIGatewayService{}
			req, err := svc.buildUpstreamRequest(context.Background(), c, account, []byte(`{"model":"gpt-5.5"}`), "token", true, "", true)
			require.NoError(t, err)
			ws, _, err := svc.buildOpenAIWSHeaders(context.Background(), c, account, "token", OpenAIWSProtocolDecision{}, true, "", "", "", "", "")
			require.NoError(t, err)
			for _, headers := range []http.Header{req.Header, ws} {
				require.Equal(t, effectiveUA, headers.Get("User-Agent"))
				require.Equal(t, "0.154.0", headers.Get("version"))
				require.Equal(t, "codex-tui", headers.Get("originator"))
			}
			account.Credentials = map[string]any{"user_agent": "codex_vscode/0.153.3 (Mac OS 26.5.1; arm64) vscode"}
			require.Equal(t, account.GetOpenAIUserAgent(), codexAccountUserAgent(account), "explicit account override remains first")
		})
	}
	require.Empty(t, codexAccountUserAgent(&Account{Type: AccountTypeAPIKey, Platform: PlatformOpenAI}))
}

func TestCodexEmptyPanelOverrideKeepsGeneratedAccountEnvironment(t *testing.T) {
	settings := NewSettingService(&codexVersionSettingRepoStub{values: map[string]string{}}, nil)
	require.Empty(t, settings.GetOpenAICodexUserAgentOverride(context.Background()))
	require.Equal(t, DefaultOpenAICodexUserAgent, settings.GetOpenAICodexUserAgent(context.Background()))
	SetCodexConfiguredUserAgentResolver(func() string { return settings.GetOpenAICodexUserAgentOverride(context.Background()) })
	t.Cleanup(func() { SetCodexConfiguredUserAgentResolver(nil) })
	account := &Account{ID: 70, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Extra: map[string]any{codexFingerprintSeedExtraKey: testCodexFingerprintSeed}}
	require.NotEmpty(t, codexAccountUserAgent(account))
	require.NotEqual(t, DefaultOpenAICodexUserAgent, codexAccountUserAgent(account))
}

func TestCodexNormalizedIdentityMatchesAcrossBodyHTTPPassthroughAndWS(t *testing.T) {
	const installationID = "550e8400-e29b-41d4-a716-446655440000"
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeSetupToken} {
		for _, sessionHeader := range []string{"session-id", "session_id", ""} {
			t.Run(accountType+"/"+sessionHeader, func(t *testing.T) {
				account := &Account{ID: 70, Platform: PlatformOpenAI, Type: accountType,
					Credentials: map[string]any{"chatgpt_account_id": "upstream-one"},
					Extra:       map[string]any{openAICodexInstallationIDExtraKey: installationID, codexFingerprintModeExtraKey: "off"}}
				body := []byte(`{"model":"gpt-5.5","stream":true,"prompt_cache_key":"cache-one","client_metadata":{"x-codex-installation-id":"client-installation","session_id":"session-one","thread_id":"thread-one"},"input":[{"role":"user","content":"keep this input"}]}`)
				normalized, _, err := applyCodexAccountIdentityClientMetadataRaw(body, account, 91)
				require.NoError(t, err)
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
				c.Set("api_key", &APIKey{ID: 91})
				if sessionHeader != "" {
					c.Request.Header.Set(sessionHeader, "session-one")
				}
				c.Request.Header.Set("conversation_id", "separate-conversation")
				svc := &OpenAIGatewayService{}
				regular, err := svc.buildUpstreamRequest(context.Background(), c, account, normalized, "token", true, "cache-one", true)
				require.NoError(t, err)
				passthrough, err := svc.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, account, normalized, "token")
				require.NoError(t, err)
				ws, _, err := svc.buildOpenAIWSHeaders(context.Background(), c, account, "token", OpenAIWSProtocolDecision{}, true, "", "", "cache-one", "", "")
				require.NoError(t, err)
				for _, headers := range []http.Header{regular.Header, passthrough.Header, ws} {
					applyCodexNormalizedRequestIdentityHeaders(c, account, headers, normalized)
					require.Equal(t, gjson.GetBytes(normalized, "client_metadata.session_id").String(), headers.Get("session-id"))
					require.Equal(t, headers.Get("session-id"), headers.Get("session_id"))
					require.Equal(t, gjson.GetBytes(normalized, "client_metadata.thread_id").String(), headers.Get("thread-id"))
					require.Equal(t, installationID, headers.Get("x-codex-installation-id"))
					require.Equal(t, installationID, gjson.GetBytes(normalized, "client_metadata.x-codex-installation-id").String())
					require.NotEqual(t, headers.Get("session-id"), headers.Get("conversation_id"))
					_, err := uuid.Parse(headers.Get("session-id"))
					require.NoError(t, err)
				}
				require.JSONEq(t, gjson.GetBytes(body, "input").Raw, gjson.GetBytes(normalized, "input").Raw)
				if sessionHeader != "" {
					require.Equal(t, "session-one", c.GetHeader(sessionHeader))
				}
			})
		}
	}
}

func TestCodexCacheOnlyIdentityIsNotHashedTwice(t *testing.T) {
	account := &Account{ID: 70, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Credentials: map[string]any{"chatgpt_account_id": "upstream-one"}}
	body, _, err := applyCodexAccountIdentityClientMetadataRaw([]byte(`{"prompt_cache_key":"conversation-a"}`), account, 91)
	require.NoError(t, err)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set("api_key", &APIKey{ID: 91})
	req, err := (&OpenAIGatewayService{}).buildUpstreamRequestOpenAIPassthrough(context.Background(), c, account, body, "token")
	require.NoError(t, err)
	applyCodexNormalizedRequestIdentityHeaders(c, account, req.Header, body)
	require.Equal(t, gjson.GetBytes(body, "prompt_cache_key").String(), req.Header.Get("session_id"))
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	headers := make(http.Header)
	applyCodexNormalizedRequestIdentityHeadersMap(c, account, headers, payload)
	require.Equal(t, req.Header.Get("session_id"), headers.Get("session_id"))

	otherAccount := *account
	otherAccount.Credentials = map[string]any{"chatgpt_account_id": "upstream-two"}
	require.NotEqual(t, isolateOpenAIUpstreamSessionID(91, account, "same"), isolateOpenAIUpstreamSessionID(91, &otherAccount, "same"))
	require.NotEqual(t, isolateOpenAIUpstreamSessionID(91, account, "same"), isolateOpenAIUpstreamSessionID(92, account, "same"))
	require.Equal(t, isolateOpenAIUpstreamSessionID(91, account, "same"), isolateOpenAIUpstreamSessionID(91, account, "same"))

	apiKey := &Account{ID: 70, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	original := headers.Clone()
	applyCodexNormalizedRequestIdentityHeaders(c, apiKey, headers, []byte(`{"client_metadata":{"session_id":"should-not-apply"}}`))
	require.Equal(t, original, headers)
}

func TestCodexForwardTransportIdentityParityWithoutSessionConvergence(t *testing.T) {
	for _, useWS := range []bool{false, true} {
		t.Run(map[bool]string{false: "http", true: "ws"}[useWS], func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Gateway.OpenAIWS.Enabled = useWS
			cfg.Gateway.OpenAIWS.OAuthEnabled = useWS
			cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = useWS
			cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
			cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
			capture := &openAIWSCaptureConn{events: [][]byte{[]byte(`{"type":"response.completed","response":{"id":"resp_identity","model":"gpt-5.5","usage":{"input_tokens":2,"output_tokens":1}}}`)}}
			dialer := &openAIWSCaptureDialer{conn: capture}
			pool := newOpenAIWSConnPool(cfg)
			pool.setClientDialerForTest(dialer)
			upstream := &httpUpstreamRecorder{responses: []*http.Response{openAICompatSSECompletedResponse("resp_identity", "gpt-5.5")}}
			svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream, cache: &stubGatewayCache{},
				openaiWSResolver: NewOpenAIWSProtocolResolver(cfg), toolCorrector: NewCodexToolCorrector(), openaiWSPool: pool}
			account := &Account{ID: 77, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1,
				Credentials: map[string]any{"access_token": "test-token", "chatgpt_account_id": "upstream-77"},
				Extra: map[string]any{codexFingerprintModeExtraKey: "off", codexFingerprintSeedExtraKey: testCodexFingerprintSeed,
					openAICodexInstallationIDExtraKey: "550e8400-e29b-41d4-a716-446655440000", "responses_websockets_v2_enabled": useWS}}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			c.Request.Header.Set("session-id", "client-session")
			c.Request.Header.Set("x-codex-turn-metadata", `{"installation_id":"client-installation","session_id":"client-session","thread_id":"client-thread"}`)
			body := []byte(`{"model":"gpt-5.5","stream":true,"prompt_cache_key":"client-session","client_metadata":{"session_id":"client-session","thread_id":"client-thread"},"input":[{"role":"user","content":"hi"}]}`)
			result, err := svc.Forward(context.Background(), c, account, body)
			require.NoError(t, err)
			require.NotNil(t, result)
			var headers http.Header
			var sent []byte
			if useWS {
				headers = dialer.lastHeaders
				sent = []byte(requestToJSONString(capture.lastWrite))
			} else {
				require.Len(t, upstream.requests, 1)
				headers, sent = upstream.requests[0].Header, upstream.bodies[0]
			}
			require.Equal(t, gjson.GetBytes(sent, "client_metadata.session_id").String(), headers.Get("session-id"))
			require.Equal(t, headers.Get("session-id"), headers.Get("session_id"))
			require.Equal(t, gjson.GetBytes(sent, "client_metadata.x-codex-installation-id").String(), headers.Get("x-codex-installation-id"))
			if useWS {
				metadata := gjson.GetBytes(sent, "client_metadata.x-codex-turn-metadata").String()
				require.Equal(t, headers.Get("session-id"), gjson.Get(metadata, "session_id").String())
				require.Equal(t, headers.Get("x-codex-installation-id"), gjson.Get(metadata, "installation_id").String())
			}
		})
	}
}
