package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type ompResponsesFixture struct {
	Name    string            `json:"name"`
	Headers map[string]string `json:"headers"`
	Body    json.RawMessage   `json:"body"`
}

func ompRegressionAccount(mode string, passthrough bool) *Account {
	return &Account{
		ID:          781,
		Name:        "omp-regression",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "test-token",
			"chatgpt_account_id": "omp-upstream-account",
		},
		Extra: map[string]any{
			codexFingerprintModeExtraKey: mode,
			codexFingerprintSeedExtraKey: testCodexFingerprintSeed,
			"openai_oauth_passthrough":   passthrough,
		},
		Status:      StatusActive,
		Schedulable: true,
	}
}

func ompRegressionContext(apiKeyID int64, headers map[string]string) *gin.Context {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	for key, value := range headers {
		c.Request.Header.Set(key, value)
	}
	c.Set("api_key", &APIKey{ID: apiKeyID})
	return c
}

func TestOMPRecordedResponsesKeepCacheWithoutInventingConversations(t *testing.T) {
	data, err := os.ReadFile("testdata/omp_responses_18_2_6.json")
	require.NoError(t, err)
	var fixtures []ompResponsesFixture
	require.NoError(t, json.Unmarshal(data, &fixtures))
	require.Len(t, fixtures, 4)

	for _, mode := range []string{"off", "device", "session", "full"} {
		for _, passthrough := range []bool{false, true} {
			name := mode + "/normal"
			if passthrough {
				name = mode + "/passthrough"
			}
			t.Run(name, func(t *testing.T) {
				upstream := &httpUpstreamRecorder{}
				for range fixtures {
					upstream.responses = append(upstream.responses, openAICompatSSECompletedResponse("resp_omp", "gpt-6-astra"))
				}
				svc := &OpenAIGatewayService{
					cfg:          &config.Config{},
					httpUpstream: upstream,
					cache:        &stubGatewayCache{},
				}
				account := ompRegressionAccount(mode, passthrough)
				for _, fixture := range fixtures {
					c := ompRegressionContext(1652, fixture.Headers)
					result, forwardErr := svc.Forward(context.Background(), c, account, fixture.Body)
					require.NoError(t, forwardErr, fixture.Name)
					require.NotNil(t, result, fixture.Name)
				}

				require.Len(t, upstream.bodies, len(fixtures))
				require.Len(t, upstream.requests, len(fixtures))
				for i, body := range upstream.bodies {
					inputKey := gjson.GetBytes(fixtures[i].Body, "prompt_cache_key").String()
					expectedKey := scopeCodexAccountIdentityValue(account, 1652, "prompt-cache", inputKey)
					require.Equal(t, expectedKey, gjson.GetBytes(body, "prompt_cache_key").String(), fixtures[i].Name)
					for _, field := range []string{"session_id", "thread_id", "turn_id", "x-codex-window-id"} {
						require.False(t, gjson.GetBytes(body, "client_metadata."+field).Exists(), field)
					}
					require.Equal(t, expectedKey, upstream.requests[i].Header.Get("session-id"), fixtures[i].Name)
					for _, header := range []string{"session_id", "thread-id", "conversation_id", "x-client-request-id", "x-codex-window-id"} {
						require.Empty(t, upstream.requests[i].Header.Get(header), header)
					}
				}

				require.Equal(t, upstream.requests[0].Header.Get("session-id"), upstream.requests[1].Header.Get("session-id"))
				require.Equal(t, upstream.requests[2].Header.Get("session-id"), upstream.requests[3].Header.Get("session-id"))
				require.NotEqual(t, upstream.requests[0].Header.Get("session-id"), upstream.requests[2].Header.Get("session-id"))
				for _, start := range []int{0, 2} {
					for _, field := range []string{"prompt_cache_key", "instructions", "tools", "input.0"} {
						require.Equal(t, gjson.GetBytes(upstream.bodies[start], field).Raw, gjson.GetBytes(upstream.bodies[start+1], field).Raw, field)
					}
				}
			})
		}
	}
}

