package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	openaipkg "github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestApplyOpenAIOAuthHTTPAllowlist_PreservesAllowedFieldsAndForcesStreamStore(t *testing.T) {
	body := []byte(`{
		"model":"gpt-5.4",
		"input":[{"type":"message","role":"user","content":"hello"}],
		"instructions":"be helpful",
		"tools":[{"type":"function","name":"big","parameters":{"const":9007199254740993}}],
		"tool_choice":"auto",
		"parallel_tool_calls":true,
		"reasoning":{"effort":"medium"},
		"store":true,
		"stream":false,
		"include":["reasoning.encrypted_content"],
		"service_tier":"priority",
		"prompt_cache_key":"cache_abc",
		"text":{"verbosity":"low"},
		"client_metadata":{"trace_id":9007199254740995},
		"max_output_tokens":1024,
		"temperature":0.5,
		"top_p":0.9,
		"metadata":{"drop":true},
		"user":"drop",
		"stream_options":{"include_usage":true},
		"unknown_field":"drop"
	}`)

	normalized, changed, err := applyOpenAIOAuthHTTPAllowlist(body)
	require.NoError(t, err)
	require.True(t, changed)

	require.Equal(t, "gpt-5.4", gjson.GetBytes(normalized, "model").String())
	require.Equal(t, "hello", gjson.GetBytes(normalized, "input.0.content").String())
	require.Equal(t, "be helpful", gjson.GetBytes(normalized, "instructions").String())
	require.Equal(t, "big", gjson.GetBytes(normalized, "tools.0.name").String())
	require.Equal(t, "9007199254740993", gjson.GetBytes(normalized, "tools.0.parameters.const").Raw)
	require.Equal(t, "auto", gjson.GetBytes(normalized, "tool_choice").String())
	require.True(t, gjson.GetBytes(normalized, "parallel_tool_calls").Bool())
	require.Equal(t, "medium", gjson.GetBytes(normalized, "reasoning.effort").String())
	require.False(t, gjson.GetBytes(normalized, "store").Bool())
	require.True(t, gjson.GetBytes(normalized, "stream").Bool())
	require.Equal(t, "reasoning.encrypted_content", gjson.GetBytes(normalized, "include.0").String())
	require.Equal(t, "priority", gjson.GetBytes(normalized, "service_tier").String())
	require.Equal(t, "cache_abc", gjson.GetBytes(normalized, "prompt_cache_key").String())
	require.Equal(t, "low", gjson.GetBytes(normalized, "text.verbosity").String())
	require.Equal(t, "9007199254740995", gjson.GetBytes(normalized, "client_metadata.trace_id").Raw)

	for _, field := range []string{"max_output_tokens", "temperature", "top_p", "metadata", "user", "stream_options", "unknown_field"} {
		require.False(t, gjson.GetBytes(normalized, field).Exists(), "%s should be removed by the OAuth HTTP terminal allowlist", field)
	}
}

func TestApplyOpenAIOAuthHTTPAllowlist_AddsMissingCodexStreamStore(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","input":"hello"}`)

	normalized, changed, err := applyOpenAIOAuthHTTPAllowlist(body)
	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, gjson.GetBytes(normalized, "store").Bool())
	require.True(t, gjson.GetBytes(normalized, "stream").Bool())
}

func TestApplyOpenAIOAuthHTTPAllowlist_RejectsMalformedJSON(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","input":"hello"} trailing`)

	normalized, changed, err := applyOpenAIOAuthHTTPAllowlist(body)
	require.Error(t, err)
	require.False(t, changed)
	require.Equal(t, body, normalized)
}

