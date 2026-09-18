package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
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

func testOpenAICodexTurnState(length int, issuedAt time.Time, marker byte) string {
	if length < 12 || length%4 != 0 {
		panic("test turn-state length must be a base64 block length")
	}
	rawLength := (length/4-1)*3 + 1
	raw := make([]byte, rawLength)
	raw[0] = 0x80
	binary.BigEndian.PutUint64(raw[1:9], uint64(issuedAt.Unix()))
	for i := 9; i < len(raw); i++ {
		raw[i] = marker
	}
	value := base64.URLEncoding.EncodeToString(raw)
	if len(value) != length {
		panic("generated turn-state has unexpected length")
	}
	return value
}

func testOpenAICodexPreferredTurnState(seed string) string {
	marker := sha256.Sum256([]byte(seed))
	return testOpenAICodexTurnState(openAICodexPreferredTurnStateLength, time.Now().UTC().Truncate(time.Second), marker[0])
}

func expireOpenAICodexTurnState(pool *openAICodexTurnStatePool, value string, expiresAt time.Time) {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	for _, record := range pool.entries {
		if record != nil && record.StateValue == value {
			record.ExpiresAt = expiresAt
		}
	}
	if pool.selected != nil && pool.selected.StateValue == value {
		pool.selected.ExpiresAt = expiresAt
	}
}

func TestOpenAICodexTurnStateSeed(t *testing.T) {
	c, _ := newTurnStateTestContext(t, 7, "sess-1")
	require.Equal(t, "7\x00sess-1", openAICodexTurnStateSeed(c))

	c.Request.Header.Set("session-id", "sess-hyphen")
	require.Equal(t, "7\x00sess-hyphen", openAICodexTurnStateSeed(c))

	cNoSession, _ := newTurnStateTestContext(t, 7, "")
	require.Empty(t, openAICodexTurnStateSeed(cNoSession))
	require.Empty(t, openAICodexTurnStateSeed(nil))
}

func TestRelayOpenAICodexTurnState_StoresOAuthState(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	state := testOpenAICodexTurnState(312, now, 'a')
	svc := &OpenAIGatewayService{}
	account := &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	c, _ := newTurnStateTestContext(t, 7, "sess-relay")
	stageOpenAICodexTurnStateModel(c, "gpt-5.6-codex")
	upstream := http.Header{"X-Codex-Turn-State": []string{state}}

	svc.relayOpenAICodexTurnState(c, account, upstream)

	require.Equal(t, state, c.Writer.Header().Get("X-Codex-Turn-State"))
	selected, ok := svc.getOpenAICodexTurnStatePool().longestActive()
	require.True(t, ok)
	require.Equal(t, state, selected)
}

