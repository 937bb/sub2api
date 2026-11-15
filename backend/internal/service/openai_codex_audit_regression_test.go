package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestCodexParentReferencesShareExistingThreadAndTurnNamespace(t *testing.T) {
	account := accountDeviceFixture(11, "owner")
	body := []byte(`{"input":[],"client_metadata":{"parent_thread_id":"parent","x-codex-parent-thread-id":"parent","forked_from_thread_id":"parent","parent_turn_id":"turn","x-codex-turn-metadata":"{\"parent_thread_id\":\"parent\",\"parent_turn_id\":\"turn\",\"counter\":9007199254740993,\"analytics_enabled\":false}"}}`)
	parent := scopeCodexAccountIdentityValue(account, 77, "thread", "parent")
	turn := scopeCodexAccountIdentityValue(account, 77, "turn", "turn")
	out, changed, err := applyCodexAccountIdentityClientMetadataRaw(body, account, 77)
	require.NoError(t, err)
	require.True(t, changed)
	for _, name := range []string{"parent_thread_id", "x-codex-parent-thread-id", "forked_from_thread_id"} {
		require.Equal(t, parent, gjson.GetBytes(out, "client_metadata."+name).String())
	}
	require.Equal(t, turn, gjson.GetBytes(out, "client_metadata.parent_turn_id").String())
	embedded := gjson.GetBytes(out, "client_metadata.x-codex-turn-metadata").String()
	require.Equal(t, parent, gjson.Get(embedded, "parent_thread_id").String())
	require.Equal(t, turn, gjson.Get(embedded, "parent_turn_id").String())
	require.Equal(t, "9007199254740993", gjson.Get(embedded, "counter").Raw)
	require.Equal(t, "false", gjson.Get(embedded, "analytics_enabled").Raw)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(body, &decoded))
	require.True(t, applyCodexAccountIdentityClientMetadataMap(decoded, account, 77))
	encoded, err := json.Marshal(decoded)
	require.NoError(t, err)
	require.JSONEq(t, string(out), string(encoded))
	other, _, err := applyCodexAccountIdentityClientMetadataRaw(body, account, 78)
	require.NoError(t, err)
	require.NotEqual(t, parent, gjson.GetBytes(other, "client_metadata.parent_thread_id").String())
	require.Contains(t, string(body), `"parent_thread_id":"parent"`)
}

func TestCodexSubagentHeadersSurviveHTTPPassthroughAndWS(t *testing.T) {
	account := accountDeviceFixture(11, "owner")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	c.Set("api_key", &APIKey{ID: 77})
	c.Request.Header.Set(codexParentThreadIDHeader, "parent")
	c.Request.Header.Set(openAISubagentHeader, "guardian")
	svc := &OpenAIGatewayService{}
	body := []byte(`{"input":[]}`)
	req, err := svc.buildUpstreamRequest(context.Background(), c, account, body, "fixture", true, "", true)
	require.NoError(t, err)
	passthrough, err := svc.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, account, body, "fixture")
	require.NoError(t, err)
	ws, _, err := svc.buildOpenAIWSHeaders(context.Background(), c, account, "fixture", OpenAIWSProtocolDecision{}, true, "", "", "", "", "")
	require.NoError(t, err)
	for _, h := range []http.Header{req.Header, passthrough.Header, ws} {
		require.Equal(t, "guardian", h.Get(openAISubagentHeader))
		require.Equal(t, scopeCodexAccountIdentityValue(account, 77, "thread", "parent"), h.Get(codexParentThreadIDHeader))
	}
	require.Equal(t, "parent", c.Request.Header.Get(codexParentThreadIDHeader))
}