func TestApplyOpenAIOAuthCompactAllowlist_PreservesAllowedFieldsRawAndDropsNonCompactionFields(t *testing.T) {
	body := []byte(`{
		"model":"gpt-5.4",
		"input":[{"type":"message","role":"user","content":"compact me"}],
		"instructions":"compact-test",
		"tools":[{"type":"function","name":"big","parameters":{"const":9007199254740993}}],
		"parallel_tool_calls":true,
		"reasoning":{"effort":"high"},
		"service_tier":"priority",
		"prompt_cache_key":"cache_abc",
		"text":{"trace":9007199254740995},
		"store":true,
		"stream":true,
		"include":["reasoning.encrypted_content"],
		"tool_choice":"auto",
		"client_metadata":{"drop":true},
		"metadata":{"drop":true},
		"user":"drop",
		"max_output_tokens":1024,
		"unknown_field":"drop"
	}`)

	normalized, changed, err := applyOpenAIOAuthCompactAllowlist(body)
	require.NoError(t, err)
	require.True(t, changed)

	require.Equal(t, "gpt-5.4", gjson.GetBytes(normalized, "model").String())
	require.Equal(t, "compact me", gjson.GetBytes(normalized, "input.0.content").String())
	require.Equal(t, "compact-test", gjson.GetBytes(normalized, "instructions").String())
	require.Equal(t, "9007199254740993", gjson.GetBytes(normalized, "tools.0.parameters.const").Raw)
	require.True(t, gjson.GetBytes(normalized, "parallel_tool_calls").Bool())
	require.Equal(t, "high", gjson.GetBytes(normalized, "reasoning.effort").String())
	require.Equal(t, "priority", gjson.GetBytes(normalized, "service_tier").String())
	require.Equal(t, "cache_abc", gjson.GetBytes(normalized, "prompt_cache_key").String())
	require.Equal(t, "9007199254740995", gjson.GetBytes(normalized, "text.trace").Raw)

	for _, field := range []string{"store", "stream", "include", "tool_choice", "client_metadata", "metadata", "user", "max_output_tokens", "unknown_field"} {
		require.False(t, gjson.GetBytes(normalized, field).Exists(), "%s should be removed by the OAuth compact terminal allowlist", field)
	}
}

func TestApplyOpenAIOAuthCompactAllowlist_RejectsMalformedJSON(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","input":"hello"`)

	normalized, changed, err := applyOpenAIOAuthCompactAllowlist(body)
	require.Error(t, err)
	require.False(t, changed)
	require.Equal(t, body, normalized)
}

func TestOpenAIGatewayService_ForwardOAuthHTTPAllowlistPreservesRawNumbersAfterCodexTransform(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{
		"model":"gpt-5.4",
		"stream":false,
		"store":true,
		"instructions":"preserve raw numeric JSON",
		"input":[{"type":"message","role":"user","content":"hello"}],
		"tools":[{"type":"function","name":"big","parameters":{"type":"object","properties":{"id":{"const":9007199254740993}}}}],
		"client_metadata":{"trace_id":9007199254740995},
		"unknown_field":"drop"
	}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid-raw-number"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"resp_raw","status":"completed","model":"gpt-5.4","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`)),
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	account := httptestOpenAIOAuthBodyPolicyAccount()

	_, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.Equal(t, "9007199254740993", gjson.GetBytes(upstream.lastBody, "tools.0.parameters.properties.id.const").Raw)
	require.Equal(t, "9007199254740995", gjson.GetBytes(upstream.lastBody, "client_metadata.trace_id").Raw)
	require.False(t, gjson.GetBytes(upstream.lastBody, "unknown_field").Exists())
}

func TestOpenAIGatewayService_OAuthHTTPResponsesInstructions(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name             string
		body             []byte
		wantInstructions string
	}{
		{
			name:             "empty native responses uses model aware synthetic default",
			body:             []byte(`{"model":"gpt-5.5","stream":false,"store":true,"input":[{"type":"text","text":"hi"}]}`),
			wantInstructions: openaipkg.CodexSyntheticDefaultInstructionsForModel("gpt-5.5"),
		},
		{
			name:             "explicit native responses instructions preserved",
			body:             []byte(`{"model":"gpt-5.5","stream":false,"store":true,"instructions":"  keep these exact instructions  ","input":[{"type":"text","text":"hi"}]}`),
			wantInstructions: "  keep these exact instructions  ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(tt.body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Request.Header.Set("User-Agent", "codex_cli_rs/0.98.0")

			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid-instructions"}},
				Body:       io.NopCloser(strings.NewReader(`{"id":"resp_instructions","status":"completed","model":"gpt-5.5","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`)),
			}}
			svc := &OpenAIGatewayService{httpUpstream: upstream}

			_, err := svc.Forward(context.Background(), c, httptestOpenAIOAuthBodyPolicyAccount(), tt.body)
			require.NoError(t, err)
			require.Equal(t, tt.wantInstructions, gjson.GetBytes(upstream.lastBody, "instructions").String())
		})
	}
}

func TestOpenAIGatewayService_ForwardOAuthHTTPRejectsMalformedJSONBeforeUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.4","input":"hello"} trailing`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{}`))}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}

	_, err := svc.Forward(context.Background(), c, httptestOpenAIOAuthBodyPolicyAccount(), body)
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse request")
	require.Empty(t, upstream.requests)
}