func TestOMPMissingCacheKeyIsPreparedBeforeRouteSplit(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		name := "normal"
		if passthrough {
			name = "passthrough"
		}
		t.Run(name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{resp: openAICompatSSECompletedResponse("resp_missing", "gpt-6-astra")}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream, cache: &stubGatewayCache{}}
			account := ompRegressionAccount("session", passthrough)
			c := ompRegressionContext(1652, map[string]string{"User-Agent": "omp/18.2.6"})
			body := []byte(`{"model":"gpt-6-astra","instructions":"Work carefully.","stream":true,"input":"Inspect the repository"}`)
			expected := deriveOMPResponsesPromptCacheKey(c, body, "gpt-6-astra")
			require.NotEmpty(t, expected)

			result, forwardErr := svc.Forward(context.Background(), c, account, body)
			require.NoError(t, forwardErr)
			require.NotNil(t, result)
			scoped := scopeCodexAccountIdentityValue(account, 1652, "prompt-cache", expected)
			require.Equal(t, scoped, gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String())
			require.Equal(t, scoped, upstream.lastReq.Header.Get("session-id"))
		})
	}
}

func TestOMPToolRoundTripKeepsCacheRouting(t *testing.T) {
	const cacheKey = "01a0ddfa-3fb5-7000-b75a-24ddb2145816"
	first := []byte(`{"model":"gpt-6-astra","instructions":"Tool test","stream":true,"prompt_cache_key":"` + cacheKey + `","store":false,"tools":[{"type":"function","name":"read","parameters":{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}}],"input":[{"role":"user","content":[{"type":"input_text","text":"Read the fixture"}]}]}`)
	continued := []byte(`{"model":"gpt-6-astra","instructions":"Tool test","stream":true,"prompt_cache_key":"` + cacheKey + `","store":false,"tools":[{"type":"function","name":"read","parameters":{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}}],"input":[{"role":"user","content":[{"type":"input_text","text":"Read the fixture"}]},{"type":"function_call","call_id":"call_omp","name":"read","arguments":"{\"path\":\"tool-fixture.txt\"}"},{"type":"function_call_output","call_id":"call_omp","output":"OMP tool round trip fixture."}]}`)

	for _, passthrough := range []bool{false, true} {
		name := "normal"
		if passthrough {
			name = "passthrough"
		}
		t.Run(name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{responses: []*http.Response{
				openAICompatSSECompletedResponse("resp_tool_1", "gpt-6-astra"),
				openAICompatSSECompletedResponse("resp_tool_2", "gpt-6-astra"),
			}}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream, cache: &stubGatewayCache{}}
			account := ompRegressionAccount("session", passthrough)
			for _, body := range [][]byte{first, continued} {
				c := ompRegressionContext(1652, map[string]string{"User-Agent": "omp/18.3.2"})
				result, forwardErr := svc.Forward(context.Background(), c, account, body)
				require.NoError(t, forwardErr)
				require.NotNil(t, result)
			}

			require.Len(t, upstream.bodies, 2)
			expectedKey := scopeCodexAccountIdentityValue(account, 1652, "prompt-cache", cacheKey)
			for i := range upstream.bodies {
				require.Equal(t, expectedKey, gjson.GetBytes(upstream.bodies[i], "prompt_cache_key").String())
				require.Equal(t, expectedKey, upstream.requests[i].Header.Get("session-id"))
				require.Empty(t, upstream.requests[i].Header.Get("session_id"))
			}
			for _, field := range []string{"instructions", "tools", "input.0"} {
				require.Equal(t, gjson.GetBytes(upstream.bodies[0], field).Raw, gjson.GetBytes(upstream.bodies[1], field).Raw, field)
			}
			require.Equal(t, "function_call", gjson.GetBytes(upstream.bodies[1], "input.1.type").String())
			require.Equal(t, "function_call_output", gjson.GetBytes(upstream.bodies[1], "input.2.type").String())
		})
	}
}

