package service

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newOpenAICodexDelegationTestContext(t *testing.T) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
	c.Request.Header.Set("User-Agent", "Codex Desktop/0.148.0-alpha.9")
	c.Request.Header.Set("originator", "Codex Desktop")
	c.Request.Header.Set(openAICodexParentThreadIDHeader, "019fc825-ceab-75f2-b05d-176dcc60a898")
	c.Request.Header.Set(openAICodexSubagentHeader, "review")
	return c
}

func TestCopyOpenAICodexDelegationHeadersEligibilityAndValidation(t *testing.T) {
	c := newOpenAICodexDelegationTestContext(t)
	oauth := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	t.Run("official OAuth preserves current desktop lineage", func(t *testing.T) {
		headers := make(http.Header)
		copyOpenAICodexDelegationHeaders(c, oauth, true, headers)
		require.Equal(t, "019fc825-ceab-75f2-b05d-176dcc60a898", headers.Get(openAICodexParentThreadIDHeader))
		require.Equal(t, "review", headers.Get(openAICodexSubagentHeader))
	})

	t.Run("API key cannot synthesize private lineage", func(t *testing.T) {
		headers := make(http.Header)
		copyOpenAICodexDelegationHeaders(c, &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, true, headers)
		require.Empty(t, headers.Get(openAICodexParentThreadIDHeader))
		require.Empty(t, headers.Get(openAICodexSubagentHeader))
	})

	t.Run("unidentified client cannot synthesize private lineage", func(t *testing.T) {
		headers := make(http.Header)
		copyOpenAICodexDelegationHeaders(c, oauth, false, headers)
		require.Empty(t, headers.Get(openAICodexParentThreadIDHeader))
		require.Empty(t, headers.Get(openAICodexSubagentHeader))
	})

	t.Run("oversized and non ASCII values are dropped independently", func(t *testing.T) {
		invalid := newOpenAICodexDelegationTestContext(t)
		invalid.Request.Header.Set(openAICodexParentThreadIDHeader, strings.Repeat("a", openAICodexDelegationHeaderMaxBytes+1))
		invalid.Request.Header.Set(openAICodexSubagentHeader, "子代理")
		headers := make(http.Header)
		copyOpenAICodexDelegationHeaders(invalid, oauth, true, headers)
		require.Empty(t, headers.Get(openAICodexParentThreadIDHeader))
		require.Empty(t, headers.Get(openAICodexSubagentHeader))
	})
}

func TestOpenAICodexDelegationHeadersReachHTTPPassthroughAndWS(t *testing.T) {
	account := &Account{
		ID:       194,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"chatgpt_account_id": "chatgpt-account",
		},
	}
	body := []byte(`{"model":"gpt-5.6-sol","stream":true,"store":false,"input":[],"prompt_cache_key":"session-194"}`)

	t.Run("HTTP", func(t *testing.T) {
		c := newOpenAICodexDelegationTestContext(t)
		svc := &OpenAIGatewayService{}
		req, err := svc.buildUpstreamRequest(context.Background(), c, account, body, "token", true, "session-194", true)
		require.NoError(t, err)
		require.Equal(t, c.GetHeader(openAICodexParentThreadIDHeader), req.Header.Get(openAICodexParentThreadIDHeader))
		require.Equal(t, c.GetHeader(openAICodexSubagentHeader), req.Header.Get(openAICodexSubagentHeader))
	})

	t.Run("passthrough", func(t *testing.T) {
		c := newOpenAICodexDelegationTestContext(t)
		svc := &OpenAIGatewayService{}
		req, err := svc.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, account, body, "token")
		require.NoError(t, err)
		require.Equal(t, c.GetHeader(openAICodexParentThreadIDHeader), req.Header.Get(openAICodexParentThreadIDHeader))
		require.Equal(t, c.GetHeader(openAICodexSubagentHeader), req.Header.Get(openAICodexSubagentHeader))
	})

	t.Run("WS", func(t *testing.T) {
		c := newOpenAICodexDelegationTestContext(t)
		svc := &OpenAIGatewayService{}
		headers, _, err := svc.buildOpenAIWSHeaders(
			context.Background(), c, account, "token",
			OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2},
			true, "", "", "session-194", "gpt-5.6-sol", "",
		)
		require.NoError(t, err)
		require.Equal(t, c.GetHeader(openAICodexParentThreadIDHeader), headers.Get(openAICodexParentThreadIDHeader))
		require.Equal(t, c.GetHeader(openAICodexSubagentHeader), headers.Get(openAICodexSubagentHeader))
	})
}

func TestOpenAIWSHandshakeCompatibilitySeparatesDelegationLineage(t *testing.T) {
	base := http.Header{
		"X-Codex-Beta-Features":    []string{"remote_compaction_v2"},
		"X-Codex-Parent-Thread-Id": []string{"thread-parent-a"},
		"X-Openai-Subagent":        []string{"review"},
	}
	baseKey := normalizeOpenAIWSHandshakeCompatibility(nil, base)

	differentParent := base.Clone()
	differentParent.Set(openAICodexParentThreadIDHeader, "thread-parent-b")
	require.NotEqual(t, baseKey, normalizeOpenAIWSHandshakeCompatibility(nil, differentParent))

	differentSubagent := base.Clone()
	differentSubagent.Set(openAICodexSubagentHeader, "memory")
	require.NotEqual(t, baseKey, normalizeOpenAIWSHandshakeCompatibility(nil, differentSubagent))

	unrelated := base.Clone()
	unrelated.Set("Accept-Language", "zh-CN")
	require.Equal(t, baseKey, normalizeOpenAIWSHandshakeCompatibility(nil, unrelated))
}
