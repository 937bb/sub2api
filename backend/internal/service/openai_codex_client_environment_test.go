package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestCodexClientEnvironmentRawPreservesPayload(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","prompt_cache_key":"unchanged-cache","instructions":"sub2api Asia/Shanghai 中文","input":[{"role":"user","content":[{"type":"input_text","text":"Asia/Shanghai sub2api 中文"},{"type":"input_image","image_url":"data:image/png;base64,AAAA"}]},{"type":"function_call_output","call_id":"call-1","output":"sub2api Asia/Shanghai"}],"tools":[{"type":"function","name":"sub2api","parameters":{"type":"object"}}],"client_metadata":{"timezone":"Asia/Shanghai","locale":"zh-CN","country_code":"CN","client_name":"Sub2API/0.2.4","counter":9007199254740993,"routing_hint":"opaque-cn-value","environment":{"timeZone":"UTC"},"x-codex-turn-metadata":"{\"timezone\":\"Asia/Shanghai\",\"thread_id\":\"keep-thread\"}"}}`)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	next, changed, err := applyCodexClientEnvironmentRaw(body, account)
	require.NoError(t, err)
	require.True(t, changed)
	for _, path := range []string{"instructions", "input", "tools", "prompt_cache_key", "client_metadata.counter", "client_metadata.routing_hint"} {
		require.Equal(t, gjson.GetBytes(body, path).Raw, gjson.GetBytes(next, path).Raw, path)
	}
	require.Equal(t, codexClientTimezone, gjson.GetBytes(next, "client_metadata.timezone").String())
	require.Equal(t, codexClientTimezone, gjson.GetBytes(next, "client_metadata.environment.timeZone").String())
	require.Equal(t, codexClientLocale, gjson.GetBytes(next, "client_metadata.locale").String())
	require.Equal(t, "US", gjson.GetBytes(next, "client_metadata.country_code").String())
	require.Equal(t, "api-client", gjson.GetBytes(next, "client_metadata.client_name").String())
	embedded := gjson.Parse(gjson.GetBytes(next, "client_metadata.x-codex-turn-metadata").String())
	require.Equal(t, codexClientTimezone, embedded.Get("timezone").String())
	require.Equal(t, "keep-thread", embedded.Get("thread_id").String())
	again, changed, err := applyCodexClientEnvironmentRaw(next, account)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, next, again)
}

func TestCodexClientEnvironmentAccountBoundaries(t *testing.T) {
	for _, account := range []*Account{nil, {Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, {Platform: PlatformAnthropic, Type: AccountTypeOAuth}} {
		body := []byte(`{"client_metadata":{"timezone":"Asia/Shanghai"}}`)
		next, changed, err := applyCodexClientEnvironmentRaw(body, account)
		require.NoError(t, err)
		require.False(t, changed)
		require.Equal(t, body, next)
		headers := http.Header{"Accept-Language": {"zh-CN"}}
		applyCodexClientEnvironmentHeaders(headers, account)
		require.Equal(t, "zh-CN", headers.Get("Accept-Language"))
	}
	for _, body := range []string{`{}`, `{"client_metadata":null}`, `{"client_metadata":"opaque"}`, `{"client_metadata":{"timezone":12,"other":"Asia/Shanghai"}}`} {
		next, changed, err := applyCodexClientEnvironmentRaw([]byte(body), &Account{Platform: PlatformOpenAI, Type: AccountTypeSetupToken})
		require.NoError(t, err)
		require.False(t, changed)
		require.Equal(t, body, string(next))
	}
}

func TestCodexClientEnvironmentHTTPBoundaries(t *testing.T) {
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeSetupToken} {
		for _, path := range []string{"/v1/responses", "/v1/responses/compact"} {
			t.Run(accountType+path, func(t *testing.T) {
				account := &Account{ID: 77, Platform: PlatformOpenAI, Type: accountType,
					Credentials: map[string]any{"chatgpt_account_id": "test-account"}}
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, path, nil)
				c.Request.Header.Set("Accept-Language", "zh-CN")
				c.Request.Header.Set(openAIWSTurnMetadataHeader, `{"timezone":"Asia/Shanghai","session_id":"s1"}`)
				body := []byte(`{"model":"gpt-5.5","input":[{"role":"user","content":"中文 Asia/Shanghai"}],"client_metadata":{"timezone":"Asia/Shanghai"}}`)
				svc := &OpenAIGatewayService{}
				regular, err := svc.buildUpstreamRequest(context.Background(), c, account, body, "dummy-token", true, "", true)
				require.NoError(t, err)
				passthrough, err := svc.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, account, body, "dummy-token")
				require.NoError(t, err)
				for _, req := range []*http.Request{regular, passthrough} {
					applyCodexNormalizedRequestIdentityHeaders(c, account, req.Header, body)
					sent, err := io.ReadAll(req.Body)
					require.NoError(t, err)
					require.Equal(t, codexClientTimezone, gjson.GetBytes(sent, "client_metadata.timezone").String())
					require.Equal(t, gjson.GetBytes(body, "input").Raw, gjson.GetBytes(sent, "input").Raw)
					require.Equal(t, codexClientAcceptLanguage, req.Header.Get("Accept-Language"))
					require.Equal(t, codexClientTimezone, gjson.Get(req.Header.Get(openAIWSTurnMetadataHeader), "timezone").String())
				}
				require.Equal(t, "zh-CN", c.GetHeader("Accept-Language"))
			})
		}
	}
}

func TestCodexClientEnvironmentMapWithoutIdentityNamespace(t *testing.T) {
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{"client_metadata":{"tz":"Asia/Shanghai","x-codex-turn-metadata":"not-json"}}`), &payload))
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeSetupToken}
	require.True(t, applyCodexAccountIdentityClientMetadataMap(payload, account, 1))
	metadata := payload["client_metadata"].(map[string]any)
	require.Equal(t, codexClientTimezone, metadata["tz"])
	require.Equal(t, "not-json", metadata[openAIWSTurnMetadataHeader])
}

func TestCodexClientEnvironmentPacificDSTAndQuotaLanguage(t *testing.T) {
	location, err := time.LoadLocation(codexClientTimezone)
	require.NoError(t, err)
	_, winterOffset := time.Date(2026, time.January, 15, 12, 0, 0, 0, location).Zone()
	_, summerOffset := time.Date(2026, time.July, 15, 12, 0, 0, 0, location).Zone()
	require.Equal(t, -8*3600, winterOffset)
	require.Equal(t, -7*3600, summerOffset)
	require.Equal(t, codexClientLocale, buildCodexCommonHeaders("dummy", "test-account", false)["oai-language"])
}
