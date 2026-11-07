package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type kiroToolUpstream struct {
	t        *testing.T
	requests int
	apiKey   string
	endpoint string
}

func (u *kiroToolUpstream) Do(req *http.Request, proxy string, id int64, _ int) (*http.Response, error) {
	require.Equal(u.t, "http://test:secret@192.0.2.17:8000", proxy)
	require.EqualValues(u.t, 19, id)
	assertKiroTransportContext(u.t, req)
	if u.apiKey != "" {
		require.Equal(u.t, "Bearer "+u.apiKey, req.Header.Get("Authorization"))
		require.Equal(u.t, "API_KEY", req.Header.Get("tokentype"))
	}
	if req.URL.Host == "management.us-east-1.kiro.dev" {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"models":[{"modelId":"claude-haiku-4.5","modelName":"Haiku"}]}`))}, nil
	}
	if u.endpoint == "cli" {
		require.Equal(u.t, "q.us-east-1.amazonaws.com", req.URL.Host)
		require.Equal(u.t, "AmazonCodeWhispererStreamingService.GenerateAssistantResponse", req.Header.Get("X-Amz-Target"))
	} else {
		require.Equal(u.t, "runtime.us-east-1.kiro.dev", req.URL.Host)
	}
	u.requests++
	raw, err := io.ReadAll(req.Body)
	require.NoError(u.t, err)
	if u.apiKey != "" {
		require.NotContains(u.t, string(raw), "profileArn")
	}
	if u.endpoint == "cli" {
		require.Contains(u.t, string(raw), `"origin":"KIRO_CLI"`)
		require.NotContains(u.t, string(raw), "agentContinuationId")
	}
	var body struct {
		Conversation struct {
			Current struct {
				User struct {
					Context struct {
						Tools []struct {
							Spec struct {
								Name string `json:"name"`
							} `json:"toolSpecification"`
						} `json:"tools"`
					} `json:"userInputMessageContext"`
				} `json:"userInputMessage"`
			} `json:"currentMessage"`
		} `json:"conversationState"`
	}
	require.NoError(u.t, json.Unmarshal(raw, &body))
	definitions := body.Conversation.Current.User.Context.Tools
	require.Len(u.t, definitions, 2)
	for _, tool := range definitions {
		require.LessOrEqual(u.t, len(tool.Spec.Name), 63)
	}
	event := func(id, name, input string, stop bool) []byte {
		payload, _ := json.Marshal(map[string]any{"toolUseId": id, "name": name, "input": input, "stop": stop})
		return kiroTestFrame("toolUseEvent", string(payload))
	}
	frames := bytes.Join([][]byte{
		event("call_one", definitions[0].Spec.Name, `{"path":"`, false),
		event("call_two", definitions[1].Spec.Name, `{"path":"second.txt"}`, true),
		event("call_one", "", `first.txt"}`, true),
		kiroTestFrame("meteringEvent", `{"inputTokens":15,"outputTokens":9}`),
	}, nil)
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(frames))}, nil
}
func (u *kiroToolUpstream) DoWithTLS(r *http.Request, p string, id int64, c int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(r, p, id, c)
}

