package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func enableTestCodexUSLocale(t *testing.T) {
	t.Helper()
	before := codexUSLocaleEnabled.Swap(true)
	t.Cleanup(func() { codexUSLocaleEnabled.Store(before) })
}

const localeEnvironment = "<environment_context>\n<cwd>/work</cwd>\n<current_date>2026-11-15</current_date>\n<timezone>Asia/Shanghai</timezone>\n</environment_context>"

func TestCodexUSLocalePreservesUserContentAndNormalizesOnlyEnvironment(t *testing.T) {
	enableTestCodexUSLocale(t)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	encoded, err := json.Marshal(map[string]any{
		"input": []any{
			map[string]any{"role": "user", "content": localeEnvironment},
			map[string]any{"role": "user", "content": "Explain Asia/Shanghai and 上海"},
			map[string]any{"role": "assistant", "content": localeEnvironment},
			map[string]any{"type": "function_call_output", "call_id": "call1", "output": localeEnvironment},
			map[string]any{"role": "developer", "content": []any{map[string]any{"type": "input_text", "text": localeEnvironment}}},
		},
		"instructions": "Discuss Shanghai", "opaque": "Asia/Shanghai",
		"client_metadata": map[string]any{"timezone": "Asia/Shanghai", "locale": "zh-CN", "utc_offset": "+08:00", "current_date": "2026-11-15", "custom": "Asia/Shanghai",
			"x-codex-turn-metadata": `{"timezone":"Asia/Shanghai","id":9007199254740993}`},
	})
	require.NoError(t, err)
	original := append([]byte(nil), encoded...)
	out, changed, err := normalizeCodexUSLocale(encoded, account)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, original, encoded)
	require.Contains(t, gjson.GetBytes(out, "input.0.content").String(), "<timezone>America/New_York</timezone>")
	require.NotContains(t, gjson.GetBytes(out, "input.0.content").String(), "current_date")
	require.Equal(t, "Explain Asia/Shanghai and 上海", gjson.GetBytes(out, "input.1.content").String())
	require.Equal(t, localeEnvironment, gjson.GetBytes(out, "input.2.content").String())
	require.Equal(t, localeEnvironment, gjson.GetBytes(out, "input.3.output").String())
	require.Contains(t, gjson.GetBytes(out, "input.4.content.0.text").String(), codexUSTimezone)
	require.Equal(t, "Asia/Shanghai", gjson.GetBytes(out, "opaque").String())
	require.Equal(t, "Discuss Shanghai", gjson.GetBytes(out, "instructions").String())
	require.Equal(t, codexUSTimezone, gjson.GetBytes(out, "client_metadata.timezone").String())
	require.Equal(t, "en-US", gjson.GetBytes(out, "client_metadata.locale").String())
	require.False(t, gjson.GetBytes(out, "client_metadata.utc_offset").Exists())
	require.False(t, gjson.GetBytes(out, "client_metadata.current_date").Exists())
	embedded := gjson.GetBytes(out, "client_metadata.x-codex-turn-metadata").String()
	require.Equal(t, "9007199254740993", gjson.Get(embedded, "id").Raw)
	require.Equal(t, codexUSTimezone, gjson.Get(embedded, "timezone").String())
	again, changed, err := normalizeCodexUSLocale(out, account)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, out, again)
}

func TestCodexUSLocaleRawAndPooledWSAgreeWithoutMutation(t *testing.T) {
	enableTestCodexUSLocale(t)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	body := map[string]any{"type": "response.create", "input": []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": localeEnvironment}}}}, "client_metadata": map[string]any{"timezone": "Asia/Shanghai"}}
	before, err := json.Marshal(body)
	require.NoError(t, err)
	clone := map[string]any{}
	for k, v := range body {
		clone[k] = v
	}
	normalizeCodexUSLocaleMap(clone, account)
	actual, err := json.Marshal(clone)
	require.NoError(t, err)
	expected, _, err := normalizeCodexUSLocale(before, account)
	require.NoError(t, err)
	require.JSONEq(t, string(expected), string(actual))
	after, err := json.Marshal(body)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestCodexUSLocaleDisabledAndOtherPlatformsUnchanged(t *testing.T) {
	enableTestCodexUSLocale(t)
	body := []byte(`{"client_metadata":{"timezone":"Asia/Shanghai"}}`)
	for _, a := range []*Account{nil, {Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, {Platform: PlatformAnthropic, Type: AccountTypeOAuth}} {
		out, changed, err := normalizeCodexUSLocale(body, a)
		require.NoError(t, err)
		require.False(t, changed)
		require.Equal(t, body, out)
	}
	codexUSLocaleEnabled.Store(false)
	out, changed, err := normalizeCodexUSLocale(body, &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth})
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, body, out)
}

func TestCodexUSLocaleLeavesQuotedBlocksAndProtocolData(t *testing.T) {
	enableTestCodexUSLocale(t)
	for _, text := range []string{"Example:\n" + localeEnvironment, "```xml\n" + localeEnvironment + "\n```", localeEnvironment + localeEnvironment, "Asia/Shanghai"} {
		require.Equal(t, text, localizeCodexEnvironment(text))
	}
	a := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	for _, s := range []string{`{"type":"response.cancel","client_metadata":{"timezone":"Asia/Shanghai"}}`, `{"input":[{"type":"reasoning","encrypted_content":"Asia/Shanghai"}],"large":9007199254740993}`} {
		out, changed, err := normalizeCodexUSLocale([]byte(s), a)
		require.NoError(t, err)
		require.False(t, changed)
		require.Equal(t, s, string(out))
	}
	_, _, err := normalizeCodexUSLocale([]byte(`{"input":`), a)
	require.Error(t, err)
}

func TestCodexUSLocaleHTTPAndWSHeaders(t *testing.T) {
	enableTestCodexUSLocale(t)
	a := &Account{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"chatgpt_account_id": "test"}}
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	c.Request.Header.Set("Accept-Language", "zh-CN")
	c.Request.Header.Set("X-Timezone", "Asia/Shanghai")
	svc := &OpenAIGatewayService{}
	req, err := svc.buildUpstreamRequest(context.Background(), c, a, []byte(`{"input":[]}`), "dummy", true, "", true)
	require.NoError(t, err)
	ws, _, err := svc.buildOpenAIWSHeaders(context.Background(), c, a, "dummy", OpenAIWSProtocolDecision{}, true, "", `{"timezone":"Asia/Shanghai"}`, "", "", "")
	require.NoError(t, err)
	for _, h := range []http.Header{req.Header, ws} {
		// Native Desktop inference does not emit Accept-Language. Localization
		// still strips timezone hints and rewrites protocol metadata.
		require.Empty(t, h.Get("Accept-Language"))
		require.Empty(t, h.Get("X-Timezone"))
	}
	require.Equal(t, codexUSTimezone, gjson.Get(ws.Get(openAIWSTurnMetadataHeader), "timezone").String())
	require.Equal(t, "zh-CN", c.Request.Header.Get("Accept-Language"))
	h := http.Header{"accept-language": {"zh-CN"}, "Accept-Language": {"zh-TW"}, "X-Timezone": {"Asia/Shanghai"}}
	applyCodexUSLocaleHeaders(h)
	require.Len(t, h, 1)
	require.Equal(t, codexUSLanguage, h.Get("Accept-Language"))
}