func TestStageOpenAICodexTurnState_StoresOnlyAfterCommit(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	state := testOpenAICodexTurnState(292, now, 'b')
	svc := &OpenAIGatewayService{}
	account := &Account{ID: 44, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	c, _ := newTurnStateTestContext(t, 9, "sess-staged")
	stageOpenAICodexTurnStateModel(c, "gpt-5.6-codex")
	var staged http.Header
	stageOpenAICodexTurnState(&staged, http.Header{"X-Codex-Turn-State": []string{state}})

	_, ok := svc.getOpenAICodexTurnStatePool().longestActive()
	require.False(t, ok)
	svc.noteStagedOpenAICodexTurnStateCommitted(c, account, staged)
	selected, ok := svc.getOpenAICodexTurnStatePool().longestActive()
	require.True(t, ok)
	require.Equal(t, state, selected)

	stageOpenAICodexTurnState(&staged, http.Header{})
	require.Empty(t, staged.Get("X-Codex-Turn-State"))
}

func TestOpenAICodexTurnStatePool_SelectsLongestAndFallsBackAfterExpiry(t *testing.T) {
	pool := newOpenAICodexTurnStatePool()
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	pool.now = func() time.Time { return now }
	short := testOpenAICodexTurnState(292, now, 'a')
	longest := testOpenAICodexTurnState(356, now, 'b')
	pool.observe(short, nil, "", "", "http")
	pool.observe(longest, nil, "", "", "ws")

	selected, ok := pool.longestActive()
	require.True(t, ok)
	require.Equal(t, longest, selected)

	expireOpenAICodexTurnState(pool, longest, now.Add(-time.Second))
	selected, ok = pool.longestActive()
	require.True(t, ok)
	require.Equal(t, short, selected)
}

func TestOpenAICodexTurnStatePool_EqualLengthUsesNewest(t *testing.T) {
	pool := newOpenAICodexTurnStatePool()
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	pool.now = func() time.Time { return now }
	first := testOpenAICodexTurnState(292, now, 'a')
	pool.observe(first, nil, "", "", "http")
	now = now.Add(time.Second)
	later := testOpenAICodexTurnState(292, now, 'b')
	pool.observe(later, nil, "", "", "http")

	selected, ok := pool.longestActive()
	require.True(t, ok)
	require.Equal(t, later, selected)
}

func TestOpenAICodexTurnStatePool_CleanupRemovesExpiredEntries(t *testing.T) {
	pool := newOpenAICodexTurnStatePool()
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	pool.now = func() time.Time { return now }
	expired := testOpenAICodexTurnState(292, now, 'a')
	active := testOpenAICodexTurnState(312, now, 'b')
	pool.observe(expired, nil, "", "", "http")
	pool.observe(active, nil, "", "", "http")
	expireOpenAICodexTurnState(pool, expired, now.Add(-time.Second))

	pool.cleanupExpired(now)

	pool.mu.RLock()
	defer pool.mu.RUnlock()
	require.Len(t, pool.entries, 1)
	for _, record := range pool.entries {
		require.Equal(t, active, record.StateValue)
	}
}

func TestGuardOpenAICodexTurnStateEcho_OnlyReplaces312ForSameAccountAndModel(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	svc := &OpenAIGatewayService{}
	pool := svc.getOpenAICodexTurnStatePool()
	pool.now = func() time.Time { return now }
	accountID := int64(43)
	otherAccountID := int64(42)
	model := "gpt-5.6-codex"
	preferred := testOpenAICodexTurnState(292, now, 'a')
	otherAccount := testOpenAICodexTurnState(292, now, 'b')
	pool.observe(preferred, &accountID, "source-session", model, "http")
	pool.observe(otherAccount, &otherAccountID, "source-session", model, "http")

	c, _ := newTurnStateTestContext(t, 7, "target-session")
	incoming := testOpenAICodexTurnState(312, now, 'c')
	h := http.Header{"X-Codex-Turn-State": []string{incoming}}
	svc.guardOpenAICodexTurnStateEcho(c, &Account{ID: accountID, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, h, model)

	require.Equal(t, preferred, h.Get(openAICodexTurnStateHeader))
	require.NotEqual(t, otherAccount, h.Get(openAICodexTurnStateHeader))
	require.Equal(t, openAICodexTurnStateReuseScopeAccountModel, c.GetString(openAICodexTurnStateReuseScopeContextKey))
}

func TestGuardOpenAICodexTurnStateEcho_DoesNotCrossAccountOrModel(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	svc := &OpenAIGatewayService{}
	pool := svc.getOpenAICodexTurnStatePool()
	pool.now = func() time.Time { return now }
	sourceID := int64(42)
	state := testOpenAICodexTurnState(292, now, 'a')
	pool.observe(state, &sourceID, "source-session", "gpt-5.6-codex", "http")
	incoming := testOpenAICodexTurnState(312, now, 'b')

	t.Run("other_account", func(t *testing.T) {
		c, _ := newTurnStateTestContext(t, 7, "target")
		h := http.Header{"X-Codex-Turn-State": []string{incoming}}
		svc.guardOpenAICodexTurnStateEcho(c, &Account{ID: 43, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, h, "gpt-5.6-codex")
		require.Equal(t, incoming, h.Get(openAICodexTurnStateHeader))
	})

	t.Run("other_model", func(t *testing.T) {
		c, _ := newTurnStateTestContext(t, 7, "target")
		h := http.Header{"X-Codex-Turn-State": []string{incoming}}
		svc.guardOpenAICodexTurnStateEcho(c, &Account{ID: sourceID, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, h, "gpt-5.5-codex")
		require.Equal(t, incoming, h.Get(openAICodexTurnStateHeader))
	})
}

func TestOpenAICodexTurnStatePool_KeepsSameStateSeparatedByAccount(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	pool := newOpenAICodexTurnStatePool()
	pool.now = func() time.Time { return now }
	firstID := int64(42)
	secondID := int64(43)
	model := "gpt-5.6-codex"
	state := testOpenAICodexTurnState(292, now, 'a')

	pool.observe(state, &firstID, "first", model, "http")
	pool.observe(state, &secondID, "second", model, "http")

	require.Len(t, pool.entries, 2)
	first, firstOK := pool.preferredForBucket(firstID, model)
	second, secondOK := pool.preferredForBucket(secondID, model)
	require.True(t, firstOK)
	require.True(t, secondOK)
	require.Equal(t, state, first)
	require.Equal(t, state, second)
}

func TestGuardOpenAICodexTurnStateEcho_PreservesNon312Inputs(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	svc := &OpenAIGatewayService{}
	pool := svc.getOpenAICodexTurnStatePool()
	pool.now = func() time.Time { return now }
	accountID := int64(43)
	model := "gpt-5.6-codex"
	pool.observe(testOpenAICodexTurnState(292, now, 'a'), &accountID, "source", model, "http")
	account := &Account{ID: accountID, Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	for _, length := range []int{292, 332, 356} {
		t.Run(http.StatusText(length), func(t *testing.T) {
			c, _ := newTurnStateTestContext(t, 7, "target")
			incoming := testOpenAICodexTurnState(length, now, byte(length))
			h := http.Header{"X-Codex-Turn-State": []string{incoming}}
			svc.guardOpenAICodexTurnStateEcho(c, account, h, model)
			require.Equal(t, incoming, h.Get(openAICodexTurnStateHeader))
		})
	}

	c, _ := newTurnStateTestContext(t, 7, "target-empty")
	h := http.Header{}
	svc.guardOpenAICodexTurnStateEcho(c, account, h, model)
	require.Empty(t, h.Get(openAICodexTurnStateHeader))
}

func TestGuardOpenAICodexTurnStateEcho_NoTemplatePreserves312(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	incoming := testOpenAICodexTurnState(312, now, 'a')
	svc := &OpenAIGatewayService{}
	c, _ := newTurnStateTestContext(t, 7, "target")
	h := http.Header{"X-Codex-Turn-State": []string{incoming}}

	svc.guardOpenAICodexTurnStateEcho(c, &Account{ID: 43, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, h, "gpt-5.6-codex")

	require.Equal(t, incoming, h.Get(openAICodexTurnStateHeader))
}

func TestGuardOpenAICodexTurnStateEcho_APIKeyUnaffected(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	incoming := testOpenAICodexTurnState(312, now, 'a')
	svc := &OpenAIGatewayService{}
	c, _ := newTurnStateTestContext(t, 7, "sess-apikey")
	h := http.Header{"X-Codex-Turn-State": []string{incoming}}

	svc.guardOpenAICodexTurnStateEcho(c, &Account{ID: 99, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, h, "gpt-5.6-codex")

	require.Equal(t, incoming, h.Get(openAICodexTurnStateHeader))
}

func TestOpenAICodexTurnStatePool_UsesFernetIssuedAtForExpiry(t *testing.T) {
	pool := newOpenAICodexTurnStatePool()
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	pool.now = func() time.Time { return now }
	accountID := int64(42)
	model := "gpt-5.6-codex"

	expired := testOpenAICodexTurnState(292, now.Add(-time.Hour), 'a')
	future := testOpenAICodexTurnState(292, now.Add(time.Second), 'b')
	active := testOpenAICodexTurnState(292, now.Add(-30*time.Minute), 'c')
	pool.observe(expired, &accountID, "expired", model, "http")
	pool.observe(future, &accountID, "future", model, "http")
	pool.observe(active, &accountID, "active", model, "http")

	selected, ok := pool.preferredForBucket(accountID, model)
	require.True(t, ok)
	require.Equal(t, active, selected)

	issuedAt, ok := parseOpenAICodexTurnStateIssuedAt(active)
	require.True(t, ok)
	require.Equal(t, now.Add(-30*time.Minute), issuedAt)
	for _, record := range pool.entries {
		if record.StateValue == active {
			require.Equal(t, issuedAt.Add(time.Hour), record.ExpiresAt)
		}
	}

	now = issuedAt.Add(time.Hour)
	_, ok = pool.preferredForBucket(accountID, model)
	require.False(t, ok)
}

func TestBuildOpenAIWSHeaders_UsesSameAccountModel292State(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	svc := &OpenAIGatewayService{}
	pool := svc.getOpenAICodexTurnStatePool()
	pool.now = func() time.Time { return now }
	accountID := int64(43)
	model := "gpt-5.6-codex"
	accountState := testOpenAICodexTurnState(292, now, 'a')
	incoming := testOpenAICodexTurnState(312, now, 'b')
	pool.observe(accountState, &accountID, "source-session", model, "http")
	c, _ := newTurnStateTestContext(t, 7, "sess-ws")
	account := &Account{
		ID:          accountID,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"chatgpt_account_id": "test-account"},
	}

	headers, _, err := svc.buildOpenAIWSHeaders(
		context.Background(), c, account, "token",
		OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2},
		true, incoming, "", "", model, "",
	)
	require.NoError(t, err)
	require.Equal(t, accountState, headers.Get(openAIWSTurnStateHeader))
}

func TestOpenAICodexTurnState_UsesCredentialOwnerForShadowAccount(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	svc := &OpenAIGatewayService{}
	pool := svc.getOpenAICodexTurnStatePool()
	pool.now = func() time.Time { return now }
	parent := &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	parentID := parent.ID
	shadow := &Account{ID: 43, ParentAccountID: &parentID, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	c, _ := newTurnStateTestContext(t, 7, "sess-shadow")
	c.Set(codexAccountIdentitySourceContextKey, parent)
	model := "gpt-5.6-codex"
	stageOpenAICodexTurnStateModel(c, model)
	state := testOpenAICodexTurnState(292, now, 's')
	svc.observeOpenAICodexTurnState(c, shadow, state, "http")

	incoming := testOpenAICodexTurnState(312, now, 'i')
	h := http.Header{"X-Codex-Turn-State": []string{incoming}}
	svc.guardOpenAICodexTurnStateEcho(c, shadow, h, model)

	require.Equal(t, state, h.Get(openAICodexTurnStateHeader))
	_, shadowOwnsState := pool.preferredForBucket(shadow.ID, model)
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