func TestKiroParallelToolsAllIngressProtocols(t *testing.T) {
	name := strings.Repeat("mcp__long_server_", 5) + "read"
	for _, variant := range []string{"oauth", "api-key", "api-key-cli"} {
		for _, protocol := range []string{"messages", "chat", "responses", "gemini"} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/stream=%v", variant, protocol, stream), func(t *testing.T) {
					upstream := &kiroToolUpstream{t: t}
					svc := &GatewayService{httpUpstream: upstream, cfg: &config.Config{}}
					account := fixedKiroAccount()
					account.Credentials["access_token"] = t.Name()
					account.Credentials["expires_at"] = time.Now().Add(time.Hour).Format(time.RFC3339)
					if variant != "oauth" {
						delete(account.Credentials, "access_token")
						delete(account.Credentials, "refresh_token")
						delete(account.Credentials, "expires_at")
						account.Credentials["auth_method"] = "api_key"
						account.Credentials["api_key"] = t.Name()
						account.Credentials["profile_arn"] = "must-not-be-forwarded"
						upstream.apiKey = t.Name()
					}
					if variant == "api-key-cli" {
						account.Credentials["endpoint"] = "cli"
						upstream.endpoint = "cli"
					}
					schema := map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}}
					body := map[string]any{"model": "claude-haiku-4.5", "max_tokens": 1024, "stream": stream, "messages": []any{map[string]any{"role": "user", "content": "read both files"}}}
					switch protocol {
					case "messages":
						body["tools"] = []any{map[string]any{"name": name, "input_schema": schema}, map[string]any{"name": "read_two", "input_schema": schema}}
					case "chat":
						body["tools"] = []any{map[string]any{"type": "function", "function": map[string]any{"name": name, "parameters": schema}}, map[string]any{"type": "function", "function": map[string]any{"name": "read_two", "parameters": schema}}}
					case "responses":
						delete(body, "messages")
						delete(body, "max_tokens")
						body["input"] = "read both files"
						body["tools"] = []any{map[string]any{"type": "function", "name": name, "parameters": schema}, map[string]any{"type": "function", "name": "read_two", "parameters": schema}}
					case "gemini":
						body = map[string]any{"contents": []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": "read both files"}}}}, "tools": []any{map[string]any{"functionDeclarations": []any{map[string]any{"name": name, "parameters": schema}, map[string]any{"name": "read_two", "parameters": schema}}}}}
					}
					raw, err := json.Marshal(body)
					require.NoError(t, err)
					recorder := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(recorder)
					c.Request = httptest.NewRequest("POST", "/v1/"+protocol, bytes.NewReader(raw))
					c.Set("api_key", &APIKey{ID: 1, UserID: 2})
					var result *ForwardResult
					switch protocol {
					case "messages":
						parsed, e := ParseGatewayRequest(NewRequestBodyRef(raw), PlatformAnthropic)
						require.NoError(t, e)
						result, err = svc.Forward(context.Background(), c, account, parsed)
					case "chat":
						result, err = svc.ForwardAsChatCompletions(context.Background(), c, account, raw, nil)
					case "responses":
						result, err = svc.ForwardAsResponses(context.Background(), c, account, raw, nil)
					case "gemini":
						result, err = svc.ForwardKiroGemini(context.Background(), c, account, "claude-haiku-4.5", "generateContent", stream, raw)
					}
					require.NoError(t, err)
					require.NotNil(t, result)
					require.Equal(t, 200, recorder.Code)
					text := recorder.Body.String()
					require.Contains(t, text, name)
					require.Contains(t, text, "read_two")
					require.Contains(t, text, "call_one")
					require.Contains(t, text, "call_two")
					// Full arguments must survive the terminal response, even when deltas interleave.
					if !stream || protocol == "responses" || protocol == "gemini" {
						require.Contains(t, text, "first.txt")
						require.Contains(t, text, "second.txt")
					}
					require.Equal(t, 15, result.Usage.InputTokens)
					require.Equal(t, 9, result.Usage.OutputTokens)
					require.Equal(t, 1, upstream.requests)
				})
			}
		}
	}

}

func TestKiroOpenAIToolControlsRejectedBeforeConversion(t *testing.T) {
	for _, protocol := range []string{"chat", "responses"} {
		for _, control := range []string{`"parallel_tool_calls":false`, `"tool_choice":"required"`, `"tool_choice":{"type":"function","name":"read"}`} {
			t.Run(protocol+control, func(t *testing.T) {
				raw := []byte(`{"model":"claude-haiku-4.5","messages":[{"role":"user","content":"go"}],"input":"go",` + control + `}`)
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest("POST", "/", bytes.NewReader(raw))
				svc := &GatewayService{cfg: &config.Config{}}
				var err error
				if protocol == "chat" {
					_, err = svc.ForwardAsChatCompletions(context.Background(), c, fixedKiroAccount(), raw, nil)
				} else {
					_, err = svc.ForwardAsResponses(context.Background(), c, fixedKiroAccount(), raw, nil)
				}
				require.Error(t, err)
				require.Equal(t, 400, rec.Code)
			})
		}
	}
}
