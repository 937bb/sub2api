package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func mustRawJSON(t *testing.T, s string) json.RawMessage {
	t.Helper()
	return json.RawMessage(s)
}

func TestShouldAutoInjectPromptCacheKeyForCompat(t *testing.T) {
	require.True(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-5.5"))
	require.True(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-5.5-pro"))
	require.True(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-5.4"))
	require.True(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-5.4-mini"))
	require.True(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-5.2"))
	require.True(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-5.3"))
	require.True(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-5.3-codex"))
	require.True(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-5.3-codex-spark"))
	require.False(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-4o"))
}

func TestShouldAutoInjectPromptCacheKeyForCompat_GPT6AstraForms(t *testing.T) {
	for _, model := range []string{
		"gpt-6",
		"gpt-6-astra",
		"openai/gpt-6",
		"openai/gpt-6-astra",
		"OPENAI/GPT-6_ASTRA",
		"provider/gpt-6-astra",
	} {
		require.True(t, shouldAutoInjectPromptCacheKeyForCompat(model), model)
	}

	for _, model := range []string{
		"gpt-6-terra",
		"gpt-6.1",
		"gpt-6-astra-preview",
		"gpt-6-astra-unrelated-name",
		"gpt-6-astra-2026-09-01",
		"gpt-6-astra-2026-09-01-extra",
		"gpt-6-astra-v",
		"gpt-6-astra-v1.beta",
		"claude-sonnet-4-5",
		"gpt-4o",
	} {
		require.False(t, shouldAutoInjectPromptCacheKeyForCompat(model), model)
	}
}

func TestDeriveCompatPromptCacheKey_StableAcrossLaterTurns(t *testing.T) {
	base := &apicompat.ChatCompletionsRequest{
		Model: "gpt-5.4",
		Messages: []apicompat.ChatMessage{
			{Role: "system", Content: mustRawJSON(t, `"You are helpful."`)},
			{Role: "user", Content: mustRawJSON(t, `"Hello"`)},
		},
	}
	extended := &apicompat.ChatCompletionsRequest{
		Model: "gpt-5.4",
		Messages: []apicompat.ChatMessage{
			{Role: "system", Content: mustRawJSON(t, `"You are helpful."`)},
			{Role: "user", Content: mustRawJSON(t, `"Hello"`)},
			{Role: "assistant", Content: mustRawJSON(t, `"Hi there!"`)},
			{Role: "user", Content: mustRawJSON(t, `"How are you?"`)},
		},
	}

	k1 := deriveCompatPromptCacheKey(base, "gpt-5.4")
	k2 := deriveCompatPromptCacheKey(extended, "gpt-5.4")
	require.Equal(t, k1, k2, "cache key should be stable across later turns")
	require.NotEmpty(t, k1)
}

func TestDeriveCompatPromptCacheKey_DiffersAcrossSessions(t *testing.T) {
	req1 := &apicompat.ChatCompletionsRequest{
		Model: "gpt-5.4",
		Messages: []apicompat.ChatMessage{
			{Role: "user", Content: mustRawJSON(t, `"Question A"`)},
		},
	}
	req2 := &apicompat.ChatCompletionsRequest{
		Model: "gpt-5.4",
		Messages: []apicompat.ChatMessage{
			{Role: "user", Content: mustRawJSON(t, `"Question B"`)},
		},
	}

	k1 := deriveCompatPromptCacheKey(req1, "gpt-5.4")
	k2 := deriveCompatPromptCacheKey(req2, "gpt-5.4")
	require.NotEqual(t, k1, k2, "different first user messages should yield different keys")
}

func TestDeriveCompatPromptCacheKey_UsesResolvedSparkFamily(t *testing.T) {
	req := &apicompat.ChatCompletionsRequest{
		Model: "gpt-5.3-codex-spark",
		Messages: []apicompat.ChatMessage{
			{Role: "user", Content: mustRawJSON(t, `"Question A"`)},
		},
	}

	k1 := deriveCompatPromptCacheKey(req, "gpt-5.3-codex-spark")
	k2 := deriveCompatPromptCacheKey(req, " openai/gpt-5.3-codex-spark ")
	require.NotEmpty(t, k1)
	require.Equal(t, k1, k2, "resolved spark family should derive a stable compat cache key")
}

func TestDeriveOMPResponsesPromptCacheKeyStableAcrossTurns(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Request.Header.Set("User-Agent", "omp/18.2.6")

	first := []byte(`{"model":"gpt-6-astra","instructions":"Work carefully.","tools":[{"type":"function","name":"shell"}],"input":[{"role":"user","content":[{"type":"input_text","text":"Inspect the repository"}]}]}`)
	second := []byte(`{"model":"gpt-6-astra","instructions":"Work carefully.","tools":[{"type":"function","name":"shell"}],"input":[{"role":"user","content":[{"type":"input_text","text":"Inspect the repository"}]},{"role":"assistant","content":[{"type":"output_text","text":"Done"}]},{"role":"user","content":[{"type":"input_text","text":"Run the tests"}]}]}`)

	firstKey := deriveOMPResponsesPromptCacheKey(ctx, first, "gpt-6-astra")
	secondKey := deriveOMPResponsesPromptCacheKey(ctx, second, "gpt-6-astra")
	require.NotEmpty(t, firstKey)
	require.Equal(t, firstKey, secondKey)
	require.True(t, strings.HasPrefix(firstKey, ompResponsesPromptCacheKeyPrefix))
}

func TestDeriveOMPResponsesPromptCacheKeyUsesBodySession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Request.Header.Set("User-Agent", "omp/18.2.5")

	first := []byte(`{"model":"gpt-5.6-sol","client_metadata":{"session_id":"omp-session"},"input":"first"}`)
	second := []byte(`{"model":"gpt-5.6-sol","client_metadata":{"session_id":"omp-session"},"input":"different payload"}`)
	firstKey := deriveOMPResponsesPromptCacheKey(ctx, first, "gpt-5.6-sol")
	require.NotEmpty(t, firstKey)
	require.Equal(t, firstKey, deriveOMPResponsesPromptCacheKey(ctx, second, "gpt-5.6-sol"))
}

func TestDeriveOMPResponsesPromptCacheKeyPreservesExplicitAndOtherClients(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-6-astra","input":"hello"}`)

	other, _ := gin.CreateTestContext(httptest.NewRecorder())
	other.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	other.Request.Header.Set("User-Agent", "pi/0.50.0")
	require.Empty(t, deriveOMPResponsesPromptCacheKey(other, body, "gpt-6-astra"))

	omp, _ := gin.CreateTestContext(httptest.NewRecorder())
	omp.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	omp.Request.Header.Set("User-Agent", "omp/18.2.6")
	require.Empty(t, deriveOMPResponsesPromptCacheKey(omp, []byte(`{"model":"gpt-6-astra","prompt_cache_key":"client-key","input":"hello"}`), "gpt-6-astra"))
}

func TestDeriveAnthropicCompatPromptCacheKey_StableAcrossLaterTurns(t *testing.T) {
	base := &apicompat.AnthropicRequest{
		Model:  "claude-sonnet-4-5",
		System: mustRawJSON(t, `"You are helpful."`),
		Messages: []apicompat.AnthropicMessage{
			{Role: "user", Content: mustRawJSON(t, `"Open repo"`)},
		},
	}
	extended := &apicompat.AnthropicRequest{
		Model:  "claude-sonnet-4-5",
		System: mustRawJSON(t, `"You are helpful."`),
		Messages: []apicompat.AnthropicMessage{
			{Role: "user", Content: mustRawJSON(t, `"Open repo"`)},
			{Role: "assistant", Content: mustRawJSON(t, `"Opened."`)},
			{Role: "user", Content: mustRawJSON(t, `"Run tests"`)},
		},
	}

	k1 := deriveAnthropicCompatPromptCacheKey(base, "gpt-5.3-codex")
	k2 := deriveAnthropicCompatPromptCacheKey(extended, "gpt-5.3-codex")
	require.NotEmpty(t, k1)
	require.Equal(t, k1, k2, "cache key should stay stable as later Claude Code turns append history")
}

func TestDeriveAnthropicCompatPromptCacheKey_UsesCacheControlAnchors(t *testing.T) {
	base := &apicompat.AnthropicRequest{
		Model: "claude-sonnet-4-5",
		System: mustRawJSON(t, `[
			{"type":"text","text":"project instructions","cache_control":{"type":"ephemeral"}}
		]`),
		Messages: []apicompat.AnthropicMessage{
			{Role: "user", Content: mustRawJSON(t, `[
				{"type":"text","text":"repo anchor","cache_control":{"type":"ephemeral"}}
			]`)},
		},
	}
	extended := &apicompat.AnthropicRequest{
		Model:  base.Model,
		System: base.System,
		Messages: []apicompat.AnthropicMessage{
			base.Messages[0],
			{Role: "assistant", Content: mustRawJSON(t, `[{"type":"text","text":"Opened."}]`)},
			{Role: "user", Content: mustRawJSON(t, `[{"type":"text","text":"Run tests"}]`)},
		},
	}

	k1 := deriveAnthropicCompatPromptCacheKey(base, "gpt-5.4")
	k2 := deriveAnthropicCompatPromptCacheKey(extended, "gpt-5.4")
	require.NotEmpty(t, k1)
	require.Equal(t, k1, k2)
	require.True(t, strings.HasPrefix(k1, "anthropic-cache-"))
	require.False(t, strings.HasPrefix(k1, compatPromptCacheKeyPrefix))
}