func BenchmarkCodexUSLocaleLargeHistory(b *testing.B) {
	prior := codexUSLocaleEnabled.Swap(true)
	defer codexUSLocaleEnabled.Store(prior)
	a := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	env, _ := json.Marshal(localeEnvironment)
	body := []byte(`{"input":[{"role":"user","content":` + string(env) + `},{"type":"function_call_output","output":"` + strings.Repeat("x", 3_000_000) + `"}]}`)
	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := normalizeCodexUSLocale(body, a); err != nil {
			b.Fatal(err)
		}
	}
}

func TestCodexUSLocaleFinalWSMetadataAfterClientHeaderInsertion(t *testing.T) {
	enableTestCodexUSLocale(t)
	a := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	svc := &OpenAIGatewayService{}
	payload := svc.buildOpenAIWSCreatePayload(map[string]any{"input": []any{}}, a)
	setOpenAIWSTurnMetadata(payload, `{"timezone":"Asia/Shanghai","utc_offset":"+08:00","session_id":"keep-session"}`)
	normalizeCodexUSLocaleMetadataMap(payload, a)
	encoded, err := json.Marshal(payload)
	require.NoError(t, err)
	embedded := gjson.GetBytes(encoded, "client_metadata.x-codex-turn-metadata").String()
	require.Equal(t, codexUSTimezone, gjson.Get(embedded, "timezone").String())
	require.False(t, gjson.Get(embedded, "utc_offset").Exists())
	require.Equal(t, "keep-session", gjson.Get(embedded, "session_id").String())
}

func TestCodexUSLocaleAuthAndQuotaHeaders(t *testing.T) {
	enableTestCodexUSLocale(t)
	h := http.Header{"Version": {"stale"}}
	ApplyCodexCanonicalAuthIdentity(h)
	require.Empty(t, h.Get("Accept-Language"), "auth identity must not synthesize locale headers")
	require.Empty(t, h.Get("Version"))
	require.Empty(t, h.Get("X-Codex-Installation-ID"))
	require.NotContains(t, buildCodexCommonHeaders("fixture", "fixture-account", false), "accept-language")
	codexUSLocaleEnabled.Store(false)
	h = make(http.Header)
	ApplyCodexCanonicalAuthIdentity(h)
	require.Empty(t, h.Get("Accept-Language"))
	require.NotContains(t, buildCodexCommonHeaders("fixture", "fixture-account", false), "accept-language")
}

func TestCodexUSLocaleCollapsesAlreadyLocalizedMetadataAliases(t *testing.T) {
	enableTestCodexUSLocale(t)
	h := http.Header{
		"X-Codex-Turn-Metadata": {`{"timezone":"America/New_York","id":9007199254740993}`},
		"x-codex-turn-metadata": {`{"timezone":"Asia/Shanghai"}`},
	}
	applyCodexUSLocaleHeaders(h)
	require.Len(t, h, 1)
	require.NotContains(t, h, "x-codex-turn-metadata")
	require.Equal(t, "9007199254740993", gjson.Get(h.Get(openAIWSTurnMetadataHeader), "id").Raw)
}

func TestCodexUSLocaleAPIKeyHTTPAndWSUnchanged(t *testing.T) {
	enableTestCodexUSLocale(t)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	c.Request.Header.Set("Accept-Language", "zh-CN")
	svc := &OpenAIGatewayService{}
	req, err := svc.buildUpstreamRequest(context.Background(), c, account, []byte(`{"input":[]}`), "fixture", true, "", true)
	require.NoError(t, err)
	ws, _, err := svc.buildOpenAIWSHeaders(context.Background(), c, account, "fixture", OpenAIWSProtocolDecision{}, true, "", "", "", "", "")
	require.NoError(t, err)
	for _, headers := range []http.Header{req.Header, ws} {
		require.Equal(t, "zh-CN", headers.Get("Accept-Language"))
	}
}