func TestOpenAIChatCompletionsResponsesShapeBridgeBody_OAuthLeavesUnsupportedFieldsForTerminalAllowlist(t *testing.T) {
	body := []byte(`{
		"model":"gpt-5.4",
		"input":[{"type":"message","role":"user","content":"hello"}],
		"service_tier":"fast",
		"metadata":{"drop":true},
		"stream_options":{"include_usage":true},
		"prompt_cache_retention":"24h",
		"safety_identifier":"safe-user"
	}`)

	bridged, serviceTier, err := buildOpenAIChatCompletionsResponsesShapeBridgeBody(body, "gpt-5.4", httptestOpenAIOAuthBodyPolicyAccount())
	require.NoError(t, err)
	require.Equal(t, "priority", serviceTier)
	require.Equal(t, "priority", gjson.GetBytes(bridged, "service_tier").String())

	for _, field := range []string{"metadata", "stream_options", "prompt_cache_retention", "safety_identifier"} {
		require.True(t, gjson.GetBytes(bridged, field).Exists(), "%s should stay in the bridge output for terminal policy", field)
	}

	normalized, _, err := normalizeOpenAIOAuthHTTPBody(bridged, false)
	require.NoError(t, err)
	for _, field := range []string{"metadata", "stream_options", "prompt_cache_retention", "safety_identifier"} {
		require.False(t, gjson.GetBytes(normalized, field).Exists(), "%s should be removed by the terminal OAuth allowlist", field)
	}
}

func TestOpenAIChatCompletionsResponsesShapeBridgeBody_SetupTokenLeavesUnsupportedFieldsForTerminalAllowlist(t *testing.T) {
	body := []byte(`{
		"model":"gpt-5.4",
		"input":[{"type":"message","role":"user","content":"hello"}],
		"metadata":{"drop":true},
		"stream_options":{"include_usage":true},
		"prompt_cache_retention":"24h",
		"safety_identifier":"safe-user"
	}`)
	account := httptestOpenAIOAuthBodyPolicyAccount()
	account.Type = AccountTypeSetupToken

	bridged, _, err := buildOpenAIChatCompletionsResponsesShapeBridgeBody(body, "gpt-5.4", account)
	require.NoError(t, err)
	for _, field := range []string{"metadata", "stream_options", "prompt_cache_retention", "safety_identifier"} {
		require.True(t, gjson.GetBytes(bridged, field).Exists(), "%s should stay in the setup-token bridge output for terminal policy", field)
	}
}

func TestOpenAIChatCompletionsResponsesShapeBridgeBody_APIKeyKeepsExistingCursorFieldStrip(t *testing.T) {
	body := []byte(`{
		"model":"gpt-5.4",
		"input":[{"type":"message","role":"user","content":"hello"}],
		"metadata":{"drop":true},
		"stream_options":{"include_usage":true},
		"prompt_cache_retention":"24h",
		"safety_identifier":"safe-user"
	}`)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	bridged, _, err := buildOpenAIChatCompletionsResponsesShapeBridgeBody(body, "gpt-5.4", account)
	require.NoError(t, err)
	for _, field := range []string{"metadata", "stream_options", "prompt_cache_retention", "safety_identifier"} {
		require.False(t, gjson.GetBytes(bridged, field).Exists(), "%s should keep the existing APIKey responses-shape bridge behavior", field)
	}
}

func TestOpenAIGatewayService_ForwardChatCompletionsOAuthResponsesShapePreservesRawNumbersUntilTerminalAllowlist(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{
		"model":"gpt-5.4",
		"stream":false,
		"instructions":"bridge raw numeric JSON",
		"input":[{"type":"message","role":"user","content":"hello"}],
		"tools":[{"type":"function","name":"big","parameters":{"type":"object","properties":{"id":{"const":9007199254740993}}}}],
		"client_metadata":{"trace_id":9007199254740995},
		"metadata":{"drop":true},
		"stream_options":{"include_usage":true}
	}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid-chat-bridge-raw-number"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"type":"response.completed","response":{"id":"resp_chat_bridge","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":1,"output_tokens":1}}}`,
			"",
			"data: [DONE]",
			"",
		}, "\n"))),
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}

	_, err := svc.ForwardAsChatCompletions(context.Background(), c, httptestOpenAIOAuthBodyPolicyAccount(), body, "", "gpt-5.4")
	require.NoError(t, err)
	require.Equal(t, "9007199254740993", gjson.GetBytes(upstream.lastBody, "tools.0.parameters.properties.id.const").Raw)
	require.Equal(t, "9007199254740995", gjson.GetBytes(upstream.lastBody, "client_metadata.trace_id").Raw)
	for _, field := range []string{"metadata", "stream_options"} {
		require.False(t, gjson.GetBytes(upstream.lastBody, field).Exists(), "%s should be removed by terminal OAuth allowlist", field)
	}
}