func TestCodexMetadataPreservesExecutionAndAnalytics(t *testing.T) {
	account := accountDeviceFixture(11, "owner")
	for _, raw := range []string{
		`{"client_metadata":{"thread_id":"fixture"}}`,
		`{"client_metadata":{"thread_id":"fixture","analytics_enabled":false,"sandbox":"seatbelt","sandbox_mode":"read-only","thread_source":"user","turn_trigger":"user","request_kind":"turn","agent_name":"main","model":"gpt-6.1-sol","window_number":2,"context_window_id":"window","history_ingest_requested":true,"history_initialization":"startup","forked_from_ordinal_exclusive":4,"auto_review_enabled":true,"node_repl_auto_review_required":false,"node_repl_disabled":false,"token_provider":"chatgpt","guardian_credits_requested":3,"guardian_recorded_items":1,"mcp_attribution":{"server":"fixture"},"tool_namespaces_info":[{"name":"mcp"}],"code_mode_tool_names":["exec"],"workspaces":["/workspace"],"responses_api_metadata":{"trace_id":"real-inbound"},"traceparent":"00-0123456789abcdef0123456789abcdef-0123456789abcdef-01","tracestate":"vendor=value","baggage":"fixture=value"}}`,
	} {
		out, _, err := applyCodexAccountIdentityClientMetadataRaw([]byte(raw), account, 77)
		require.NoError(t, err)
		for _, field := range []string{
			"analytics_enabled", "sandbox", "sandbox_mode", "thread_source", "turn_trigger",
			"request_kind", "agent_name", "model", "window_number",
			"history_ingest_requested", "history_initialization", "forked_from_ordinal_exclusive",
			"auto_review_enabled", "node_repl_auto_review_required", "node_repl_disabled",
			"timezone", "locale", "token_provider", "guardian_credits_requested",
			"guardian_recorded_items", "mcp_attribution", "tool_namespaces_info",
			"code_mode_tool_names", "workspaces", "responses_api_metadata",
			"traceparent", "tracestate", "baggage",
		} {
			require.Equal(t, gjson.Get(raw, "client_metadata."+field).Raw, gjson.GetBytes(out, "client_metadata."+field).Raw)
		}
		if contextWindow := gjson.Get(raw, "client_metadata.context_window_id"); contextWindow.Exists() {
			require.NotEqual(t, contextWindow.String(), gjson.GetBytes(out, "client_metadata.context_window_id").String())
		}
	}
}

func TestCodexFingerprintLinkedReferencesUseOneNamespace(t *testing.T) {
	account := newTestOAuthAccount(81, map[string]any{codexFingerprintModeExtraKey: "session"})
	seed, ok := codexFingerprintSeed(account.Extra)
	require.True(t, ok)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("session-id", "child-thread")
	c.Request.Header.Set(codexParentThreadIDHeader, "parent-thread")
	c.Request.Header.Set(openAIWSTurnMetadataHeader, `{"thread_id":"child-thread","turn_id":"child-turn","root_turn_id":"child-turn","parent_thread_id":"parent-thread","parent_turn_id":"parent-turn"}`)
	c.Set("api_key", &APIKey{ID: 77})
	ids := resolveCodexFingerprintIDsForAttempt(c, account, c.Request.Header)
	require.NotNil(t, ids)
	stageCodexFingerprintIDs(c, ids)

	body := []byte(`{"input":[],"client_metadata":{"session_id":"child-thread","thread_id":"child-thread","turn_id":"child-turn","root_turn_id":"child-turn","parent_thread_id":"parent-thread","x-codex-parent-thread-id":"parent-thread","forked_from_thread_id":"parent-thread","parent_turn_id":"parent-turn","x-codex-turn-metadata":"{\"thread_id\":\"child-thread\",\"turn_id\":\"child-turn\",\"root_turn_id\":\"child-turn\",\"parent_thread_id\":\"parent-thread\",\"parent_turn_id\":\"parent-turn\"}"}}`)
	out, changed, err := applyCodexOutboundIdentityClientMetadataRaw(c, account, body)
	require.NoError(t, err)
	require.True(t, changed)
	wantParentThread := resolveConvergedThreadID(seed, "parent-thread")
	wantParentTurn := resolveConvergedTurnID(seed, "parent-turn")
	wantChildTurn := resolveConvergedTurnID(seed, "child-turn")
	for _, name := range []string{"parent_thread_id", "x-codex-parent-thread-id", "forked_from_thread_id"} {
		require.Equal(t, wantParentThread, gjson.GetBytes(out, "client_metadata."+name).String())
	}
	require.Equal(t, wantParentTurn, gjson.GetBytes(out, "client_metadata.parent_turn_id").String())
	require.Equal(t, wantChildTurn, gjson.GetBytes(out, "client_metadata.turn_id").String())
	require.Equal(t, wantChildTurn, gjson.GetBytes(out, "client_metadata.root_turn_id").String())
	embedded := gjson.GetBytes(out, "client_metadata.x-codex-turn-metadata").String()
	require.Equal(t, wantParentThread, gjson.Get(embedded, "parent_thread_id").String())
	require.Equal(t, wantParentTurn, gjson.Get(embedded, "parent_turn_id").String())
	require.Equal(t, wantChildTurn, gjson.Get(embedded, "turn_id").String())
	require.Equal(t, wantChildTurn, gjson.Get(embedded, "root_turn_id").String())

	headers := c.Request.Header.Clone()
	applyCodexOutboundIdentityHeaders(c, account, headers)
	require.Equal(t, wantParentThread, headers.Get(codexParentThreadIDHeader))
	headerMetadata := headers.Get(openAIWSTurnMetadataHeader)
	require.Equal(t, wantParentThread, gjson.Get(headerMetadata, "parent_thread_id").String())
	require.Equal(t, wantParentTurn, gjson.Get(headerMetadata, "parent_turn_id").String())
	require.Equal(t, wantChildTurn, gjson.Get(headerMetadata, "turn_id").String())
	require.Equal(t, wantChildTurn, gjson.Get(headerMetadata, "root_turn_id").String())

	doubleMapped := scopeCodexAccountIdentityValue(account, 77, "thread", wantParentThread)
	require.NotEqual(t, doubleMapped, wantParentThread)
	require.NotEqual(t, doubleMapped, headers.Get(codexParentThreadIDHeader))
}

