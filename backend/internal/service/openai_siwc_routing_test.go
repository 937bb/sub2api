package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/siwc"
	"github.com/stretchr/testify/require"
)

// Exercise the forwarding entry points with both grants in one service. The
// account's stored auth mode must select the endpoint and matching transport.
func TestSIWCMixedOAuthRouting(t *testing.T) {
	for _, protocol := range []string{"responses", "chat", "messages"} {
		for _, passthrough := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/passthrough=%t", protocol, passthrough), func(t *testing.T) {
				upstream := &httpUpstreamRecorder{}
				gateway := newOpenAIRejectedFieldTestService(upstream)
				publicCalls := 0
				gateway.siwcTransport = siwcCatalogRoundTripper(func(req *http.Request) (*http.Response, error) {
					publicCalls++
					require.Equal(t, siwc.ResponsesURL, req.URL.String())
					require.Equal(t, "Bearer siwc-token", req.Header.Get("Authorization"))
					require.Empty(t, req.Header.Get("ChatGPT-Account-ID"))
					return siwcRoutingResponse(), nil
				})
				// Switch back to an unmarked legacy account after a SIWC request
				// to catch accidental service-wide or cached endpoint selection.
				for _, mode := range []string{"", "oauth", "siwc", "SIWC", ""} {
					account := siwcFixtureAccount()
					account.Name = "SIWC display name does not select the route"
					account.Extra = map[string]any{"openai_passthrough": passthrough, "auth_mode": "siwc"}
					delete(account.Credentials, "auth_mode")
					if mode != "" {
						account.Credentials["auth_mode"] = mode
					}
					isSIWC := strings.EqualFold(mode, "siwc")
					account.Credentials["access_token"] = "legacy-token"
					account.Credentials["chatgpt_account_id"] = "legacy-chatgpt-account"
					if isSIWC {
						account.Credentials["access_token"] = "siwc-token"
					}
					body := []byte(`{"model":"gpt-6-astra","stream":true,"instructions":"Keep caller instructions","input":[{"role":"user","content":"hello"}]}`)
					path := "/v1/responses"
					if protocol != "responses" {
						body = []byte(`{"model":"gpt-6-astra","stream":true,"max_tokens":32,"messages":[{"role":"user","content":"hello"}]}`)
						path = "/v1/chat/completions"
						if protocol == "messages" {
							path = "/v1/messages"
						}
					}
					c := newOpenAIRejectedFieldTestContext(body)
					c.Request.URL.Path = path
					c.Request.Header.Set("X-Auth-Mode", "siwc")
					c.Request.Header.Set("Authorization", "Bearer customer-key")
					legacyBefore, publicBefore := len(upstream.requests), publicCalls
					upstream.resp = siwcRoutingResponse()
					var result *OpenAIForwardResult
					var err error
					switch protocol {
					case "responses":
						result, err = gateway.Forward(context.Background(), c, account, body)
					case "chat":
						result, err = gateway.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
					case "messages":
						result, err = gateway.ForwardAsAnthropic(context.Background(), c, account, body, "", "")
					}
					require.NoError(t, err, "auth mode %q", mode)
					require.NotNil(t, result)
					require.Equal(t, http.StatusOK, c.Writer.Status())
					require.Equal(t, 100, result.Usage.InputTokens)
					require.Equal(t, 5, result.Usage.OutputTokens)
					if isSIWC {
						require.Equal(t, legacyBefore, len(upstream.requests))
						require.Equal(t, publicBefore+1, publicCalls)
					} else {
						require.Equal(t, publicBefore, publicCalls)
						require.Equal(t, legacyBefore+1, len(upstream.requests))
						require.Equal(t, chatgptCodexURL, upstream.lastReq.URL.String())
						require.Equal(t, "Bearer legacy-token", upstream.lastReq.Header.Get("Authorization"))
						require.Equal(t, "legacy-chatgpt-account", upstream.lastReq.Header.Get("ChatGPT-Account-ID"))
					}
				}
			})
		}
	}
}

func siwcRoutingResponse() *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(siwcForwardSSE))}
}

func TestSIWCBuilderRejectsOtherAccountGrants(t *testing.T) {
	legacy := siwcFixtureAccount()
	delete(legacy.Credentials, "auth_mode")
	apiKey := siwcFixtureAccount()
	apiKey.Type = AccountTypeAPIKey
	otherPlatform := siwcFixtureAccount()
	otherPlatform.Platform = PlatformAnthropic
	for _, account := range []*Account{nil, legacy, apiKey, otherPlatform} {
		body := []byte(`{"model":"gpt-6-astra","input":"hello"}`)
		req, err := buildSIWCResponsesRequest(context.Background(), newOpenAIRejectedFieldTestContext(body), account, body, "must-not-send")
		require.ErrorContains(t, err, "SIWC account authorization required")
		require.Nil(t, req)
	}
}
