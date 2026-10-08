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

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/siwc"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSIWCCompatibilityForwardPreservesHistoryAndUsage(t *testing.T) {
	for _, protocol := range []string{"chat", "messages"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", protocol, stream), func(t *testing.T) {
				messages := make([]map[string]any, 0, 16)
				if protocol == "chat" {
					messages = append(messages, map[string]any{"role": "system", "content": "caller system instructions"})
				}
				for i := range 15 {
					role := "user"
					if i%2 == 1 {
						role = "assistant"
					}
					messages = append(messages, map[string]any{"role": role, "content": fmt.Sprintf("history-message-%02d", i)})
				}
				payload := map[string]any{"model": "gpt-6-astra", "messages": messages, "stream": stream, "max_tokens": 32}
				path := "/v1/chat/completions"
				if protocol == "messages" {
					path = "/v1/messages"
					payload["system"] = "caller system instructions"
				}
				body, err := json.Marshal(payload)
				require.NoError(t, err)
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
				for _, header := range []string{"Authorization", "Cookie", "User-Agent", "Session_id", "Conversation_id", "ChatGPT-Account-ID", "Originator", "OpenAI-Beta", "X-Codex-Turn-State"} {
					c.Request.Header.Set(header, "inbound-private-value")
				}
				calls := 0
				service := &OpenAIGatewayService{cfg: &config.Config{}}
				service.siwcTransport = siwcCatalogRoundTripper(func(req *http.Request) (*http.Response, error) {
					calls++
					require.Equal(t, siwc.ResponsesURL, req.URL.String())
					require.Equal(t, siwc.UserAgent, req.UserAgent())
					require.Equal(t, "Bearer upstream-token", req.Header.Get("Authorization"))
					for _, header := range []string{"Cookie", "Session_id", "Conversation_id", "ChatGPT-Account-ID", "Originator", "OpenAI-Beta", "X-Codex-Turn-State"} {
						require.Empty(t, req.Header.Get(header), header)
					}
					upstreamBody, err := io.ReadAll(req.Body)
					require.NoError(t, err)
					require.Equal(t, "gpt-6-astra", gjson.GetBytes(upstreamBody, "model").String())
					require.True(t, gjson.GetBytes(upstreamBody, "stream").Bool())
					require.False(t, gjson.GetBytes(upstreamBody, "store").Bool())
					require.False(t, gjson.GetBytes(upstreamBody, "previous_response_id").Exists())
					require.Contains(t, string(upstreamBody), "caller system instructions")
					require.NotContains(t, string(upstreamBody), openAICompatClaudeCodeTodoGuardMarker)
					for i := range 15 {
						require.Contains(t, string(upstreamBody), fmt.Sprintf("history-message-%02d", i))
					}
					wire := "data: {\"type\":\"response.output_text.delta\",\"output_index\":0,\"content_index\":0,\"delta\":\"hello\"}\n\n" +
						"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_siwc\",\"object\":\"response\",\"model\":\"gpt-6-astra\",\"status\":\"completed\",\"output\":[{\"id\":\"msg_siwc\",\"type\":\"message\",\"role\":\"assistant\",\"status\":\"completed\",\"content\":[{\"type\":\"output_text\",\"text\":\"hello\"}]}],\"usage\":{\"input_tokens\":23,\"output_tokens\":7,\"input_tokens_details\":{\"cached_tokens\":5}}}}\n\n"
					return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(wire))}, nil
				})
				account := siwcFixtureAccount()
				var result *OpenAIForwardResult
				if protocol == "chat" {
					result, err = service.ForwardAsChatCompletions(context.Background(), c, account, body, "caller-cache-key", "")
				} else {
					result, err = service.ForwardAsAnthropic(context.Background(), c, account, body, "caller-cache-key", "")
				}
				require.NoError(t, err)
				require.Equal(t, 1, calls)
				require.Equal(t, http.StatusOK, recorder.Code)
				require.Contains(t, recorder.Body.String(), "hello")
				require.NotNil(t, result)
				require.Equal(t, "gpt-6-astra", result.UpstreamModel)
				require.Equal(t, 23, result.Usage.InputTokens)
				require.Equal(t, 7, result.Usage.OutputTokens)
				require.Equal(t, 5, result.Usage.CacheReadInputTokens)
				if stream {
					require.Contains(t, recorder.Header().Get("Content-Type"), "text/event-stream")
					require.True(t, recorder.Flushed)
				} else {
					require.True(t, json.Valid(recorder.Body.Bytes()))
				}
			})
		}
	}
}

func TestSIWCTokenCountingNeverAcquiresCredentialsOrSendsUpstream(t *testing.T) {
	account := siwcFixtureAccount()
	delete(account.Credentials, "access_token")
	service := &OpenAIGatewayService{siwcTransport: siwcCatalogRoundTripper(func(*http.Request) (*http.Response, error) {
		t.Fatal("SIWC token-counting must not call upstream")
		return nil, nil
	})}
	for _, protocol := range []string{"responses", "messages"} {
		t.Run(protocol, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			var err error
			if protocol == "responses" {
				body := []byte(`{"model":"gpt-6-astra","input":"hello from the full request"}`)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/input_tokens", bytes.NewReader(body))
				err = service.ForwardResponsesInputTokens(context.Background(), c, account, body)
				require.Equal(t, "response.input_tokens", gjson.GetBytes(recorder.Body.Bytes(), "object").String())
			} else {
				body := []byte(`{"model":"gpt-6-astra","messages":[{"role":"user","content":"hello from the full request"}]}`)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", bytes.NewReader(body))
				err = service.ForwardCountTokensAsAnthropic(context.Background(), c, account, body, "")
			}
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, recorder.Code)
			require.Greater(t, gjson.GetBytes(recorder.Body.Bytes(), "input_tokens").Int(), int64(1))
			_, err = service.buildInputTokensUpstreamRequest(context.Background(), c, account, []byte(`{}`), "must-not-send")
			require.ErrorContains(t, err, "local estimation")
		})
	}
}

func TestSIWCUnsupportedCodexBuildersRejectBeforeCredentialAcquisition(t *testing.T) {
	service := &OpenAIGatewayService{}
	account := siwcFixtureAccount()
	delete(account.Credentials, "access_token")
	ctx := context.Background()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/alpha/search", nil)
	_, err := service.ForwardAlphaSearch(ctx, c, account, []byte(`{"model":"gpt-6-astra","query":"hello"}`))
	require.ErrorContains(t, err, "SIWC standalone alpha search")
	_, err = service.buildOpenAIAlphaSearchRequest(ctx, c, account, []byte(`{}`), "must-not-send")
	require.ErrorContains(t, err, "SIWC standalone alpha search")
	_, err = service.openAIAlphaSearchURL(account)
	require.ErrorContains(t, err, "SIWC standalone alpha search")
	_, err = service.createUpstreamLiveCall(ctx, account, nil, "")
	require.ErrorContains(t, err, "SIWC Live calls")
	_, err = service.liveSidebandHeaders(ctx, account, nil)
	require.ErrorContains(t, err, "SIWC Live sideband")
}