func TestCodexFingerprintFullModeKeepsParentOnCollapsedThread(t *testing.T) {
	account := newTestOAuthAccount(82, map[string]any{codexFingerprintModeExtraKey: "full"})
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("session-id", "child-thread")
	c.Set("api_key", &APIKey{ID: 77})
	ids := resolveCodexFingerprintIDsForAttempt(c, account, c.Request.Header)
	require.NotNil(t, ids)
	stageCodexFingerprintIDs(c, ids)
	body := []byte(`{"client_metadata":{"thread_id":"child-thread","parent_thread_id":"parent-thread","forked_from_thread_id":"parent-thread"}}`)
	out, changed, err := applyCodexOutboundIdentityClientMetadataRaw(c, account, body)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, ids.threadID, gjson.GetBytes(out, "client_metadata.thread_id").String())
	require.Equal(t, ids.threadID, gjson.GetBytes(out, "client_metadata.parent_thread_id").String())
	require.Equal(t, ids.threadID, gjson.GetBytes(out, "client_metadata.forked_from_thread_id").String())
}

func TestCodexOutboundTurnMetadataUsesStagedNamespaceOnce(t *testing.T) {
	account := newTestOAuthAccount(83, map[string]any{codexFingerprintModeExtraKey: "session"})
	seed, ok := codexFingerprintSeed(account.Extra)
	require.True(t, ok)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("session-id", "child-thread")
	c.Set("api_key", &APIKey{ID: 77})
	ids := resolveCodexFingerprintIDsForAttempt(c, account, c.Request.Header)
	require.NotNil(t, ids)
	stageCodexFingerprintIDs(c, ids)

	raw := `{"thread_id":"child-thread","turn_id":"child-turn","root_turn_id":"child-turn","parent_thread_id":"parent-thread","parent_turn_id":"parent-turn","analytics_enabled":false}`
	out := applyCodexOutboundIdentityTurnMetadata(c, account, raw)
	wantParentThread := resolveConvergedThreadID(seed, "parent-thread")
	wantParentTurn := resolveConvergedTurnID(seed, "parent-turn")
	wantChildTurn := resolveConvergedTurnID(seed, "child-turn")
	require.Equal(t, ids.threadID, gjson.Get(out, "thread_id").String())
	require.Equal(t, wantChildTurn, gjson.Get(out, "turn_id").String())
	require.Equal(t, wantChildTurn, gjson.Get(out, "root_turn_id").String())
	require.Equal(t, wantParentThread, gjson.Get(out, "parent_thread_id").String())
	require.Equal(t, wantParentTurn, gjson.Get(out, "parent_turn_id").String())
	require.False(t, gjson.Get(out, "analytics_enabled").Bool())
	require.NotEqual(t, resolveConvergedThreadID(seed, wantParentThread), gjson.Get(out, "parent_thread_id").String())
}

