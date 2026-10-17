package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSIWCForwardPreservesPromptCacheControls(t *testing.T) {
	for _, codexClient := range []bool{false, true} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("codex=%t/stream=%t", codexClient, stream), func(t *testing.T) {
				body := []byte(fmt.Sprintf(`{"model":"gpt-6-astra","stream":%t,"prompt_cache_key":"client-cache","prompt_cache_options":{"mode":"explicit","ttl":"30m"},"prompt_cache_retention":"24h","input":[{"role":"user","content":[{"type":"input_text","text":"stable prefix","prompt_cache_breakpoint":{"mode":"explicit"}}]}]}`, stream))
				original := string(body)
				service := newOpenAIRejectedFieldTestService(nil)
				var sentBodies [][]byte
				service.siwcTransport = siwcCatalogRoundTripper(func(req *http.Request) (*http.Response, error) {
					sent, err := io.ReadAll(req.Body)
					require.NoError(t, err)
					sentBodies = append(sentBodies, sent)
					require.JSONEq(t, `{"mode":"explicit","ttl":"30m"}`, gjson.GetBytes(sent, "prompt_cache_options").Raw)
					require.JSONEq(t, `{"mode":"explicit"}`, gjson.GetBytes(sent, "input.0.content.0.prompt_cache_breakpoint").Raw)
					require.NotEmpty(t, gjson.GetBytes(sent, "prompt_cache_key").String())
					require.False(t, gjson.GetBytes(sent, "prompt_cache_retention").Exists())
					require.Empty(t, req.Header.Get("session_id"))
					require.Empty(t, req.Header.Get("conversation_id"))
					return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(siwcForwardSSE))}, nil
				})
				for range 2 {
					c := newOpenAIRejectedFieldTestContext(body)
					c.Set("api_key", &APIKey{ID: 100})
					if codexClient {
						c.Request.Header.Set("User-Agent", "codex_cli_rs/0.101.0")
						c.Request.Header.Set("originator", "codex_cli_rs")
					}
					result, err := service.Forward(context.Background(), c, siwcFixtureAccount(), body)
					require.NoError(t, err)
					require.Equal(t, 60, result.Usage.CacheReadInputTokens)
					require.Equal(t, stream, result.Stream)
				}
				require.Len(t, sentBodies, 2)
				require.Equal(t, sentBodies[0], sentBodies[1], "repeated forwards must not change the cache prefix or rehash a mutated body")
				require.Equal(t, original, string(body))
			})
		}
	}
}

func TestSIWCForwardPromptCacheKeyAcrossProtocols(t *testing.T) {
	for _, protocol := range []string{"responses", "chat", "chat_responses_shape", "messages"} {
		t.Run(protocol, func(t *testing.T) {
			forward := func(apiKeyID int64, subject, token, bodyKey, fallbackKey string) string {
				t.Helper()
				payload := map[string]any{"model": "gpt-6-astra", "stream": true}
				if protocol == "responses" || protocol == "chat_responses_shape" {
					payload["input"] = "stable prefix"
				} else {
					payload["messages"] = []any{map[string]any{"role": "user", "content": "stable prefix"}}
				}
				if bodyKey != "" {
					payload["prompt_cache_key"] = bodyKey
				}
				body, err := json.Marshal(payload)
				require.NoError(t, err)
				c := newOpenAIRejectedFieldTestContext(body)
				c.Set("api_key", &APIKey{ID: apiKeyID})
				account := siwcFixtureAccount()
				account.Credentials["subject"] = subject
				account.Credentials["client_id"] = "oaiapp_cache_test"
				account.Credentials["access_token"] = token
				service := newOpenAIRejectedFieldTestService(nil)
				var sent []byte
				service.siwcTransport = siwcCatalogRoundTripper(func(req *http.Request) (*http.Response, error) {
					sent, err = io.ReadAll(req.Body)
					require.NoError(t, err)
					require.Empty(t, req.Header.Get("session_id"))
					require.Empty(t, req.Header.Get("conversation_id"))
					return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(siwcForwardSSE))}, nil
				})
				var result *OpenAIForwardResult
				switch protocol {
				case "responses":
					result, err = service.Forward(context.Background(), c, account, body)
				case "chat", "chat_responses_shape":
					result, err = service.ForwardAsChatCompletions(context.Background(), c, account, body, fallbackKey, "")
				case "messages":
					result, err = service.ForwardAsAnthropic(context.Background(), c, account, body, fallbackKey, "")
				}
				require.NoError(t, err)
				require.Equal(t, 60, result.Usage.CacheReadInputTokens)
				require.NotEmpty(t, sent)
				require.False(t, gjson.GetBytes(sent, "prompt_cache_options").Exists(), "do not invent cache options")
				return gjson.GetBytes(sent, "prompt_cache_key").String()
			}
			bodyKey := ""
			if protocol == "responses" || protocol == "chat_responses_shape" {
				bodyKey = "client-cache"
			}
			first := forward(100, "subject-a", "token-a", bodyKey, "client-cache")
			require.NotEmpty(t, first, "SIWC must receive the compatible client's cache key in JSON")
			require.NotEqual(t, "client-cache", first, "raw client session identifiers must be isolated")
			require.Equal(t, first, forward(100, "subject-a", "refreshed-token", bodyKey, "client-cache"), "token refresh must not rotate the cache key")
			require.NotEqual(t, first, forward(101, "subject-a", "token-a", bodyKey, "client-cache"), "different tenants must not share a cache key")
			require.NotEqual(t, first, forward(100, "subject-b", "token-b", bodyKey, "client-cache"), "different upstream identities must not share a cache key")
			if protocol == "chat_responses_shape" {
				require.Equal(t, first, forward(100, "subject-a", "token-a", bodyKey, "header-cache"), "an existing Responses body key takes precedence over the fallback")
			}
			if protocol == "responses" || protocol == "chat" || protocol == "chat_responses_shape" {
				require.Empty(t, forward(100, "subject-a", "token-a", "", ""), "do not invent a cache key when the caller has none")
			}
		})
	}
}
