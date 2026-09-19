package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestCodexCacheOnlyHTTPIdentity_TransportBuildersDoNotInventConversations(t *testing.T) {
	for _, mode := range []string{"off", "device", "session", "full"} {
		for _, accountType := range []string{AccountTypeOAuth, AccountTypeSetupToken} {
			t.Run(accountType+"/"+mode, func(t *testing.T) {
				account := conversationTestAccount(accountType, mode)
				c := conversationTestContext(91, "")
				c.Request.Header.Set("User-Agent", "omp/18.2.6")
				c.Request.Header.Set("x-client-request-id", "request-only-id")
				original := []byte(`{"model":"gpt-6-astra","prompt_cache_key":"shared-prefix","client_metadata":{"custom":"preserve"},"input":[{"role":"user","content":"hello"}]}`)
				body, _, err := applyCodexAccountIdentityClientMetadataRaw(original, account, 91)
				require.NoError(t, err)
				svc := &OpenAIGatewayService{}
				ids := svc.resolveCodexIsolatedFingerprintForRequest(context.Background(), c, account, body)
				require.True(t, isCodexCacheOnlyHTTPIdentity(c, account))
				body, _, err = applyCodexFingerprintClientMetadataRaw(body, ids)
				require.NoError(t, err)
				stageCodexFingerprintIDs(c, ids)
				require.Equal(t, scopeCodexAccountIdentityValue(account, 91, "prompt-cache", "shared-prefix"), gjson.GetBytes(body, "prompt_cache_key").String())
				require.Equal(t, "preserve", gjson.GetBytes(body, "client_metadata.custom").String())
				for _, field := range []string{"session_id", "thread_id", "turn_id", "x-codex-window-id"} {
					require.False(t, gjson.GetBytes(body, "client_metadata."+field).Exists(), field)
				}
				regular, err := svc.buildUpstreamRequest(context.Background(), c, account, body, "test-token", true, "shared-prefix", false)
				require.NoError(t, err)
				passthrough, err := svc.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, account, body, "test-token")
				require.NoError(t, err)
				ws, resolution, err := svc.buildOpenAIWSHeaders(context.Background(), c, account, "test-token", OpenAIWSProtocolDecision{}, false, "", "", "shared-prefix", "gpt-6-astra", "")
				require.NoError(t, err)
				require.Empty(t, resolution.SessionID)
				applyCodexNormalizedRequestIdentityHeaders(c, account, regular.Header, body)
				applyCodexNormalizedRequestIdentityHeaders(c, account, passthrough.Header, body)
				var payload map[string]any
				require.NoError(t, json.Unmarshal(body, &payload))
				applyCodexNormalizedRequestIdentityHeadersMap(c, account, ws, payload)
				normalizeCodexResponsesTransportHeaders(regular, account)
				normalizeCodexResponsesTransportHeaders(passthrough, account)
				normalizeCodexWebSocketTransportHeaders(ws, account)
				for _, headers := range []http.Header{regular.Header, passthrough.Header, ws} {
					for _, name := range []string{"session-id", "session_id", "thread-id", "conversation_id", "x-client-request-id", "x-codex-window-id"} {
						require.Empty(t, headers.Get(name), name)
					}
					if ids != nil {
						require.Equal(t, ids.installationID, headers.Get("x-codex-installation-id"))
					}
				}
			})
		}
	}
}

