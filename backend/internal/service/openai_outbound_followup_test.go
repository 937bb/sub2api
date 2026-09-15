package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/imroc/req/v3"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestCodexOutboundFollowupPrivacyAndQuotaHeaders(t *testing.T) {
	var captured []*http.Request
	factory := func(string) (*req.Client, error) {
		client := req.C().ImpersonateChrome()
		client.GetTransport().WrapRoundTripFunc(func(http.RoundTripper) req.HttpRoundTripFunc {
			return func(r *http.Request) (*http.Response, error) {
				captured = append(captured, r.Clone(r.Context()))
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{}`)), Request: r}, nil
			}
		})
		return client, nil
	}
	require.Equal(t, PrivacyModeTrainingOff, disableOpenAITraining(context.Background(), factory, "dummy-token", ""))
	fetchChatGPTAccountInfo(context.Background(), factory, "dummy-token", "", "dummy-account")
	fetchChatGPTSubscriptionExpiresAt(context.Background(), factory, "dummy-token", "", "dummy-account")
	client, err := factory("")
	require.NoError(t, err)
	_, err = client.R().SetHeaders(buildCodexCommonHeaders("dummy-token", "dummy-account", false)).Get(chatGPTUsageURL)
	require.NoError(t, err)
	require.Len(t, captured, 4)
	for _, r := range captured {
		require.Equal(t, codexClientAcceptLanguage, r.Header.Get("Accept-Language"), r.URL.Path)
	}
	require.Equal(t, codexClientLocale, captured[3].Header.Get("Oai-Language"))
}

func TestCodexOutboundFollowupImageIdentity(t *testing.T) {
	for _, kind := range []string{AccountTypeOAuth, AccountTypeSetupToken} {
		t.Run(kind, func(t *testing.T) {
			account := &Account{ID: 77, Platform: PlatformOpenAI, Type: kind,
				Credentials: map[string]any{"access_token": "dummy-token", "chatgpt_account_id": "dummy-account"},
				Extra:       map[string]any{codexFingerprintModeExtraKey: "full", codexFingerprintSeedExtraKey: testCodexFingerprintSeed}}
			var sessions []string
			var installations []string
			for _, apiKeyID := range []int64{42, 42, 43} {
				body := []byte(`{"model":"gpt-image-2","prompt":"draw a cat"}`)
				c, _ := newOpenAIImagesTestContext(t, body)
				c.Set("api_key", &APIKey{ID: apiKeyID})
				c.Request.Header.Set("Session_ID", "same-downstream-session")
				upstream := &httpUpstreamRecorder{resp: &http.Response{
					StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}},
					Body: io.NopCloser(strings.NewReader("data: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"type\":\"image_generation_call\",\"result\":\"aGVsbG8=\",\"output_format\":\"png\"}]}}\n\n")),
				}}
				svc := newOpenAIImagesTestService(upstream)
				parsed, err := svc.ParseOpenAIImagesRequest(c, body)
				require.NoError(t, err)
				_, err = svc.ForwardImages(context.Background(), c, account, body, parsed, "")
				require.NoError(t, err)
				require.NotNil(t, upstream.lastReq)
				h := upstream.lastReq.Header
				require.Empty(t, h.Get("OpenAI-Beta"))
				require.NotNil(t, stagedCodexFingerprintIDs(c, account))
				metadata := gjson.GetBytes(upstream.lastBody, "client_metadata")
				require.NotEmpty(t, h.Get("x-codex-installation-id"))
				require.Equal(t, metadata.Get("x-codex-installation-id").String(), h.Get("x-codex-installation-id"))
				require.Equal(t, metadata.Get("session_id").String(), h.Get("session_id"))
				require.Equal(t, "draw a cat", gjson.GetBytes(upstream.lastBody, "input.0.content.0.text").String())
				sessions = append(sessions, h.Get("session_id"))
				installations = append(installations, h.Get("x-codex-installation-id"))
			}
			require.Equal(t, sessions[0], sessions[1])
			require.NotEqual(t, sessions[0], sessions[2])
			require.Equal(t, installations[0], installations[2])
		})
	}
}

func TestCodexOutboundFollowupEnvironmentRepresentations(t *testing.T) {
	fixture := "<environment_context><timezone>Asia/Shanghai</timezone></environment_context>"
	prefix := "# AGENTS.md instructions for /repo\n\n<INSTRUCTIONS>\nExample: " + fixture + "\n</INSTRUCTIONS>\n"
	want := prefix + strings.ReplaceAll(fixture, "Asia/Shanghai", codexClientTimezone)
	require.Equal(t, want, normalizeCodexEnvironmentContext(prefix+fixture))
	for _, input := range []string{"Explain " + fixture, "```xml\n" + fixture + "\n```", prefix + "Please explain " + fixture, prefix + fixture + " extra prose", "# AGENTS.md instructions for /repo\n" + fixture} {
		require.Equal(t, input, normalizeCodexEnvironmentContext(input))
	}
	body := []byte(`{"input":"<\u0065nvironment_context><timezone>Asia/Shanghai</timezone></\u0065nvironment_context>"}`)
	raw, changed := applyCodexEnvironmentContextRaw(body)
	require.True(t, changed)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(body, &decoded))
	require.True(t, applyCodexEnvironmentContextMap(decoded))
	require.Equal(t, decoded["input"], gjson.GetBytes(raw, "input").String())
	require.Contains(t, decoded["input"], codexClientTimezone)
}

func TestCodexOutboundFollowupDelegationAndMetadataProjection(t *testing.T) {
	for _, kind := range []string{AccountTypeOAuth, AccountTypeSetupToken, AccountTypeAPIKey} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		c.Request.Header.Set("x-openai-subagent", "reviewer")
		c.Request.Header.Set("x-codex-parent-thread-id", "dummy-parent")
		account := &Account{ID: 77, Platform: PlatformOpenAI, Type: kind}
		headers := make(http.Header)
		copyOpenAICodexDelegationHeaders(c, account, true, headers)
		metadata := `{"tool_namespaces_info":{"mcp":{"tools":["example"]}},"agent_name":"custom-agent","counter":9007199254740993}`
		headers.Set(openAIWSTurnMetadataHeader, metadata)
		applyCodexClientEnvironmentHeaders(headers, account)
		if kind == AccountTypeAPIKey {
			require.Empty(t, headers.Get("x-openai-subagent"))
			require.Equal(t, metadata, headers.Get(openAIWSTurnMetadataHeader))
			continue
		}
		require.Equal(t, "reviewer", headers.Get("x-openai-subagent"))
		require.Equal(t, "dummy-parent", headers.Get("x-codex-parent-thread-id"))
		next := gjson.Parse(headers.Get(openAIWSTurnMetadataHeader))
		require.False(t, next.Get("tool_namespaces_info").Exists())
		require.Equal(t, "custom-agent", next.Get("agent_name").String())
		require.Equal(t, "9007199254740993", next.Get("counter").Raw)
		body := map[string]any{"client_metadata": map[string]any{openAIWSTurnMetadataHeader: metadata}}
		applyCodexClientEnvironmentMap(body, account)
		require.Equal(t, metadata, body["client_metadata"].(map[string]any)[openAIWSTurnMetadataHeader])
	}
}