func TestOpenAIGatewayService_ForwardChatCompletionsSetupTokenResponsesShapeUsesOAuthAdapterPolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{
		"model":"gpt-5.4",
		"stream":false,
		"instructions":"bridge raw numeric JSON",
		"input":[{"type":"message","role":"user","content":"hello"}],
		"tools":[{"type":"function","name":"big","parameters":{"type":"object","properties":{"id":{"const":9007199254740993}}}}],
		"client_metadata":{"trace_id":9007199254740995},
		"metadata":{"drop":true},
		"stream_options":{"include_usage":true}
	}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid-chat-bridge-setup-token"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"type":"response.completed","response":{"id":"resp_chat_bridge_setup_token","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":1,"output_tokens":1}}}`,
			"",
			"data: [DONE]",
			"",
		}, "\n"))),
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	account := httptestOpenAIOAuthBodyPolicyAccount()
	account.Type = AccountTypeSetupToken

	_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "gpt-5.4")
	require.NoError(t, err)
	require.Equal(t, chatgptCodexURL, upstream.lastReq.URL.String())
	require.Equal(t, "chatgpt-acc", upstream.lastReq.Header.Get("chatgpt-account-id"))
	require.Equal(t, "9007199254740993", gjson.GetBytes(upstream.lastBody, "tools.0.parameters.properties.id.const").Raw)
	require.Equal(t, "9007199254740995", gjson.GetBytes(upstream.lastBody, "client_metadata.trace_id").Raw)
	for _, field := range []string{"metadata", "stream_options"} {
		require.False(t, gjson.GetBytes(upstream.lastBody, field).Exists(), "%s should be removed by terminal OAuth-like allowlist", field)
	}
}

func TestOpenAIGatewayService_ForwardChatCompletionsSetupTokenNormalPathUsesOAuthTransform(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{
		"model":"gpt-5.4-high",
		"stream":false,
		"messages":[{"role":"user","content":"hello"}],
		"tools":[{"type":"function","function":{"name":"big","parameters":{"type":"object","properties":{"id":{"const":9007199254740993}}}}}]
	}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid-chat-normal-setup-token"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"type":"response.completed","response":{"id":"resp_chat_normal_setup_token","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":1,"output_tokens":1}}}`,
			"",
			"data: [DONE]",
			"",
		}, "\n"))),
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	account := httptestOpenAIOAuthBodyPolicyAccount()
	account.Type = AccountTypeSetupToken

	_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "gpt-5.4")
	require.NoError(t, err)
	require.Equal(t, chatgptCodexURL, upstream.lastReq.URL.String())
	require.Equal(t, "gpt-5.4", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, "9007199254740993", gjson.GetBytes(upstream.lastBody, "tools.0.parameters.properties.id.const").Raw)
	require.NotEmpty(t, upstream.lastReq.Header.Get("session_id"))
	require.NotEmpty(t, gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String())
	require.Equal(t, upstream.lastReq.Header.Get(openAICodexThreadIDHeader), gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "max_output_tokens").Exists())
}

func TestOpenAIGatewayService_ForwardChatCompletionsOAuthKeepsIsolatedAdapterSessionID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.4-high","stream":false,"messages":[{"role":"user","content":"hello"}]}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Set("api_key", &APIKey{ID: 77})
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid-chat-oauth-session"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"type":"response.completed","response":{"id":"resp_chat_oauth_session","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":1,"output_tokens":1}}}`,
			"",
			"data: [DONE]",
			"",
		}, "\n"))),
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	account := httptestOpenAIOAuthBodyPolicyAccount()

	_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "shared-cache-key", "gpt-5.4")

	require.NoError(t, err)
	isolatedSessionID := isolateOpenAICodexOAuthSessionID(77, "shared-cache-key", "session")
	isolatedThreadID := isolateOpenAICodexOAuthSessionID(77, "shared-cache-key", "thread")
	require.Equal(t, isolatedSessionID, upstream.lastReq.Header.Get("session_id"))
	require.Equal(t, isolatedSessionID, upstream.lastReq.Header.Get(openAICodexSessionIDHeader))
	require.Equal(t, isolatedThreadID, gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String())
}

