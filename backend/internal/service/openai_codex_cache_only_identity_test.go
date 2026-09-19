package service

import (
	"context"
	"encoding/json"
	"net/http"
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
