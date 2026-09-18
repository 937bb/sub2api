package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newTurnStateTestContext(t *testing.T, apiKeyID int64, sessionID string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	if sessionID != "" {
		c.Request.Header.Set("session_id", sessionID)
	}
	if apiKeyID > 0 {
		c.Set("api_key", &APIKey{ID: apiKeyID})
	}
	return c, rec
}

func testOpenAICodexPreferredTurnState(prefix string) string {
	if len(prefix) > openAICodexPreferredTurnStateLength {
		panic("test turn-state prefix exceeds preferred length")
	}
	return prefix + strings.Repeat("_", openAICodexPreferredTurnStateLength-len(prefix))
}

func TestOpenAICodexTurnStateSeed(t *testing.T) {
	c, _ := newTurnStateTestContext(t, 7, "sess-1")
	require.Equal(t, "7\x00sess-1", openAICodexTurnStateSeed(c))

	// The hyphenated Codex CLI header takes precedence.
	c.Request.Header.Set("session-id", "sess-hyphen")
	require.Equal(t, "7\x00sess-hyphen", openAICodexTurnStateSeed(c))

	// Requests without a session identifier are not tracked.
	cNoSession, _ := newTurnStateTestContext(t, 7, "")
	require.Empty(t, openAICodexTurnStateSeed(cNoSession))

	require.Empty(t, openAICodexTurnStateSeed(nil))
}

func TestRelayOpenAICodexTurnState_StoresOAuthState(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	c, _ := newTurnStateTestContext(t, 7, "sess-relay")
	upstream := http.Header{"X-Codex-Turn-State": []string{"blob-A-long"}}

	svc.relayOpenAICodexTurnState(c, account, upstream)

	require.Equal(t, "blob-A-long", c.Writer.Header().Get("X-Codex-Turn-State"))
	selected, ok := svc.getOpenAICodexTurnStatePool().longestActive()
	require.True(t, ok)
	require.Equal(t, "blob-A-long", selected)
}