func TestCodexCacheOnlyHTTPIdentityRequiresCacheWithoutConversation(t *testing.T) {
	account := ompRegressionAccount("session", false)
	for _, tc := range []struct {
		name, body, header, value string
	}{
		{name: "valid", body: `{"prompt_cache_key":"shared"}`},
		{name: "no_cache", body: `{"input":"hello"}`},
		{name: "body_session", body: `{"prompt_cache_key":"shared","client_metadata":{"session_id":"chat"}}`},
		{name: "body_thread_alias", body: `{"prompt_cache_key":"shared","client_metadata":{"thread-id":"thread"}}`},
		{name: "body_embedded", body: `{"prompt_cache_key":"shared","client_metadata":{"x-codex-turn-metadata":"{\"session_id\":\"chat\"}"}}`},
		{name: "header_session", body: `{"prompt_cache_key":"shared"}`, header: "session-id", value: "chat"},
		{name: "header_conversation", body: `{"prompt_cache_key":"shared"}`, header: "conversation_id", value: "chat"},
		{name: "header_thread", body: `{"prompt_cache_key":"shared"}`, header: "thread-id", value: "thread"},
		{name: "native_ws", body: `{"prompt_cache_key":"shared"}`, header: "Upgrade", value: "websocket"},
		{name: "native_ws_frame", body: `{"type":"response.create","prompt_cache_key":"shared"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := ompRegressionContext(1652, nil)
			if tc.header != "" {
				c.Request.Header.Set(tc.header, tc.value)
			}
			got := stageCodexCacheOnlyHTTPIdentity(c, account, []byte(tc.body))
			require.Equal(t, tc.name == "valid", got)
		})
	}

	c := ompRegressionContext(1652, nil)
	body := []byte(`{"prompt_cache_key":"shared"}`)
	require.False(t, stageCodexCacheOnlyHTTPIdentity(c, &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, body))
	c.Request.URL.Path = "/v1/responses/compact"
	require.False(t, stageCodexCacheOnlyHTTPIdentity(c, account, body))
}

func TestCodexCacheOnlyHTTPRoutingRequiresScopedMarker(t *testing.T) {
	account := ompRegressionAccount("session", false)
	for _, reason := range []string{"valid", "unstaged", "raw_key", "no_api_key", "changed_api_key", "other_host", "websocket", "compact"} {
		t.Run(reason, func(t *testing.T) {
			c := ompRegressionContext(1652, nil)
			body := map[string]any{"prompt_cache_key": "shared-prefix"}
			applyCodexAccountIdentityClientMetadataMap(body, account, 1652)
			if reason == "raw_key" {
				body["prompt_cache_key"] = "unscoped-raw-key"
			}
			if reason == "no_api_key" {
				c.Set("api_key", &APIKey{})
			}
			if reason != "unstaged" {
				require.True(t, stageCodexCacheOnlyHTTPIdentity(c, account, body))
			}
			request := httptest.NewRequest(http.MethodPost, "https://chatgpt.com/backend-api/codex/responses", nil)
			switch reason {
			case "changed_api_key":
				c.Set("api_key", &APIKey{ID: 1653})
			case "other_host":
				request.URL.Host = "api.openai.com"
			case "websocket":
				request.Header.Set("Upgrade", "websocket")
			case "compact":
				request.URL.Path += "/compact"
			}
			applyCodexCacheOnlyHTTPRoutingHeaders(c, account, request)
			if reason == "valid" {
				require.Equal(t, body["prompt_cache_key"], request.Header.Get("session-id"))
			} else {
				require.Empty(t, request.Header.Get("session-id"))
			}
		})
	}
}

func TestCodexCacheOnlyHTTPIdentityUsesShadowCredentialSource(t *testing.T) {
	parent := ompRegressionAccount("session", false)
	parent.ID = 9001
	parentID := parent.ID
	shadow := &Account{
		ID:              9002,
		ParentAccountID: &parentID,
		Platform:        PlatformOpenAI,
		Type:            AccountTypeOAuth,
	}
	c := ompRegressionContext(1652, nil)
	c.Set(codexAccountIdentitySourceContextKey, parent)
	body := map[string]any{"prompt_cache_key": "shared-prefix"}
	applyCodexAccountIdentityClientMetadataMap(body, parent, 1652)

	require.True(t, stageCodexCacheOnlyHTTPIdentity(c, shadow, body))
	require.True(t, isCodexCacheOnlyHTTPIdentity(c, shadow))
	request := httptest.NewRequest(http.MethodPost, "https://chatgpt.com/backend-api/codex/responses", nil)
	applyCodexCacheOnlyHTTPRoutingHeaders(c, shadow, request)
	require.Equal(t, body["prompt_cache_key"], request.Header.Get("session-id"))
}
