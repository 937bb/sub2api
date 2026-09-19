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
	"github.com/tidwall/sjson"
)

func TestOMPRecordedResponsesKeepCacheWithoutInventingConversations(t *testing.T) {
	data, err := os.ReadFile("testdata/omp_responses_18_2_6.json")
	require.NoError(t, err)
	var fixtures []struct {
		Name    string            `json:"name"`
		Headers map[string]string `json:"headers"`
		Body    json.RawMessage   `json:"body"`
	}
	require.NoError(t, json.Unmarshal(data, &fixtures))
	require.Len(t, fixtures, 4)

	for _, mode := range []string{"off", "device", "session", "full"} {
		for _, passthrough := range []bool{false, true} {
			for _, ua := range []string{"omp/18.2.6", "Go-http-client/1.1"} {
				t.Run(mode+"/"+map[bool]string{false: "normal", true: "passthrough"}[passthrough]+"/"+ua, func(t *testing.T) {
					upstream := &httpUpstreamRecorder{}
					for range fixtures {
						upstream.responses = append(upstream.responses, openAICompatSSECompletedResponse("resp_wire", "gpt-6-astra"))
					}
					svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream, cache: &stubGatewayCache{}}
					account := conversationTestAccount(AccountTypeOAuth, mode)
					account.Credentials["access_token"] = "test-token"
					account.Extra["openai_oauth_passthrough"] = passthrough
					for _, fixture := range fixtures {
						c, _ := gin.CreateTestContext(httptest.NewRecorder())
						c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
						for key, value := range fixture.Headers {
							c.Request.Header.Set(key, value)
						}
						c.Request.Header.Set("User-Agent", ua)
						c.Set("api_key", &APIKey{ID: 1652})
						result, err := svc.Forward(context.Background(), c, account, fixture.Body)
						require.NoError(t, err, fixture.Name)
						require.NotNil(t, result)
					}
					require.Len(t, upstream.bodies, 4)
					for i, body := range upstream.bodies {
						inputKey := gjson.GetBytes(fixtures[i].Body, "prompt_cache_key").String()
						require.Equal(t, scopeCodexAccountIdentityValue(account, 1652, "prompt-cache", inputKey), gjson.GetBytes(body, "prompt_cache_key").String())
						for _, field := range []string{"session_id", "thread_id", "turn_id"} {
							require.Empty(t, gjson.GetBytes(body, "client_metadata."+field).String(), field)
							require.Empty(t, gjson.Get(gjson.GetBytes(body, "client_metadata.x-codex-turn-metadata").String(), field).String(), field)
							require.Empty(t, gjson.Get(upstream.requests[i].Header.Get("x-codex-turn-metadata"), field).String(), field)
						}
						for _, header := range []string{"session-id", "session_id", "thread-id", "conversation_id"} {
							require.Empty(t, upstream.requests[i].Header.Get(header), header)
						}
					}
					for _, start := range []int{0, 2} {
						first, second := upstream.bodies[start], upstream.bodies[start+1]
						for _, field := range []string{"prompt_cache_key", "instructions", "tools", "input.0"} {
							require.Equal(t, gjson.GetBytes(first, field).Raw, gjson.GetBytes(second, field).Raw, field)
						}
					}
					require.NotEqual(t, gjson.GetBytes(upstream.bodies[0], "prompt_cache_key").String(), gjson.GetBytes(upstream.bodies[2], "prompt_cache_key").String())
				})
			}
		}
	}
}

func TestOMPMissingCacheKeyIsPreparedBeforeRouteSplit(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		t.Run(map[bool]string{false: "normal", true: "passthrough"}[passthrough], func(t *testing.T) {
			upstream := &httpUpstreamRecorder{responses: []*http.Response{openAICompatSSECompletedResponse("resp_missing", "gpt-6-astra")}}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream, cache: &stubGatewayCache{}}
			account := conversationTestAccount(AccountTypeOAuth, "session")
			account.Credentials["access_token"] = "test-token"
			account.Extra["openai_oauth_passthrough"] = passthrough
			c := conversationTestContext(1652, "")
			c.Request.Header.Set("User-Agent", "omp/18.2.6")
			body := []byte(`{"model":"gpt-6-astra","instructions":"Work carefully.","stream":true,"input":"Inspect the repository"}`)
			expected := deriveOMPResponsesPromptCacheKey(c, body, "gpt-6-astra")
			require.NotEmpty(t, expected)
			_, err := svc.Forward(context.Background(), c, account, body)
			require.NoError(t, err)
			require.Len(t, upstream.bodies, 1)
			require.Equal(t, scopeCodexAccountIdentityValue(account, 1652, "prompt-cache", expected), gjson.GetBytes(upstream.bodies[0], "prompt_cache_key").String())
		})
	}
}

func TestOMPCachePreparationPreservesExplicitKeysAndAPIKeyRequests(t *testing.T) {
	c := conversationTestContext(1652, "")
	c.Request.Header.Set("User-Agent", "omp/18.2.6")
	body := []byte(`{"model":"gpt-6-astra","input":"hi"}`)
	apiKey := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	got, err := applyOMPResponsesPromptCacheKey(c, apiKey, body)
	require.NoError(t, err)
	require.Equal(t, body, got)
	explicit, err := sjson.SetBytes(body, "prompt_cache_key", "shared-cache")
	require.NoError(t, err)
	got, err = applyOMPResponsesPromptCacheKey(c, conversationTestAccount(AccountTypeOAuth, "session"), explicit)
	require.NoError(t, err)
	require.Equal(t, explicit, got)
}