func TestOpenAIGatewayService_ForwardMessagesSetupTokenUsesOAuthAdapterPolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{
		"model":"gpt-5.4",
		"max_tokens":32,
		"stream":false,
		"messages":[{"role":"user","content":"hello"}],
		"tools":[{"name":"big","description":"big id","input_schema":{"type":"object","properties":{"id":{"const":9007199254740993}}}}]
	}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":       []string{"text/event-stream"},
			"x-request-id":       []string{"rid-messages-setup-token"},
			"x-codex-turn-state": []string{"turn-state"},
		},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"type":"response.completed","response":{"id":"resp_messages_setup_token","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":1,"output_tokens":1}}}`,
			"",
			"data: [DONE]",
			"",
		}, "\n"))),
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	account := httptestOpenAIOAuthBodyPolicyAccount()
	account.Type = AccountTypeSetupToken

	_, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.4")
	require.NoError(t, err)
	require.Equal(t, chatgptCodexURL, upstream.lastReq.URL.String())
	require.Empty(t, upstream.lastReq.Header.Get("OpenAI-Beta"))
	require.Equal(t, codexOfficialOriginator, upstream.lastReq.Header.Get("originator"))
	require.Empty(t, upstream.lastReq.Header.Get("conversation_id"))
	require.NotEmpty(t, upstream.lastReq.Header.Get("session_id"))
	require.Equal(t, "9007199254740993", gjson.GetBytes(upstream.lastBody, "tools.0.parameters.properties.id.const").Raw)
	require.Contains(t, string(upstream.lastBody), openAICompatClaudeCodeTodoGuardMarker)
	require.False(t, gjson.GetBytes(upstream.lastBody, "max_output_tokens").Exists())
}

func TestOpenAIGatewayService_ForwardMessagesOAuthPreservesRawToolSchemaNumbersUntilTerminalAllowlist(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{
		"model":"gpt-5.4",
		"max_tokens":32,
		"stream":false,
		"messages":[{"role":"user","content":"hello"}],
		"tools":[{"name":"big","description":"big id","input_schema":{"type":"object","properties":{"id":{"const":9007199254740993}}}}]
	}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid-messages-bridge-raw-number"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"type":"response.completed","response":{"id":"resp_messages_bridge","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":1,"output_tokens":1}}}`,
			"",
			"data: [DONE]",
			"",
		}, "\n"))),
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}

	_, err := svc.ForwardAsAnthropic(context.Background(), c, httptestOpenAIOAuthBodyPolicyAccount(), body, "", "gpt-5.4")
	require.NoError(t, err)
	require.Equal(t, "9007199254740993", gjson.GetBytes(upstream.lastBody, "tools.0.parameters.properties.id.const").Raw)
	require.False(t, gjson.GetBytes(upstream.lastBody, "max_output_tokens").Exists())
}

func TestOpenAIGatewayService_ForwardOAuthCompactTreatsAllowlistedBodyAsNonStreaming(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.4","stream":true,"instructions":"compact","input":[{"type":"message","role":"user","content":"compact me"}]}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid-compact-nonstream"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"resp_compact","status":"completed","model":"gpt-5.4","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`)),
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}

	result, err := svc.Forward(context.Background(), c, httptestOpenAIOAuthBodyPolicyAccount(), body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.Stream)
	require.False(t, gjson.GetBytes(upstream.lastBody, "stream").Exists())
}

func TestOpenAIGatewayService_ForwardOAuthCompactRejectsMalformedJSONBeforeUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tt := range []struct {
		name string
		body []byte
	}{
		{name: "trailing tokens after object", body: []byte(`{"model":"gpt-5.4","input":[]} trailing`)},
		{name: "empty body", body: []byte(``)},
		{name: "whitespace body", body: []byte(`   `)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", bytes.NewReader(tt.body))
			c.Request.Header.Set("Content-Type", "application/json")

			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{}`))}}
			svc := &OpenAIGatewayService{httpUpstream: upstream}

			_, err := svc.Forward(context.Background(), c, httptestOpenAIOAuthBodyPolicyAccount(), tt.body)
			require.Error(t, err)
			require.Contains(t, err.Error(), "parse request")
			require.Empty(t, upstream.requests)
		})
	}
}

func TestNormalizeOpenAIOAuthHTTPUpstreamRequestBody_DoesNotTouchAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.4","max_output_tokens":1024,"temperature":0.5,"unknown_field":"keep"}`)
	req := httptestNewOAuthBodyPolicyRequest(body)
	rec := httptestNewOAuthBodyPolicyRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{"openai_passthrough": true},
	}

	normalized, err := normalizeOpenAIOAuthHTTPUpstreamRequestBody(req, c, account, body)
	require.NoError(t, err)
	require.Equal(t, body, normalized)

	actualBody, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	require.Equal(t, string(body), string(actualBody))
}