func TestCodexCompatIdentityDoesNotInventDesktopMetadata(t *testing.T) {
	account := newTestOAuthAccount(84, map[string]any{codexFingerprintModeExtraKey: "session"})
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Request.Header.Set("session-id", "desktop-session")

	plain := map[string]any{"prompt_cache_key": "compat-cache"}
	stageCodexOutboundIdentityForExistingMetadata(c, account, plain)
	require.Nil(t, stagedCodexFingerprintIDs(c, account))
	applyCodexOutboundIdentityClientMetadataMap(c, account, plain)
	require.NotContains(t, plain, "client_metadata")
	nullMetadata := map[string]any{"client_metadata": nil}
	stageCodexOutboundIdentityForExistingMetadata(c, account, nullMetadata)
	require.Nil(t, stagedCodexFingerprintIDs(c, account))
	applyCodexOutboundIdentityClientMetadataMap(c, account, nullMetadata)
	require.Nil(t, nullMetadata["client_metadata"])

	desktop := map[string]any{"client_metadata": map[string]any{"thread_id": "desktop-thread"}}
	stageCodexOutboundIdentityForExistingMetadata(c, account, desktop)
	require.NotNil(t, stagedCodexFingerprintIDs(c, account))
	require.True(t, applyCodexOutboundIdentityClientMetadataMap(c, account, desktop))
	require.NotEqual(t, "desktop-thread", gjson.Get(requestToJSONString(desktop), "client_metadata.thread_id").String())
}

func TestCodexLegacyFingerprintNoHeaderKeepsWarmCacheKeyAcrossMapRawAndWSFrame(t *testing.T) {
	seed := "11111111-1111-4111-8111-111111111111"
	account := newTestOAuthAccount(85, map[string]any{
		codexFingerprintModeExtraKey: "session",
		codexFingerprintSeedExtraKey: seed,
	})
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	want := deriveLegacyCodexUUIDv4("sub2api:codex-session-id:v2:" + seed)
	stage := func() {
		ids := resolveCodexFingerprintIDsForAttempt(c, account, c.Request.Header)
		require.NotNil(t, ids)
		require.Equal(t, want, ids.sessionID)
		stageCodexFingerprintIDs(c, ids)
	}

	stage()
	httpMap := map[string]any{
		"prompt_cache_key": "body-session",
		"client_metadata":  map[string]any{"session_id": "body-session"},
	}
	require.True(t, applyCodexOutboundIdentityClientMetadataMap(c, account, httpMap))
	require.Equal(t, want, httpMap["prompt_cache_key"])

	for _, tc := range []struct {
		name string
		body []byte
	}{
		{name: "raw passthrough", body: []byte(`{"prompt_cache_key":"body-session","client_metadata":{"session_id":"body-session"}}`)},
		{name: "websocket response.create", body: []byte(`{"type":"response.create","prompt_cache_key":"body-session","client_metadata":{"session_id":"body-session"}}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stage()
			out, changed, err := applyCodexOutboundIdentityClientMetadataRaw(c, account, tc.body)
			require.NoError(t, err)
			require.True(t, changed)
			require.Equal(t, want, gjson.GetBytes(out, "prompt_cache_key").String())
			require.Equal(t, want, gjson.GetBytes(out, "client_metadata.session_id").String())
		})
	}
}
