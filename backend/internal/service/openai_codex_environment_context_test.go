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
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const codexEnvironmentFixture = "<environment_context>\n  <cwd>/workspace/sub2api</cwd>\n  <shell>zsh</shell>\n  <current_date>2026-09-15</current_date>\n  <timezone>Asia/Shanghai</timezone>\n  <filesystem><workspace_roots><root>/workspace/sub2api</root></workspace_roots></filesystem>\n</environment_context>"

func TestCodexEnvironmentContextXML(t *testing.T) {
	for _, original := range []string{
		codexEnvironmentFixture,
		"\n" + codexEnvironmentFixture + "\n",
		codexEnvironmentFixture + "\n" + codexEnvironmentFixture,
		"<environment_context><timezone>Asia/Shanghai</timezone></environment_context>",
		"<environment_context><environments><environment id=\"remote\"><cwd>/repo&amp;tools</cwd></environment></environments><timezone>Asia/Shanghai</timezone></environment_context>",
	} {
		want := strings.ReplaceAll(original, "<timezone>Asia/Shanghai</timezone>", "<timezone>America/Los_Angeles</timezone>")
		require.Equal(t, want, normalizeCodexEnvironmentContext(original))
		require.Equal(t, want, normalizeCodexEnvironmentContext(want))
	}
	for _, original := range []string{
		"```xml\n" + codexEnvironmentFixture + "\n```",
		codexEnvironmentFixture + " please explain this example",
		"<environment_context><cwd>/repo</cwd></environment_context>",
		"<environment_context><timezone></timezone></environment_context>",
		"<environment_context><timezone><value>Asia/Shanghai</value></timezone></environment_context>",
		"<environment_context><timezone>Asia/Shanghai</timezone>",
		"<environment_context><timezone>Asia/Shanghai</timezone></invalid>",
		"<environment_context><note><timezone>Asia/Shanghai</timezone></note></environment_context>",
		"<environment_context>timezone: Asia/Shanghai; cwd: /workspace/sub2api</environment_context>",
	} {
		require.Equal(t, original, normalizeCodexEnvironmentContext(original))
	}
	require.Contains(t, normalizeCodexEnvironmentContext("Explain "+codexEnvironmentFixture), codexClientTimezone)
}

func TestCodexEnvironmentContextRawMapParityAndOpaqueItems(t *testing.T) {
	encoded, err := json.Marshal(codexEnvironmentFixture)
	require.NoError(t, err)
	body := []byte(`{"input":[{"role":"user","content":[{"type":"input_text","text":` + string(encoded) + `},{"type":"input_image","image_url":"data:image/png;base64,AAAA"}]},{"role":"developer","content":` + string(encoded) + `},{"role":"assistant","content":` + string(encoded) + `},{"type":"function_call_output","call_id":"call_1","output":` + string(encoded) + `},{"type":"custom_tool_call","input":` + string(encoded) + `,"counter":9007199254740993}],"instructions":` + string(encoded) + `,"client_metadata":{"session_id":"client-session","counter":9007199254740993}}`)
	for _, kind := range []string{AccountTypeOAuth, AccountTypeSetupToken} {
		account := &Account{ID: 77, Platform: PlatformOpenAI, Type: kind}
		next, changed, err := applyCodexClientEnvironmentRaw(body, account)
		require.NoError(t, err)
		require.True(t, changed)
		want := strings.ReplaceAll(codexEnvironmentFixture, "Asia/Shanghai", codexClientTimezone)
		for _, path := range []string{"input.0.content.0.text", "input.1.content", "instructions"} {
			require.Equal(t, want, gjson.GetBytes(next, path).String())
		}
		for _, path := range []string{"input.0.content.1", "input.2", "input.3", "input.4", "client_metadata"} {
			require.Equal(t, gjson.GetBytes(body, path).Raw, gjson.GetBytes(next, path).Raw, path)
		}
		var decoded map[string]any
		require.NoError(t, decodeOpenAIJSONUseNumber(body, &decoded))
		require.True(t, applyCodexClientEnvironmentMap(decoded, account))
		mapBody, err := json.Marshal(decoded)
		require.NoError(t, err)
		require.JSONEq(t, string(next), string(mapBody))
		again, changed, err := applyCodexClientEnvironmentRaw(next, account)
		require.NoError(t, err)
		require.False(t, changed)
		require.Equal(t, next, again)
	}
	for _, account := range []*Account{nil, {Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, {Platform: PlatformAnthropic, Type: AccountTypeOAuth}} {
		next, changed, err := applyCodexClientEnvironmentRaw(body, account)
		require.NoError(t, err)
		require.False(t, changed)
		require.Equal(t, body, next)
	}
}

func TestCodexEnvironmentContextRawStringAndMalformedJSON(t *testing.T) {
	encoded, err := json.Marshal(codexEnvironmentFixture)
	require.NoError(t, err)
	// Standard Go escaping of XML brackets must be recognized on the wire.
	require.Contains(t, string(encoded), `\u003c`)
	for _, key := range []string{"input", "instructions"} {
		body := []byte(`{"` + key + `":` + string(encoded) + `}`)
		next, changed := applyCodexEnvironmentContextRaw(body)
		require.True(t, changed)
		require.Contains(t, gjson.GetBytes(next, key).String(), "<timezone>America/Los_Angeles</timezone>")
		for _, invalid := range [][]byte{append(append([]byte(nil), body...), []byte(` {}`)...), body[:len(body)-1]} {
			next, changed := applyCodexEnvironmentContextRaw(invalid)
			require.False(t, changed)
			require.Equal(t, invalid, next)
		}
	}
}

func TestCodexEnvironmentContextOutboundHTTPCompactAndWS(t *testing.T) {
	encoded, err := json.Marshal(codexEnvironmentFixture)
	require.NoError(t, err)
	for _, kind := range []string{AccountTypeOAuth, AccountTypeSetupToken} {
		account := &Account{ID: 77, Platform: PlatformOpenAI, Type: kind,
			Credentials: map[string]any{"chatgpt_account_id": "test-account"},
			Extra:       map[string]any{codexFingerprintModeExtraKey: "off"}}
		for _, path := range []string{"/v1/responses", "/v1/responses/compact"} {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, path, nil)
			svc := &OpenAIGatewayService{}
			// A metadata-free request must still normalize its model-visible XML.
			body := []byte(`{"model":"gpt-5.5","input":[{"role":"user","content":` + string(encoded) + `}]}`)
			regular, err := svc.buildUpstreamRequest(context.Background(), c, account, body, "dummy-token", true, "", true)
			require.NoError(t, err)
			passthrough, err := svc.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, account, body, "dummy-token")
			require.NoError(t, err)
			for _, req := range []*http.Request{regular, passthrough} {
				sent, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				require.NotContains(t, gjson.GetBytes(sent, "input.0.content").String(), "Asia/Shanghai")
				require.Contains(t, gjson.GetBytes(sent, "input.0.content").String(), "<timezone>America/Los_Angeles</timezone>")
			}
			wsBody, changed, err := applyCodexAccountIdentityClientMetadataRaw(body, account, 42)
			require.NoError(t, err)
			require.True(t, changed)
			wsBody, err = svc.applyCodexFingerprintToWebSocketPayload(context.Background(), c, account, wsBody)
			require.NoError(t, err)
			require.Contains(t, gjson.GetBytes(wsBody, "input.0.content").String(), "<timezone>America/Los_Angeles</timezone>")
		}
	}
}