func TestStageOpenAICodexTurnState_StoresOnlyAfterCommit(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := &Account{ID: 44, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	c, _ := newTurnStateTestContext(t, 9, "sess-staged")
	var staged http.Header
	stageOpenAICodexTurnState(&staged, http.Header{"X-Codex-Turn-State": []string{"blob-B"}})

	_, ok := svc.getOpenAICodexTurnStatePool().longestActive()
	require.False(t, ok)
	svc.noteStagedOpenAICodexTurnStateCommitted(c, account, staged)
	selected, ok := svc.getOpenAICodexTurnStatePool().longestActive()
	require.True(t, ok)
	require.Equal(t, "blob-B", selected)

	stageOpenAICodexTurnState(&staged, http.Header{})
	require.Empty(t, staged.Get("X-Codex-Turn-State"))
}

func TestOpenAICodexTurnStatePool_SelectsLongestAndFallsBackAfterExpiry(t *testing.T) {
	pool := newOpenAICodexTurnStatePool()
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	pool.now = func() time.Time { return now }
	pool.observe("short", nil, "", "http")
	pool.observe("the-longest-value", nil, "", "ws")
	selected, ok := pool.longestActive()
	require.True(t, ok)
	require.Equal(t, "the-longest-value", selected)

	pool.mu.Lock()
	pool.entries[hashOpenAICodexTurnState("the-longest-value")].ExpiresAt = now.Add(-time.Second)
	pool.selected.ExpiresAt = now.Add(-time.Second)
	pool.mu.Unlock()
	selected, ok = pool.longestActive()
	require.True(t, ok)
	require.Equal(t, "short", selected)
}

func TestOpenAICodexTurnStatePool_EqualLengthUsesNewest(t *testing.T) {
	pool := newOpenAICodexTurnStatePool()
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	pool.now = func() time.Time { return now }
	pool.observe("first", nil, "", "http")
	now = now.Add(time.Second)
	pool.observe("later", nil, "", "http")
	selected, ok := pool.longestActive()
	require.True(t, ok)
	require.Equal(t, "later", selected)
}

func TestOpenAICodexTurnStatePool_CleanupRemovesExpiredNonSelectedEntries(t *testing.T) {
	pool := newOpenAICodexTurnStatePool()
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	pool.now = func() time.Time { return now }
	pool.observe("short", nil, "", "http")
	pool.observe("the-longest-value", nil, "", "http")

	pool.mu.Lock()
	pool.entries[hashOpenAICodexTurnState("short")].ExpiresAt = now.Add(-time.Second)
	pool.mu.Unlock()
	pool.cleanupExpired(now)

	pool.mu.RLock()
	defer pool.mu.RUnlock()
	require.Len(t, pool.entries, 1)
	require.NotNil(t, pool.entries[hashOpenAICodexTurnState("the-longest-value")])
}

func TestGuardOpenAICodexTurnStateEcho_PrefersSameSession292State(t *testing.T) {
	svc := &OpenAIGatewayService{}
	c, _ := newTurnStateTestContext(t, 7, "sess-account")
	source := &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	target := &Account{ID: 43, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	sourceState := strings.Repeat("a", openAICodexPreferredTurnStateLength)
	targetState := strings.Repeat("b", openAICodexPreferredTurnStateLength)
	svc.observeOpenAICodexTurnState(c, source, sourceState, "http")
	svc.observeOpenAICodexTurnState(c, target, targetState, "http")

	h := http.Header{"X-Codex-Turn-State": []string{"client-value"}}
	svc.guardOpenAICodexTurnStateEcho(c, target, h)
	require.Equal(t, targetState, h.Get("X-Codex-Turn-State"))
	require.NotEqual(t, sourceState, h.Get("X-Codex-Turn-State"))
	require.Equal(t, openAICodexTurnStateReuseScopeSession, c.GetString(openAICodexTurnStateReuseScopeContextKey))
}

func TestGuardOpenAICodexTurnStateEcho_FallsBackFromGlobalToAccountToSession(t *testing.T) {
	svc := &OpenAIGatewayService{}
	pool := svc.getOpenAICodexTurnStatePool()
	sourceContext, _ := newTurnStateTestContext(t, 7, "sess-source")
	targetContext, _ := newTurnStateTestContext(t, 7, "sess-target")
	sourceID := int64(42)
	globalState := strings.Repeat("a", openAICodexPreferredTurnStateLength)
	pool.observe(globalState, &sourceID, openAICodexTurnStateSessionHash(sourceContext), "http")
	h := http.Header{"X-Codex-Turn-State": []string{"client-value"}}

	svc.guardOpenAICodexTurnStateEcho(targetContext, &Account{ID: 43, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, h)
	require.Equal(t, globalState, h.Get("X-Codex-Turn-State"))
	require.Equal(t, openAICodexTurnStateReuseScopeGlobal, targetContext.GetString(openAICodexTurnStateReuseScopeContextKey))

	targetID := int64(43)
	otherTargetContext, _ := newTurnStateTestContext(t, 7, "sess-target-other")
	nonPreferred := strings.Repeat("c", openAICodexPreferredTurnStateLength+40)
	pool.observe(nonPreferred, &targetID, openAICodexTurnStateSessionHash(otherTargetContext), "http")
	svc.guardOpenAICodexTurnStateEcho(targetContext, &Account{ID: 43, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, h)
	require.Equal(t, globalState, h.Get("X-Codex-Turn-State"), "non-292 states must never become fallbacks")

	accountState := strings.Repeat("b", openAICodexPreferredTurnStateLength)
	pool.observe(accountState, &targetID, openAICodexTurnStateSessionHash(otherTargetContext), "http")
	svc.guardOpenAICodexTurnStateEcho(targetContext, &Account{ID: 43, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, h)
	require.Equal(t, accountState, h.Get("X-Codex-Turn-State"))
	require.Equal(t, openAICodexTurnStateReuseScopeAccount, targetContext.GetString(openAICodexTurnStateReuseScopeContextKey))

	targetSessionHash := openAICodexTurnStateSessionHash(targetContext)
	sessionState := strings.Repeat("d", openAICodexPreferredTurnStateLength)
	pool.observe(sessionState, &targetID, targetSessionHash, "http")
	svc.guardOpenAICodexTurnStateEcho(targetContext, &Account{ID: 43, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, h)
	require.Equal(t, sessionState, h.Get("X-Codex-Turn-State"))
	require.Equal(t, openAICodexTurnStateReuseScopeSession, targetContext.GetString(openAICodexTurnStateReuseScopeContextKey))
}

func TestGuardOpenAICodexTurnStateEcho_FallsBackAcrossSessionsWithinAccount(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := &Account{ID: 43, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	sourceContext, _ := newTurnStateTestContext(t, 7, "shared-client-session")
	targetContext, _ := newTurnStateTestContext(t, 7, "shared-client-session")
	state := strings.Repeat("s", openAICodexPreferredTurnStateLength)
	sourceHeader := http.Header{}
	sourceHeader.Set("session-id", "upstream-session-source")
	svc.guardOpenAICodexTurnStateEcho(sourceContext, account, sourceHeader)
	svc.observeOpenAICodexTurnState(sourceContext, account, state, "http")

	targetHeader := http.Header{"X-Codex-Turn-State": []string{"client-value"}}
	targetHeader.Set("session-id", "upstream-session-target")
	svc.guardOpenAICodexTurnStateEcho(targetContext, account, targetHeader)
	require.Equal(t, state, targetHeader.Get(openAICodexTurnStateHeader))
	require.Equal(t, openAICodexTurnStateReuseScopeAccount, targetContext.GetString(openAICodexTurnStateReuseScopeContextKey))

	matchingContext, _ := newTurnStateTestContext(t, 7, "another-client-session")
	matchingHeader := http.Header{}
	matchingHeader.Set("session-id", "upstream-session-source")
	svc.guardOpenAICodexTurnStateEcho(matchingContext, account, matchingHeader)
	require.Equal(t, state, matchingHeader.Get(openAICodexTurnStateHeader))
	require.Equal(t, openAICodexTurnStateReuseScopeSession, matchingContext.GetString(openAICodexTurnStateReuseScopeContextKey))
}

func TestGuardOpenAICodexTurnStateEcho_FallsBackWithoutSessionID(t *testing.T) {
	svc := &OpenAIGatewayService{}
	pool := svc.getOpenAICodexTurnStatePool()
	accountID := int64(43)
	state := testOpenAICodexPreferredTurnState("account-without-session")
	pool.observe(state, &accountID, "other-session", "http")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	h := http.Header{"X-Codex-Turn-State": []string{"client-value"}}

	svc.guardOpenAICodexTurnStateEcho(c, &Account{ID: accountID, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, h)

	require.Equal(t, state, h.Get(openAICodexTurnStateHeader))
	require.Equal(t, openAICodexTurnStateReuseScopeAccount, c.GetString(openAICodexTurnStateReuseScopeContextKey))
}

func TestGuardOpenAICodexTurnStateEcho_APIKeyUnaffected(t *testing.T) {
	svc := &OpenAIGatewayService{}
	c, _ := newTurnStateTestContext(t, 7, "sess-apikey")
	svc.getOpenAICodexTurnStatePool().observe("global-longest", nil, "", "http")
	h := http.Header{"X-Codex-Turn-State": []string{"client-value"}}

	svc.guardOpenAICodexTurnStateEcho(c, &Account{ID: 99, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, h)
	require.Equal(t, "client-value", h.Get("X-Codex-Turn-State"))
}

func TestGuardOpenAICodexTurnStateEcho_NoActiveValueClearsOAuthHeader(t *testing.T) {
	svc := &OpenAIGatewayService{}
	h := http.Header{"X-Codex-Turn-State": []string{"stale-client-value"}}
	svc.guardOpenAICodexTurnStateEcho(nil, &Account{ID: 43, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, h)
	require.Empty(t, h.Get("X-Codex-Turn-State"))
}

func TestBuildOpenAIWSHeaders_UsesSameAccount292State(t *testing.T) {
	svc := &OpenAIGatewayService{}
	accountID := int64(43)
	otherAccountID := int64(44)
	accountState := strings.Repeat("a", openAICodexPreferredTurnStateLength)
	otherAccountState := strings.Repeat("b", openAICodexPreferredTurnStateLength)
	c, _ := newTurnStateTestContext(t, 7, "sess-ws-global")
	account := &Account{
		ID:          43,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"chatgpt_account_id": "test-account"},
	}
	initialHeaders, _, err := svc.buildOpenAIWSHeaders(
		context.Background(), c, account, "token",
		OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2},
		true, "client-state", "", "", "gpt-5.6-codex", "",
	)
	require.NoError(t, err)
	require.Empty(t, initialHeaders.Get(openAIWSTurnStateHeader))
	sessionHash := openAICodexTurnStateSessionHash(c)
	require.NotEmpty(t, sessionHash)
	svc.getOpenAICodexTurnStatePool().observe(accountState, &accountID, sessionHash, "http")
	svc.getOpenAICodexTurnStatePool().observe(otherAccountState, &otherAccountID, sessionHash, "ws")

	headers, _, err := svc.buildOpenAIWSHeaders(
		context.Background(), c, account, "token",
		OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2},
		true, "client-state", "", "", "gpt-5.6-codex", "",
	)
	require.NoError(t, err)
	require.Equal(t, accountState, headers.Get(openAIWSTurnStateHeader))
}

func TestOpenAICodexTurnStatePool_Session292UsesNewestAndFallsBackAfterExpiry(t *testing.T) {
	pool := newOpenAICodexTurnStatePool()
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	pool.now = func() time.Time { return now }
	accountID := int64(42)
	sessionHash := hashOpenAICodexTurnState("session-42")
	key, ok := newOpenAICodexTurnStateSessionKey(accountID, sessionHash)
	require.True(t, ok)
	older := strings.Repeat("a", openAICodexPreferredTurnStateLength)
	newer := strings.Repeat("b", openAICodexPreferredTurnStateLength)
	pool.observe(older, &accountID, sessionHash, "http")
	now = now.Add(time.Second)
	pool.observe(newer, &accountID, sessionHash, "http")

	selected, ok := pool.preferredForSession(accountID, sessionHash)
	require.True(t, ok)
	require.Equal(t, newer, selected)

	pool.mu.Lock()
	pool.entries[hashOpenAICodexTurnState(newer)].ExpiresAt = now.Add(-time.Second)
	pool.preferredBySession[key].ExpiresAt = now.Add(-time.Second)
	pool.mu.Unlock()
	selected, ok = pool.preferredForSession(accountID, sessionHash)
	require.True(t, ok)
	require.Equal(t, older, selected)
}

func TestOpenAICodexTurnStatePool_AccountAndGlobal292UseNewestAndIgnoreOtherLengths(t *testing.T) {
	pool := newOpenAICodexTurnStatePool()
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	pool.now = func() time.Time { return now }
	accountID := int64(42)
	otherAccountID := int64(43)
	accountState := testOpenAICodexPreferredTurnState("account")
	globalState := testOpenAICodexPreferredTurnState("global")
	pool.observe(accountState, &accountID, "session-a", "http")
	now = now.Add(time.Second)
	pool.observe(strings.Repeat("x", openAICodexPreferredTurnStateLength+40), &accountID, "session-b", "http")
	now = now.Add(time.Second)
	pool.observe(globalState, &otherAccountID, "session-c", "http")

	selected, ok := pool.preferredForAccount(accountID)
	require.True(t, ok)
	require.Equal(t, accountState, selected)
	selected, ok = pool.preferredAcrossAccounts()
	require.True(t, ok)
	require.Equal(t, globalState, selected)

	pool.mu.Lock()
	pool.entries[hashOpenAICodexTurnState(globalState)].ExpiresAt = now.Add(-time.Second)
	pool.preferredGlobal.ExpiresAt = now.Add(-time.Second)
	pool.mu.Unlock()
	selected, ok = pool.preferredAcrossAccounts()
	require.True(t, ok)
	require.Equal(t, accountState, selected)
}

func TestOpenAICodexTurnState_UsesCredentialOwnerForShadowAccount(t *testing.T) {
	svc := &OpenAIGatewayService{}
	parent := &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	parentID := parent.ID
	shadow := &Account{ID: 43, ParentAccountID: &parentID, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	c, _ := newTurnStateTestContext(t, 7, "sess-shadow")
	c.Set(codexAccountIdentitySourceContextKey, parent)
	state := strings.Repeat("s", openAICodexPreferredTurnStateLength)

	svc.observeOpenAICodexTurnState(c, shadow, state, "http")
	h := http.Header{}
	svc.guardOpenAICodexTurnStateEcho(c, shadow, h)

	require.Equal(t, state, h.Get(openAICodexTurnStateHeader))
	_, shadowOwnsState := svc.getOpenAICodexTurnStatePool().preferredForSession(shadow.ID, openAICodexTurnStateSessionHash(c))
	require.False(t, shadowOwnsState)
}

func TestWriteOpenAIPassthroughResponseHeaders_RelaysAndClearsTurnState(t *testing.T) {
	// A nil filter uses the content-type fallback; turn-state remains allowlisted.
	dst := http.Header{}
	src := http.Header{}
	src.Set("X-Codex-Turn-State", "blob-P")
	writeOpenAIPassthroughResponseHeaders(dst, src, nil)
	require.Equal(t, "blob-P", dst.Get("X-Codex-Turn-State"))

	// Missing upstream state clears leftovers after account failover.
	writeOpenAIPassthroughResponseHeaders(dst, http.Header{"Content-Type": []string{"application/json"}}, nil)
	require.Empty(t, dst.Get("X-Codex-Turn-State"))
}

func TestWriteOpenAIPassthroughResponseHeaders_RelaysReasoningIncluded(t *testing.T) {
	dst := http.Header{}
	src := http.Header{}
	src.Set("X-Reasoning-Included", "1")

	writeOpenAIPassthroughResponseHeaders(
		dst,
		src,
		responseheaders.CompileHeaderFilter(config.ResponseHeaderConfig{}),
	)
	require.Equal(t, "1", dst.Get("X-Reasoning-Included"))
}

func TestEnsureOpenAIRemoteCompactionV2BetaFeature(t *testing.T) {
	t.Run("absent_sets_feature", func(t *testing.T) {
		h := http.Header{}
		ensureOpenAIRemoteCompactionV2BetaFeature(h)
		require.Equal(t, "remote_compaction_v2", h.Get("x-codex-beta-features"))
	})

	t.Run("present_unchanged", func(t *testing.T) {
		h := http.Header{}
		h.Set("x-codex-beta-features", "responses_websockets_v2, remote_compaction_v2")
		ensureOpenAIRemoteCompactionV2BetaFeature(h)
		require.Equal(t, "responses_websockets_v2, remote_compaction_v2", h.Get("x-codex-beta-features"))
	})

	t.Run("other_tokens_merged", func(t *testing.T) {
		h := http.Header{}
		h.Set("x-codex-beta-features", "responses_websockets_v2")
		ensureOpenAIRemoteCompactionV2BetaFeature(h)
		require.Equal(t, "responses_websockets_v2,remote_compaction_v2", h.Get("x-codex-beta-features"))
	})

	t.Run("multi_line_values_merged_single_line", func(t *testing.T) {
		h := http.Header{}
		h.Add("x-codex-beta-features", "feature_a")
		h.Add("x-codex-beta-features", "feature_b")
		ensureOpenAIRemoteCompactionV2BetaFeature(h)
		require.Equal(t, []string{"feature_a,feature_b,remote_compaction_v2"}, h.Values("x-codex-beta-features"))
	})
}

// Match Codex: this session-level header is sent on every OAuth request, not
// only on compaction turns (codex-rs build_model_client_beta_features_header).
func TestApplyOpenAICodexBetaFeatures(t *testing.T) {
	oauthAccount := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	apiKeyAccount := &Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	t.Run("oauth_plain_request_gets_default_codex_shape", func(t *testing.T) {
		c, _ := newTurnStateTestContext(t, 7, "sess-beta")
		h := http.Header{}
		applyOpenAICodexBetaFeatures(c, oauthAccount, h)
		require.Equal(t, "remote_compaction_v2", h.Get("x-codex-beta-features"),
			"OAuth 的普通请求也必须带会话级 beta 头")
	})

	t.Run("client_declared_header_preserved", func(t *testing.T) {
		c, _ := newTurnStateTestContext(t, 7, "sess-beta")
		h := http.Header{}
		h.Set("x-codex-beta-features", "some_other_feature")
		applyOpenAICodexBetaFeatures(c, oauthAccount, h)
		require.Equal(t, "some_other_feature", h.Get("x-codex-beta-features"),
			"客户端显式声明的能力集不得被网关改写（非空即视为用户已关闭 v2）")
	})

	t.Run("native_v2_forces_feature_even_when_client_trimmed_it", func(t *testing.T) {
		c, _ := newTurnStateTestContext(t, 7, "sess-beta")
		MarkOpenAINativeCompactionV2(c)
		h := http.Header{}
		h.Set("x-codex-beta-features", "some_other_feature")
		applyOpenAICodexBetaFeatures(c, oauthAccount, h)
		require.Contains(t, h.Get("x-codex-beta-features"), "remote_compaction_v2",
			"body 带 compaction_trigger 是实锤，必须确保 v2 在列")
		require.Contains(t, h.Get("x-codex-beta-features"), "some_other_feature")
	})

	t.Run("native_v2_applies_to_non_oauth_too", func(t *testing.T) {
		c, _ := newTurnStateTestContext(t, 7, "sess-beta")
		MarkOpenAINativeCompactionV2(c)
		h := http.Header{}
		applyOpenAICodexBetaFeatures(c, apiKeyAccount, h)
		require.Equal(t, "remote_compaction_v2", h.Get("x-codex-beta-features"))
	})

	t.Run("non_oauth_plain_request_untouched", func(t *testing.T) {
		c, _ := newTurnStateTestContext(t, 7, "sess-beta")
		h := http.Header{}
		applyOpenAICodexBetaFeatures(c, apiKeyAccount, h)
		require.Empty(t, h.Get("x-codex-beta-features"),
			"非 Codex 后端不做会话级注入")
	})

	t.Run("nil_account_plain_request_untouched", func(t *testing.T) {
		c, _ := newTurnStateTestContext(t, 7, "sess-beta")
		h := http.Header{}
		applyOpenAICodexBetaFeatures(c, nil, h)
		require.Empty(t, h.Get("x-codex-beta-features"))
	})
}

// WS handshakes and HTTP requests must share the same session beta header.
// Codex build_websocket_headers reuses build_responses_headers (client.rs),
// and mismatches split warm connections and requests into different buckets.
func TestBuildOpenAIWSHeaders_CarriesSessionBetaFeatures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{}
	decision := OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2}

	build := func(t *testing.T, account *Account, clientBeta string) http.Header {
		t.Helper()
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
		if clientBeta != "" {
			c.Request.Header.Set("x-codex-beta-features", clientBeta)
		}
		headers, _, err := svc.buildOpenAIWSHeaders(
			context.Background(), c, account, "test-token", decision,
			true, "", "", "", "gpt-5.6-codex", "",
		)
		require.NoError(t, err)
		return headers
	}

	oauthAccount := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"chatgpt_account_id": "test-account"},
	}

	headers := build(t, oauthAccount, "")
	require.Equal(t, "remote_compaction_v2", headers.Get("x-codex-beta-features"),
		"WS 握手也必须带会话级 beta 头")

	declared := build(t, oauthAccount, "some_other_feature")
	require.Equal(t, []string{"some_other_feature"}, declared.Values("x-codex-beta-features"),
		"客户端已声明时原样保留")

	apiKeyHeaders := build(t, &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, "")
	require.Empty(t, apiKeyHeaders.Get("x-codex-beta-features"),
		"非 Codex 后端不注入")
}
