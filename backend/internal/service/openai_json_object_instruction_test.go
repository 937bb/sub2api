package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestEnsureOpenAIJSONObjectInputInstruction(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantChanged bool
	}{
		{
			name:        "missing instruction is injected",
			body:        `{"text":{"format":{"type":"json_object"}},"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"Return one object"}]}]}`,
			wantChanged: true,
		},
		{
			name:        "uppercase JSON instruction is preserved",
			body:        `{"text":{"format":{"type":"json_object"}},"input":[{"type":"message","role":"user","content":"Return JSON"}]}`,
			wantChanged: false,
		},
		{
			name:        "lowercase json instruction is preserved",
			body:        `{"text":{"format":{"type":"json_object"}},"input":[{"role":"developer","content":[{"type":"input_text","text":"respond with json"}]}]}`,
			wantChanged: false,
		},
		{
			name:        "json schema is unchanged",
			body:        `{"text":{"format":{"type":"json_schema","schema":{"type":"object"}}},"input":"hello"}`,
			wantChanged: false,
		},
		{
			name:        "plain text format is unchanged",
			body:        `{"text":{"format":{"type":"text"}},"input":"hello"}`,
			wantChanged: false,
		},
		{
			name:        "tool argument does not satisfy message requirement",
			body:        `{"text":{"format":{"type":"json_object"}},"input":[{"type":"function_call","call_id":"call_1","name":"probe","arguments":"{\"format\":\"json\"}"},{"type":"message","role":"user","content":"hello"}]}`,
			wantChanged: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := []byte(tt.body)
			normalized, changed, err := ensureOpenAIJSONObjectInputInstruction(original)
			require.NoError(t, err)
			require.Equal(t, tt.wantChanged, changed)
			if !tt.wantChanged {
				require.Equal(t, original, normalized)
				return
			}
			require.Equal(t, "message", gjson.GetBytes(normalized, "input.0.type").String())
			require.Equal(t, "developer", gjson.GetBytes(normalized, "input.0.role").String())
			require.Equal(t, "Return the response as JSON.", gjson.GetBytes(normalized, "input.0.content.0.text").String())

			again, changedAgain, err := ensureOpenAIJSONObjectInputInstruction(normalized)
			require.NoError(t, err)
			require.False(t, changedAgain)
			require.Equal(t, normalized, again)
		})
	}
}

func TestEnsureOpenAIJSONObjectInputInstructionPreservesStringInput(t *testing.T) {
	body := []byte(`{"model":"gpt-6.1-sol","text":{"format":{"type":"json_object"}},"input":"describe this"}`)

	normalized, changed, err := ensureOpenAIJSONObjectInputInstruction(body)
	require.NoError(t, err)
	require.True(t, changed)
	require.Len(t, gjson.GetBytes(normalized, "input").Array(), 2)
	require.Equal(t, "developer", gjson.GetBytes(normalized, "input.0.role").String())
	require.Equal(t, "user", gjson.GetBytes(normalized, "input.1.role").String())
	require.Equal(t, "describe this", gjson.GetBytes(normalized, "input.1.content.0.text").String())
}

func TestEnsureOpenAIJSONObjectInputInstructionPreservesOpaqueItemsAndNumbers(t *testing.T) {
	body := []byte(`{"request_id":900719925474099312345,"text":{"format":{"type":"json_object"}},"input":[{"type":"message","id":"msg_1","role":"user","content":[{"type":"input_image","image_url":"data:image/png;base64,AAAA"},{"type":"input_text","text":"describe it"}]},{"type":"function_call","id":"fc_1","call_id":"call_1","name":"probe","arguments":"{}"}]}`)

	normalized, changed, err := ensureOpenAIJSONObjectInputInstruction(body)
	require.NoError(t, err)
	require.True(t, changed)
	require.Contains(t, string(normalized), `"request_id":900719925474099312345`)
	require.Equal(t, "msg_1", gjson.GetBytes(normalized, "input.1.id").String())
	require.Equal(t, "data:image/png;base64,AAAA", gjson.GetBytes(normalized, "input.1.content.0.image_url").String())
	require.Equal(t, "fc_1", gjson.GetBytes(normalized, "input.2.id").String())
	require.Equal(t, "{}", gjson.GetBytes(normalized, "input.2.arguments").String())
}

func TestNormalizeOpenAIResponsesWebSocketCompatibilityBodyRepairsJSONObjectInstruction(t *testing.T) {
	body := []byte(`{"type":"response.create","model":"gpt-6.1-sol","text":{"format":{"type":"json_object"}},"input":[{"type":"message","role":"user","content":"hello"}]}`)

	normalized, changed, err := normalizeOpenAIResponsesWebSocketCompatibilityBody(body, &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
	}, false)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "developer", gjson.GetBytes(normalized, "input.0.role").String())
	require.Equal(t, "hello", gjson.GetBytes(normalized, "input.1.content").String())
}

func TestOpenAIGatewayServiceRepairsJSONObjectInstructionOnHTTPPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, tt := range []struct {
		name        string
		passthrough bool
	}{
		{name: "normal OAuth", passthrough: false},
		{name: "OAuth passthrough", passthrough: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body: io.NopCloser(strings.NewReader(
					"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_json\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":1,\"total_tokens\":2}}}\n\ndata: [DONE]\n\n",
				)),
			}}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			account := &Account{
				ID: 701, Name: "oauth-json-object", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1,
				Credentials: map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-account"},
				Extra: map[string]any{
					"openai_passthrough":                        tt.passthrough,
					"openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModeOff,
				},
				Status: StatusActive, Schedulable: true, RateMultiplier: f64p(1),
			}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			c.Request.Header.Set("User-Agent", "codex_cli_rs/0.163.0")
			SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
			body := []byte(`{"model":"gpt-6.1-sol","stream":true,"instructions":"Answer briefly.","text":{"format":{"type":"json_object"}},"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"give one object"}]}]}`)

			result, err := svc.Forward(context.Background(), c, account, body)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, "developer", gjson.GetBytes(upstream.lastBody, "input.0.role").String())
			require.Equal(t, "Return the response as JSON.", gjson.GetBytes(upstream.lastBody, "input.0.content.0.text").String())
			require.Equal(t, "user", gjson.GetBytes(upstream.lastBody, "input.1.role").String())
			require.Equal(t, "give one object", gjson.GetBytes(upstream.lastBody, "input.1.content.0.text").String())
		})
	}
}

func TestOpenAIGatewayServiceStrictAPIKeyPassthroughDoesNotRepairJSONObjectInstruction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalBody := []byte("{\n  \"model\": \"gpt-6.1-sol\",\n  \"stream\": false,\n  \"text\": {\"format\": {\"type\": \"json_object\"}},\n  \"input\": [{\"type\":\"message\",\"role\":\"user\",\"content\":\"give one object\"}]\n}")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(
			`{"id":"resp_json","model":"gpt-6.1-sol","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`,
		)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID: 702, Name: "official-apikey", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials:    map[string]any{"api_key": "sk-test", "base_url": "https://api.openai.com/v1"},
		Extra:          map[string]any{"openai_passthrough": true},
		Status:         StatusActive,
		Schedulable:    true,
		RateMultiplier: f64p(1),
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(originalBody))
	c.Request.Header.Set("Content-Type", "application/json")

	result, err := svc.Forward(context.Background(), c, account, originalBody)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, originalBody, upstream.lastBody)
}
