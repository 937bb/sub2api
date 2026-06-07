package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestNormalizeOpenAIPassthroughOAuthBody_AllowlistPreservesAllCodexFields(t *testing.T) {
	// 发送 Codex ResponsesApiRequest 全部 14 字段 + 若干不允许字段；
	// 验证 allowlist 保留 14 字段 + 强制 store/stream + 丢弃其余。
	body := []byte(`{
		"model": "gpt-5.4",
		"input": [{"type": "message", "role": "user", "content": "hello"}],
		"instructions": "be helpful",
		"tools": [{"type": "function", "name": "search"}],
		"tool_choice": "auto",
		"parallel_tool_calls": true,
		"reasoning": {"effort": "medium"},
		"store": true,
		"stream": false,
		"include": ["reasoning.encrypted_content"],
		"service_tier": "priority",
		"prompt_cache_key": "cache_abc",
		"text": {"verbosity": "low"},
		"client_metadata": {"x-codex-installation-id": "inst-123"},
		"user": "should-be-dropped",
		"metadata": {"should": "be-dropped"},
		"temperature": 0.5,
		"top_p": 0.9,
		"max_output_tokens": 1024,
		"previous_response_id": "resp_should_drop",
		"type": "response.create",
		"generate": false,
		"background": true,
		"truncation": "auto"
	}`)

	normalized, changed, err := normalizeOpenAIPassthroughOAuthBody(body, false)
	require.NoError(t, err)
	require.True(t, changed)

	// 14 字段保留。
	require.Equal(t, "gpt-5.4", gjson.GetBytes(normalized, "model").String())
	require.Equal(t, "hello", gjson.GetBytes(normalized, "input.0.content").String())
	require.Equal(t, "be helpful", gjson.GetBytes(normalized, "instructions").String())
	require.Equal(t, "search", gjson.GetBytes(normalized, "tools.0.name").String())
	require.Equal(t, "auto", gjson.GetBytes(normalized, "tool_choice").String())
	require.True(t, gjson.GetBytes(normalized, "parallel_tool_calls").Bool())
	require.Equal(t, "medium", gjson.GetBytes(normalized, "reasoning.effort").String())
	require.True(t, gjson.GetBytes(normalized, "stream").Bool())
	require.False(t, gjson.GetBytes(normalized, "store").Bool())
	require.Equal(t, "reasoning.encrypted_content", gjson.GetBytes(normalized, "include.0").String())
	require.Equal(t, "priority", gjson.GetBytes(normalized, "service_tier").String())
	require.Equal(t, "cache_abc", gjson.GetBytes(normalized, "prompt_cache_key").String())
	require.Equal(t, "low", gjson.GetBytes(normalized, "text.verbosity").String())
	require.Equal(t, "inst-123", gjson.GetBytes(normalized, "client_metadata.x-codex-installation-id").String())

	// 不在 allowlist 的全部丢弃。
	for _, field := range []string{
		"user", "metadata", "temperature", "top_p", "max_output_tokens",
		"previous_response_id", "type", "generate", "background", "truncation",
	} {
		require.False(t, gjson.GetBytes(normalized, field).Exists(), "%s should be dropped", field)
	}
}

func TestNormalizeOpenAIPassthroughOAuthBody_ClientMetadataPreserved(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","input":"hi","client_metadata":{"x-codex-installation-id":"inst-abc","x-codex-window-id":"win-1"}}`)

	normalized, changed, err := normalizeOpenAIPassthroughOAuthBody(body, false)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "inst-abc", gjson.GetBytes(normalized, "client_metadata.x-codex-installation-id").String())
	require.Equal(t, "win-1", gjson.GetBytes(normalized, "client_metadata.x-codex-window-id").String())
	require.True(t, gjson.GetBytes(normalized, "stream").Bool())
	require.False(t, gjson.GetBytes(normalized, "store").Bool())
}

func TestNormalizeOpenAIPassthroughOAuthBody_RemovesUnsupportedUser(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","input":"hello","user":"user_123","metadata":{"user_id":"user_123"},"prompt_cache_retention":"24h","safety_identifier":"sid","stream_options":{"include_usage":true}}`)

	normalized, changed, err := normalizeOpenAIPassthroughOAuthBody(body, false)
	require.NoError(t, err)
	require.True(t, changed)
	for _, field := range []string{"user", "metadata", "prompt_cache_retention", "safety_identifier", "stream_options"} {
		require.False(t, gjson.GetBytes(normalized, field).Exists(), "%s should be stripped", field)
	}
	require.True(t, gjson.GetBytes(normalized, "stream").Bool())
	require.False(t, gjson.GetBytes(normalized, "store").Bool())
}

func TestNormalizeOpenAIPassthroughOAuthBody_CompactRemovesUnsupportedUser(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","input":"hello","user":"user_123","metadata":{"user_id":"user_123"},"stream":true,"store":true}`)

	normalized, changed, err := normalizeOpenAIPassthroughOAuthBody(body, true)
	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, gjson.GetBytes(normalized, "user").Exists())
	require.False(t, gjson.GetBytes(normalized, "metadata").Exists())
	require.False(t, gjson.GetBytes(normalized, "stream").Exists())
	require.False(t, gjson.GetBytes(normalized, "store").Exists())
}

func TestNormalizeOpenAIPassthroughOAuthBody_CompactRejectsMalformedJSON(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","input":"hello"`)

	normalized, changed, err := normalizeOpenAIPassthroughOAuthBody(body, true)
	require.Error(t, err)
	require.False(t, changed)
	require.Equal(t, body, normalized)
}