func TestOpenAIGatewayService_ForwardOAuthHTTPPersistsCodexUsageSnapshotOnlyForExactOAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.4","input":"hello","stream":false}`)
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("x-request-id", "rid-oauth-codex-usage")
	headers.Set(openAICodexPrimaryUsedPercentHeader, "42")
	headers.Set(openAICodexPrimaryResetSecondsHeader, "3600")
	headers.Set(openAICodexPrimaryWindowMinutesHeader, "10080")
	headers.Set(openAICodexSecondUsedPercentHeader, "7")
	headers.Set(openAICodexSecondResetSecondsHeader, "300")
	headers.Set(openAICodexSecondWindowMinutesHeader, "300")
	headers.Set(openAICodexPrimaryOverSecondHeader, "12")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     headers,
		Body:       io.NopCloser(strings.NewReader(`{"id":"resp_codex_usage","status":"completed","model":"gpt-5.4","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`)),
	}}
	repo := &snapshotUpdateAccountRepo{updateExtraCalls: make(chan map[string]any, 2)}
	svc := &OpenAIGatewayService{
		httpUpstream:          upstream,
		accountRepo:           repo,
		codexSnapshotThrottle: newAccountWriteThrottle(0),
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	_, err := svc.Forward(context.Background(), c, httptestOpenAIOAuthBodyPolicyAccount(), body)
	require.NoError(t, err)

	var snapshotUpdates map[string]any
	deadline := time.After(time.Second)
	for snapshotUpdates == nil {
		select {
		case updates := <-repo.updateExtraCalls:
			if _, ok := updates["codex_usage_updated_at"]; ok {
				snapshotUpdates = updates
			}
		case <-deadline:
			t.Fatal("expected exact OAuth responses path to persist Codex usage snapshot")
		}
	}
	require.Equal(t, float64(42), snapshotUpdates["codex_primary_used_percent"])
}

func TestOpenAIGatewayService_ForwardSetupTokenHTTPDoesNotPersistCodexUsageSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.4","input":"hello","stream":false}`)
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("x-request-id", "rid-setup-token-codex-usage")
	headers.Set(openAICodexPrimaryUsedPercentHeader, "42")
	headers.Set(openAICodexPrimaryResetSecondsHeader, "3600")
	headers.Set(openAICodexPrimaryWindowMinutesHeader, "10080")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     headers,
		Body:       io.NopCloser(strings.NewReader(`{"id":"resp_setup_codex_usage","status":"completed","model":"gpt-5.4","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`)),
	}}
	repo := &snapshotUpdateAccountRepo{updateExtraCalls: make(chan map[string]any, 1)}
	svc := &OpenAIGatewayService{
		httpUpstream:          upstream,
		accountRepo:           repo,
		codexSnapshotThrottle: newAccountWriteThrottle(0),
	}
	account := httptestOpenAIOAuthBodyPolicyAccount()
	account.Type = AccountTypeSetupToken

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	_, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)

	select {
	case updates := <-repo.updateExtraCalls:
		require.Contains(t, updates, OpenAICodexFingerprintExtraKey)
		require.NotContains(t, updates, "codex_usage_updated_at")
	case <-time.After(time.Second):
		t.Fatal("expected setup-token fingerprint persistence")
	}
	select {
	case updates := <-repo.updateExtraCalls:
		t.Fatalf("setup-token must not persist full OAuth Codex usage snapshot, got updates: %v", updates)
	case <-time.After(100 * time.Millisecond):
	}
}

func httptestOpenAIOAuthBodyPolicyAccount() *Account {
	return &Account{
		ID:          123,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
		Status:      StatusActive,
		Schedulable: true,
	}
}

func httptestNewOAuthBodyPolicyRequest(body []byte) *http.Request {
	return httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(string(body)))
}

func httptestNewOAuthBodyPolicyRecorder() *httptest.ResponseRecorder {
	return httptest.NewRecorder()
}
