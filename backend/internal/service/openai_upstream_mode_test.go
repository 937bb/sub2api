package service

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func openAIUpstreamModeTestContext(mode string) *gin.Context {
	c, _ := gin.CreateTestContext(nil)
	SetOpenAIUpstreamMode(c, mode)
	return c
}

func TestOpenAIUpstreamModeDefaultsToCodex(t *testing.T) {
	require.Equal(t, OpenAIUpstreamModeCodex, NormalizeOpenAIUpstreamMode(""))
	require.Equal(t, OpenAIUpstreamModeCodex, NormalizeOpenAIUpstreamMode("invalid"))
	require.Equal(t, OpenAIUpstreamModeWord, NormalizeOpenAIUpstreamMode(" WORD "))
	require.Equal(t, OpenAIUpstreamModeCodex, OpenAIUpstreamModeForGroup(nil))
	require.Equal(t, OpenAIUpstreamModeCodex, OpenAIUpstreamModeForGroup(&Group{Platform: PlatformAnthropic, OpenAIUpstreamMode: OpenAIUpstreamModeWord}))
}

func TestBuildOpenAIResponsesWSURLUsesWordOnlyForOAuthLikeAccounts(t *testing.T) {
	svc := &OpenAIGatewayService{}
	c := openAIUpstreamModeTestContext(OpenAIUpstreamModeWord)

	for _, accountType := range []string{AccountTypeOAuth, AccountTypeSetupToken} {
		account := &Account{Platform: PlatformOpenAI, Type: accountType}
		got, err := svc.buildOpenAIResponsesWSURL(c, account)
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(got, openAIWordResponsesWSURL+"?"))
	}

	apiKey := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	got, err := svc.buildOpenAIResponsesWSURL(c, apiKey)
	require.NoError(t, err)
	require.Equal(t, "wss://api.openai.com/v1/responses", got)
}

func TestApplyOpenAIWordWSHeadersUsesBearerSubprotocol(t *testing.T) {
	c := openAIUpstreamModeTestContext(OpenAIUpstreamModeWord)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	headers := http.Header{
		"Authorization":           []string{"Bearer old"},
		"Openai-Beta":             []string{"responses_websockets=2026-02-06"},
		"Originator":              []string{"codex_cli_rs"},
		"X-Codex-Installation-Id": []string{"device"},
	}

	applyOpenAIWordWSHeaders(c, account, "jwt.token.value", headers)

	require.Empty(t, headers.Get("Authorization"))
	require.Empty(t, headers.Get("OpenAI-Beta"))
	require.Empty(t, headers.Get("Originator"))
	require.Empty(t, headers.Get("X-Codex-Installation-Id"))
	require.Equal(t, "https://bps.openai.com", headers.Get("Origin"))
	require.Equal(t, []string{"responses", "openai-bearer.jwt.token.value"}, headers.Values("Sec-WebSocket-Protocol"))
	cleaned, protocols := extractOpenAIWSSubprotocols(headers)
	require.Empty(t, cleaned.Values("Sec-WebSocket-Protocol"))
	require.Equal(t, []string{"responses", "openai-bearer.jwt.token.value"}, protocols)
	require.Equal(t, "responses,openai-bearer", normalizeOpenAIWSSubprotocolsForCompatibility(headers))
}

func TestApplyOpenAIWordInstructionsOverridesDefaultPromptAndPreservesPayload(t *testing.T) {
	body := []byte(`{"model":"gpt-6-astra","instructions":"Keep the existing policy.","tools":[{"type":"function","name":"lookup"}]}`)

	got, err := applyOpenAIWordInstructions(body)
	require.NoError(t, err)
	require.Equal(t, "gpt-6-astra", gjson.GetBytes(got, "model").String())
	require.Equal(t, openAIWordInstruction, gjson.GetBytes(got, "instructions").String())
	require.NotContains(t, gjson.GetBytes(got, "instructions").String(), "Keep the existing policy.")
	require.Equal(t, "lookup", gjson.GetBytes(got, "tools.0.name").String())

	again, err := applyOpenAIWordInstructions(got)
	require.NoError(t, err)
	require.Equal(t, string(got), string(again), "the compatibility instruction must not be duplicated")
}

func TestBuildOpenAIWordResponsesWSURLCarriesWordClientIdentity(t *testing.T) {
	got, err := buildOpenAIWordResponsesWSURL()
	require.NoError(t, err)
	parsed, err := url.Parse(got)
	require.NoError(t, err)
	require.Equal(t, "wss", parsed.Scheme)
	require.Equal(t, "bps.openai.com", parsed.Host)
	require.Equal(t, "/basispoints/api/responses", parsed.Path)
	require.Equal(t, "chatgpt", parsed.Query().Get("bps_auth_mode"))
	require.Contains(t, parsed.Query().Get("bps_ws_affinity"), "affinity_")
	require.Equal(t, "upstream_sent,heartbeat,resume,cancel,server_draining", parsed.Query().Get("bps_control_frames"))
	info := parsed.Query().Get("bps_client_info")
	require.Equal(t, "basispoints-word-plugin", gjson.Get(info, "X-Openai-Internal-Basispoints-Client-Product").String())
	require.Equal(t, "word", gjson.Get(info, "X-Openai-Internal-Basispoints-Client-Platform").String())
	require.Equal(t, "Word", gjson.Get(info, "X-Openai-Internal-Basispoints-Office-Host").String())
}

func TestApplyOpenAIWordCreatePayloadUsesWordFrame(t *testing.T) {
	c := openAIUpstreamModeTestContext(OpenAIUpstreamModeWord)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	svc := &OpenAIGatewayService{}
	payload := svc.buildOpenAIWSCreatePayloadForRequest(c, map[string]any{
		"model":        "gpt-6-astra",
		"instructions": openAIWordInstruction,
		"tools":        []any{map[string]any{"type": "function", "name": "lookup"}},
		"input": []any{map[string]any{
			"role":    "user",
			"content": []any{map[string]any{"type": "input_text", "text": "Reply with only 21."}},
		}},
	}, account)

	require.Equal(t, "response.create", payload["type"])
	require.Equal(t, true, payload["stream"])
	require.Equal(t, false, payload["store"])
	require.Equal(t, "explicit", payload["model_selection"])
	require.NotContains(t, payload, "instructions")
	require.NotContains(t, payload, "tools")
	require.Contains(t, payload["basispoints_resume_token"], "resume_")
	input := payload["input"].([]any)
	require.Equal(t, "developer", input[0].(map[string]any)["role"])
	require.Equal(t, "user", input[1].(map[string]any)["role"])
	require.Equal(t, "Reply with only 21.", input[1].(map[string]any)["content"].([]any)[0].(map[string]any)["text"])
}