func TestCodexCacheOnlyHTTPIdentity_OnlyCacheWithoutConversationQualifies(t *testing.T) {
	for _, tc := range []struct {
		name, body, header, value string
	}{
		{name: "no_cache", body: `{"input":"hello"}`},
		{name: "body_session", body: `{"prompt_cache_key":"shared","client_metadata":{"session_id":"chat"}}`},
		{name: "body_thread_alias", body: `{"prompt_cache_key":"shared","client_metadata":{"thread-id":"thread"}}`},
		{name: "body_embedded", body: `{"prompt_cache_key":"shared","client_metadata":{"x-codex-turn-metadata":"{\"session_id\":\"chat\"}"}}`},
		{name: "header_session", header: "session-id", value: "chat"},
		{name: "header_legacy_session", header: "session_id", value: "chat"},
		{name: "header_conversation", header: "conversation_id", value: "chat"},
		{name: "header_compat_session", header: "X-Session-ID", value: "chat"},
		{name: "header_thread", header: "thread-id", value: "thread"},
		{name: "header_window", header: "x-codex-window-id", value: "thread:0"},
		{name: "header_embedded", header: "x-codex-turn-metadata", value: `{"thread_id":"thread"}`},
		{name: "native_ws", header: "Upgrade", value: "websocket"},
		{name: "native_ws_frame", body: `{"type":"response.create","prompt_cache_key":"shared"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := conversationTestContext(91, "")
			if tc.header != "" {
				c.Request.Header.Set(tc.header, tc.value)
			}
			body := tc.body
			if body == "" {
				body = `{"prompt_cache_key":"shared"}`
			}
			require.False(t, stageCodexCacheOnlyHTTPIdentity(c, conversationTestAccount(AccountTypeOAuth, "session"), []byte(body)))
		})
	}
	c := conversationTestContext(91, "")
	body := []byte(`{"prompt_cache_key":"shared"}`)
	require.False(t, stageCodexCacheOnlyHTTPIdentity(c, conversationTestAccount(AccountTypeAPIKey, "session"), body))
	require.False(t, stageCodexCacheOnlyHTTPIdentity(c, conversationTestAccount(AccountTypeOAuth, "session"), body, "compat-session"))
	c.Request.URL.Path = "/v1/chat/completions"
	require.False(t, stageCodexCacheOnlyHTTPIdentity(c, conversationTestAccount(AccountTypeOAuth, "session"), body))
	c.Request.URL.Path = "/v1/responses/compact"
	require.False(t, stageCodexCacheOnlyHTTPIdentity(c, conversationTestAccount(AccountTypeOAuth, "session"), body))
}

func TestCodexCacheOnlyHTTPIdentity_InternalScopeDoesNotShareCacheState(t *testing.T) {
	account := conversationTestAccount(AccountTypeOAuth, "session")
	body := []byte(`{"prompt_cache_key":"shared"}`)
	c := conversationTestContext(91, "")
	require.True(t, stageCodexCacheOnlyHTTPIdentity(c, account, body))
	scope := codexCacheOnlyHTTPExecutionScope(c, account)
	require.NotEmpty(t, scope)
	require.Equal(t, scope, codexCacheOnlyHTTPExecutionScope(c, account), "retries share only this request's scope")
	other := conversationTestContext(91, "")
	require.True(t, stageCodexCacheOnlyHTTPIdentity(other, account, body))
	require.NotEqual(t, scope, codexCacheOnlyHTTPExecutionScope(other, account))
	sharedCacheScope, _ := resolveOpenAIWSExecutionScope(c, body, 91)
	require.NotEqual(t, scope, sharedCacheScope)
	otherAccount := *account
	otherAccount.ID++
	require.False(t, isCodexCacheOnlyHTTPIdentity(c, &otherAccount))
	require.True(t, stageCodexCacheOnlyHTTPIdentity(c, &otherAccount, body))
	require.NotEqual(t, scope, codexCacheOnlyHTTPExecutionScope(c, &otherAccount))
	require.False(t, stageCodexCacheOnlyHTTPIdentity(c, account, []byte(`{"client_metadata":{"session_id":"real-chat"}}`)))
	require.False(t, isCodexCacheOnlyHTTPIdentity(c, account), "a new attempt cannot inherit the prior marker")
}

func TestCodexCacheOnlyHTTPIdentity_FinalHTTPRoutingIsStableAndTenantScoped(t *testing.T) {
	for _, mode := range []struct {
		accountType string
		passthrough bool
	}{{AccountTypeOAuth, false}, {AccountTypeOAuth, true}, {AccountTypeSetupToken, false}, {AccountTypeSetupToken, true}} {
		t.Run(mode.accountType+"/"+map[bool]string{false: "normal", true: "passthrough"}[mode.passthrough], func(t *testing.T) {
			upstream := &httpUpstreamRecorder{}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream, cache: &stubGatewayCache{}}
			body := []byte(`{"model":"gpt-5.5","stream":true,"store":false,"prompt_cache_key":"shared-prefix","input":"hello"}`)
			first := conversationTestAccount(mode.accountType, "session")
			second := conversationTestAccount(mode.accountType, "session")
			second.ID++
			second.Credentials["chatgpt_account_id"] = "another-upstream-account"
			for _, account := range []*Account{first, second} {
				account.Credentials["access_token"] = "test-token"
				account.Extra["openai_oauth_passthrough"] = mode.passthrough
			}
			for _, tc := range []struct {
				account *Account
				keyID   int64
			}{{first, 91}, {first, 91}, {first, 92}, {second, 91}} {
				upstream.responses = append(upstream.responses, openAICompatSSECompletedResponse("resp_cache_only", "gpt-5.5"))
				_, err := svc.Forward(context.Background(), conversationTestContext(tc.keyID, ""), tc.account, body)
				require.NoError(t, err)
			}
			require.Len(t, upstream.requests, 4)
			for i, request := range upstream.requests {
				require.Equal(t, gjson.GetBytes(upstream.bodies[i], "prompt_cache_key").String(), request.Header.Get("session-id"))
				require.Empty(t, request.Header.Get("thread-id"))
				require.Empty(t, request.Header.Get("x-client-request-id"))
				require.Empty(t, gjson.GetBytes(upstream.bodies[i], "client_metadata.session_id").String())
			}
			require.Equal(t, upstream.requests[0].Header.Get("session-id"), upstream.requests[1].Header.Get("session-id"))
			require.NotEqual(t, upstream.requests[0].Header.Get("session-id"), upstream.requests[2].Header.Get("session-id"))
			require.NotEqual(t, upstream.requests[0].Header.Get("session-id"), upstream.requests[3].Header.Get("session-id"))
		})
	}
}

func TestCodexCacheOnlyHTTPIdentity_RetriesKeepFinalHTTPRouting(t *testing.T) {
	for _, mapped := range []bool{false, true} {
		t.Run(map[bool]string{false: "http_retry", true: "mapped_retry"}[mapped], func(t *testing.T) {
			upstream := &httpUpstreamRecorder{responses: []*http.Response{
				{StatusCode: http.StatusServiceUnavailable, Header: make(http.Header), Body: http.NoBody},
				openAICompatSSECompletedResponse("resp_cache_retry", "gpt-5.5"),
			}}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream, cache: &stubGatewayCache{},
				settingService: &SettingService{settingRepo: &gatewayTTLSettingRepo{}, cfg: &config.Config{}, oauthRetryCache: &cachedOAuthRetrySettings{
					settings: OAuthRetrySettings{Enabled: true, MaxRetries: 1, StatusCodes: []int{503}}, expires: time.Now().Add(time.Minute),
				}}}
			account := conversationTestAccount(AccountTypeOAuth, "session")
			account.Credentials["access_token"] = "test-token"
			c := conversationTestContext(91, "")
			body := []byte(`{"model":"gpt-5.5","stream":true,"store":false,"prompt_cache_key":"shared-prefix","input":"hello"}`)
			if mapped {
				upstream.responses[0] = &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}},
					Body: io.NopCloser(strings.NewReader("event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"upstream_error\",\"message\":\"Upstream request failed\"}}}\n\n"))}
				svc.settingService.oauthRetryCache.settings.StatusCodes = []int{502}
				_, err := svc.Forward(context.Background(), c, account, body)
				require.NoError(t, err)
			} else {
				scoped, _, err := applyCodexAccountIdentityClientMetadataRaw(body, account, 91)
				require.NoError(t, err)
				require.True(t, stageCodexCacheOnlyHTTPIdentity(c, account, scoped))
				request, err := svc.buildUpstreamRequest(context.Background(), c, account, scoped, "test-token", true, "shared-prefix", false)
				require.NoError(t, err)
				response, err, exhausted := svc.doOAuthResponsesUpstream(c, request, "", account)
				require.NoError(t, err)
				require.False(t, exhausted)
				require.NoError(t, response.Body.Close())
			}
			require.Len(t, upstream.requests, 2)
			expected := scopeCodexAccountIdentityValue(account, 91, "prompt-cache", "shared-prefix")
			for i, request := range upstream.requests {
				require.Equal(t, expected, request.Header.Get("session-id"))
				require.Equal(t, expected, gjson.GetBytes(upstream.bodies[i], "prompt_cache_key").String())
				require.Empty(t, request.Header.Get("thread-id"))
			}
		})
	}
}

func TestCodexCacheOnlyHTTPIdentity_FinalHTTPRoutingRequiresSafeMarker(t *testing.T) {
	for _, reason := range []string{"unstaged", "raw_key", "no_namespace", "no_api_key", "changed_api_key", "changed_account", "compact", "other_host", "websocket", "api_key"} {
		t.Run(reason, func(t *testing.T) {
			account := conversationTestAccount(AccountTypeOAuth, "session")
			c := conversationTestContext(91, "")
			request := httptest.NewRequest(http.MethodPost, "https://chatgpt.com/backend-api/codex/responses", nil)
			body := map[string]any{"prompt_cache_key": "shared-prefix"}
			if reason == "no_namespace" {
				account.Credentials, account.Extra = nil, nil
			}
			applyCodexAccountIdentityClientMetadataMap(body, account, 91)
			if reason == "raw_key" {
				body["prompt_cache_key"] = "unscoped-raw-key"
			}
			if reason == "no_api_key" {
				c.Set("api_key", &APIKey{})
			}
			if reason != "unstaged" {
				require.True(t, stageCodexCacheOnlyHTTPIdentity(c, account, body))
			}
			switch reason {
			case "changed_api_key":
				c.Set("api_key", &APIKey{ID: 92})
			case "changed_account":
				account.ID++
			case "compact":
				request.URL.Path += "/compact"
			case "other_host":
				request.URL.Host = "api.openai.com"
			case "websocket":
				request.Header.Set("Upgrade", "websocket")
			case "api_key":
				account.Type = AccountTypeAPIKey
			}
			applyCodexCacheOnlyHTTPRoutingHeaders(c, account, request)
			require.Empty(t, request.Header.Get("session-id"))
		})
	}
}

func TestCodexCacheOnlyHTTPIdentity_TurnStateGuardDoesNotPromoteCacheRouting(t *testing.T) {
	account := conversationTestAccount(AccountTypeOAuth, "session")
	svc := &OpenAIGatewayService{}
	body := map[string]any{"prompt_cache_key": "shared-prefix"}
	applyCodexAccountIdentityClientMetadataMap(body, account, 91)
	var scopes []string
	for i := range 2 {
		c := conversationTestContext(91, "")
		require.True(t, stageCodexCacheOnlyHTTPIdentity(c, account, body))
		request := httptest.NewRequest(http.MethodPost, "https://chatgpt.com/backend-api/codex/responses", nil)
		svc.guardOpenAICodexTurnStateEcho(c, account, request.Header, "gpt-5.5")
		requestScope := openAICodexTurnStateSessionHash(c)
		require.Equal(t, codexCacheOnlyHTTPExecutionScope(c, account), requestScope)
		applyCodexCacheOnlyHTTPRoutingHeaders(c, account, request)
		normalizeCodexResponsesTransportHeaders(request, account)
		require.Equal(t, body["prompt_cache_key"], request.Header.Get("session-id"))
		// A later guard must not interpret the cache-affinity header as a real
		// conversation, even if send/guard ordering is refactored in the future.
		svc.guardOpenAICodexTurnStateEcho(c, account, request.Header, "gpt-5.5")
		require.Equal(t, requestScope, openAICodexTurnStateSessionHash(c))
		require.Empty(t, request.Header.Get("thread-id"))
		scopes = append(scopes, requestScope)
		svc.observeOpenAICodexTurnState(c, account, testOpenAICodexTurnState(312, time.Now(), byte('a'+i)), "http")
	}
	require.NotEqual(t, scopes[0], scopes[1], "same PCK cannot merge response provenance between HTTP requests")
	pool := svc.getOpenAICodexTurnStatePool()
	pool.mu.RLock()
	defer pool.mu.RUnlock()
	require.Len(t, pool.entries, 2)
	observedScopes := make(map[string]bool)
	for _, record := range pool.entries {
		observedScopes[record.SourceSessionHash] = true
	}
	for _, scope := range scopes {
		require.True(t, observedScopes[scope])
	}
}

func TestCodexCacheOnlyHTTPIdentity_SharedCacheKeepsExplicitChatsSeparate(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := conversationTestAccount(AccountTypeOAuth, "full")
	resolve := func(keyID int64, sessionID string) (*codexFingerprintIDs, string) {
		c := conversationTestContext(keyID, "")
		body := map[string]any{"prompt_cache_key": "shared-prefix", "client_metadata": map[string]any{"session_id": sessionID}}
		applyCodexAccountIdentityClientMetadataMap(body, account, keyID)
		ids := svc.resolveCodexIsolatedFingerprintForRequest(context.Background(), c, account, body)
		require.False(t, isCodexCacheOnlyHTTPIdentity(c, account))
		applyCodexFingerprintClientMetadata(body, ids)
		cacheKey, ok := body["prompt_cache_key"].(string)
		require.True(t, ok)
		return ids, cacheKey
	}
	a, aKey := resolve(91, "chat-a")
	again, againKey := resolve(91, "chat-a")
	b, bKey := resolve(91, "chat-b")
	tenant, tenantKey := resolve(92, "chat-a")
	require.Equal(t, aKey, againKey)
	require.Equal(t, aKey, bKey, "cache routing may be shared by distinct conversations")
	require.NotEqual(t, aKey, tenantKey)
	require.Equal(t, a.sessionID, again.sessionID)
	require.Equal(t, a.threadID, again.threadID)
	require.NotEqual(t, a.sessionID, b.sessionID)
	require.NotEqual(t, a.threadID, b.threadID)
	require.NotEqual(t, a.sessionID, tenant.sessionID)
}

func TestCodexCacheOnlyHTTPIdentity_NoCacheAndNativeWSKeepAnonymousIsolation(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := conversationTestAccount(AccountTypeOAuth, "session")
	for _, ws := range []bool{false, true} {
		body := []byte(`{"input":"hello"}`)
		first, second := conversationTestContext(91, ""), conversationTestContext(91, "")
		if ws {
			body = []byte(`{"type":"response.create","prompt_cache_key":"shared","input":"hello"}`)
			first.Request.Method, second.Request.Method = http.MethodGet, http.MethodGet
		}
		a := svc.resolveCodexIsolatedFingerprintForRequest(context.Background(), first, account, body)
		b := svc.resolveCodexIsolatedFingerprintForRequest(context.Background(), second, account, body)
		retry := svc.resolveCodexIsolatedFingerprintForRequest(context.Background(), first, account, body)
		require.NotEmpty(t, a.sessionID)
		require.NotEqual(t, a.sessionID, b.sessionID)
		require.Equal(t, a.sessionID, retry.sessionID)
		require.False(t, isCodexCacheOnlyHTTPIdentity(first, account))
	}
}

func TestCodexCacheOnlyHTTPIdentity_HTTPToWSDoesNotLoadSharedState(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	capture := &openAIWSCaptureConn{events: [][]byte{[]byte(`{"type":"response.completed","response":{"id":"resp_cache_only","model":"gpt-5.5","usage":{"input_tokens":2,"output_tokens":1}}}`)}}
	dialer := &openAIWSCaptureDialer{conn: capture, handshake: http.Header{"X-Codex-Turn-State": []string{"new-request-state"}}}
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(dialer)
	t.Cleanup(pool.Close)
	store := NewOpenAIWSStateStore(nil)
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: &httpUpstreamRecorder{}, cache: &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg), toolCorrector: NewCodexToolCorrector(), openaiWSPool: pool, openaiWSStateStore: store}
	account := conversationTestAccount(AccountTypeOAuth, "session")
	account.Credentials["access_token"] = "test-token"
	account.Extra["responses_websockets_v2_enabled"] = true
	account.Concurrency = 1
	c := conversationTestContext(91, "")
	body := []byte(`{"model":"gpt-5.5","stream":true,"store":false,"prompt_cache_key":"shared-prefix","input":[{"role":"user","content":"hello"}]}`)
	oldSharedScope, _ := resolveOpenAIWSExecutionScope(c, body, 91)
	store.BindSessionTurnState(0, oldSharedScope, "another-conversation-state", time.Minute)
	store.BindSessionConn(0, oldSharedScope, "another-conversation-connection", time.Minute)
	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Empty(t, dialer.lastHeaders.Get("x-codex-turn-state"), "shared PCK state must never be loaded")
	require.Empty(t, dialer.lastHeaders.Get("session-id"))
	require.Empty(t, dialer.lastHeaders.Get("thread-id"))
	requestScope := codexCacheOnlyHTTPExecutionScope(c, account)
	require.NotEmpty(t, requestScope)
	require.NotEqual(t, oldSharedScope, requestScope)
	state, found := store.GetSessionTurnState(0, requestScope)
	require.True(t, found)
	require.Equal(t, "new-request-state", state)
	oldState, found := store.GetSessionTurnState(0, oldSharedScope)
	require.True(t, found)
	require.Equal(t, "another-conversation-state", oldState)
	conn, found := store.GetSessionConn(0, requestScope)
	require.True(t, found)
	require.NotEqual(t, "another-conversation-connection", conn)
}
