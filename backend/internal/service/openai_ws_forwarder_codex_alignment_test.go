package service

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIWSHeadersOAuthAddsCodexIdentityFallbacks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "opencode/0.9")
	c.Request.Header.Set("originator", "opencode")

	svc := &OpenAIGatewayService{}
	account := &Account{
		ID:          43,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"chatgpt_account_id": "chatgpt-acc"},
	}
	fallbackSessionID := fallbackOpenAICodexSessionID(c, account, []byte(`{"model":"gpt-5","input":"hello"}`))

	headers, resolution, err := svc.buildOpenAIWSHeaders(
		c,
		account,
		"token",
		OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2},
		openai.IsCodexOfficialClientByHeaders(c.GetHeader("User-Agent"), c.GetHeader("originator")),
		"",
		"",
		"",
		fallbackSessionID,
	)
	require.NoError(t, err)

	require.NotEmpty(t, resolution.SessionID)
	require.Equal(t, "fallback_session_id", resolution.SessionSource)
	require.Equal(t, "fallback_session_id", resolution.ThreadSource)
	require.Equal(t, codexOfficialOriginator, headers.Get("originator"))
	require.Equal(t, codexCLIVersion, headers.Get("Version"))
	require.Equal(t, codexCLIUserAgent, headers.Get("User-Agent"))
	require.Equal(t, openAIWSBetaV2Value, headers.Get("OpenAI-Beta"))
	require.NotEmpty(t, headers.Get(openAICodexSessionIDHeader))
	require.NotEmpty(t, headers.Get(openAICodexThreadIDHeader))
	require.Equal(t, headers.Get(openAICodexThreadIDHeader), headers.Get(openAICodexClientRequestIDHeader))
	require.NotEmpty(t, headers.Get(openAICodexInstallationIDHeader))
	require.NotEmpty(t, headers.Get(openAICodexWindowIDHeader))
}

func TestOpenAIWSHeadersOAuthNilContextUsesCodexUserAgent(t *testing.T) {
	account := &Account{
		ID:       44,
		Platform: PlatformOpenAI,
		Type:     AccountTypeSetupToken,
	}
	svc := &OpenAIGatewayService{}

	require.NotPanics(t, func() {
		headers, _, err := svc.buildOpenAIWSHeaders(
			nil,
			account,
			"token",
			OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2},
			false,
			"",
			"",
			"",
			"fallback-session",
		)
		require.NoError(t, err)
		require.Equal(t, codexCLIUserAgent, headers.Get("User-Agent"))
	})
}

func TestOpenAIWSHeadersOAuthMigratesLegacyBuiltInFingerprint(t *testing.T) {
	repo := &openAICodexFingerprintRepoStub{}
	legacy := OpenAICodexFingerprint{SchemaVersion: 1, InstallationID: "550e8400-e29b-41d4-a716-446655440000", UAProfile: legacyBuiltInOpenAICodexUAProfile, CreatedAt: "2026-06-12T00:00:00Z", UpdatedAt: "2026-06-13T00:00:00Z"}
	account := &Account{ID: 45, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{OpenAICodexFingerprintExtraKey: legacy}}
	svc := &OpenAIGatewayService{codexFingerprintService: NewOpenAICodexFingerprintService(repo, nil)}
	headers, _, err := svc.buildOpenAIWSHeaders(nil, account, "token", OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2}, false, "", "", "", "fallback-session")
	require.NoError(t, err)
	require.Equal(t, codexCLIUserAgent, headers.Get("User-Agent"))
	require.Equal(t, codexCLIVersion, headers.Get("Version"))
	require.Equal(t, legacy.InstallationID, headers.Get(openAICodexInstallationIDHeader))
	require.Len(t, repo.updates, 1)
	_, _, err = svc.buildOpenAIWSHeaders(nil, account, "token", OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2}, false, "", "", "", "fallback-session")
	require.NoError(t, err)
	require.Len(t, repo.updates, 1)
}

func TestOpenAIWSHeadersOAuthUsesPersistedFingerprintIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "inbound/9.9")
	c.Request.Header.Set("originator", "inbound")
	c.Request.Header.Set(openAICodexInstallationIDHeader, "11111111-1111-4111-8111-111111111111")

	persisted := OpenAICodexFingerprint{
		SchemaVersion:  openAICodexFingerprintSchemaV1,
		InstallationID: "550e8400-e29b-41d4-a716-446655440000",
		UAProfile:      ParseOpenAICodexUAProfile("persisted-codex/9.9.9 (Persist OS; arch) Persist_Term/1.0 (persisted-codex; 9.9.9)"),
		CreatedAt:      "2026-06-12T00:00:00Z",
		UpdatedAt:      "2026-06-12T00:00:00Z",
	}
	svc := &OpenAIGatewayService{}
	account := &Account{
		ID:       45,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{OpenAICodexFingerprintExtraKey: persisted},
	}

	headers, _, err := svc.buildOpenAIWSHeaders(
		c,
		account,
		"token",
		OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2},
		false,
		"",
		"",
		"",
		"fallback-session",
	)

	require.NoError(t, err)
	require.Equal(t, persisted.InstallationID, headers.Get(openAICodexInstallationIDHeader))
	require.Equal(t, persisted.UAProfile.UserAgent(), headers.Get("User-Agent"))
	require.Equal(t, "persisted-codex", headers.Get("originator"))
	require.Equal(t, "9.9.9", headers.Get("Version"))
}

func TestOpenAIWSHeadersAPIKeyKeepsLegacyUserAgentSemantics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "inbound-client/1.0")

	svc := &OpenAIGatewayService{}
	account := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"user_agent": "api-key-custom/2.0"},
		Extra:       map[string]any{OpenAICodexFingerprintExtraKey: "must-not-be-read"},
	}

	headers, _, err := svc.buildOpenAIWSHeaders(
		c,
		account,
		"token",
		OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2},
		false,
		"",
		"",
		"",
		"fallback-session",
	)

	require.NoError(t, err)
	require.Equal(t, "api-key-custom/2.0", headers.Get("User-Agent"))
	require.NotEqual(t, "must-not-be-read", headers.Get(openAICodexInstallationIDHeader))
	require.NotEqual(t, codexCLIUserAgent, headers.Get("User-Agent"))
}

func TestOpenAIWSCodexClientMetadataIncludesRequestStart(t *testing.T) {
	// 对齐 Codex build_ws_client_metadata + response_create_client_metadata 的 key 集合。
	payload := map[string]any{"type": "response.create", "model": "gpt-5"}
	headers := http.Header{}
	headers.Set(openAICodexInstallationIDHeader, "installation-1")
	headers.Set(openAICodexWindowIDHeader, "thread-1:0")
	headers.Set(openAICodexTurnMetadataHeader, "turn=v1")
	headers.Set(openAITraceparentHeader, "00-trace-id")
	headers.Set(openAITracestateHeader, "vendor=value")

	setOpenAIWSCodexClientMetadata(payload, headers)

	metadata, ok := payload["client_metadata"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "installation-1", metadata[openAICodexInstallationIDHeader])
	require.Equal(t, "thread-1:0", metadata[openAICodexWindowIDHeader])
	require.Equal(t, "turn=v1", metadata[openAICodexTurnMetadataHeader])
	// traceparent/tracestate 在 client_metadata 中映射为 ws_request_header_* key。
	require.Equal(t, "00-trace-id", metadata[openAICodexWSTraceparentMetadataKey])
	require.Equal(t, "vendor=value", metadata[openAICodexWSTracestateMetadataKey])
	// thread-id 不放 client_metadata（Codex 仅放在 HTTP header 中）。
	require.NotContains(t, metadata, openAICodexThreadIDHeader)
	startMS, ok := metadata[openAICodexWSStreamRequestStartMSKey].(string)
	require.True(t, ok)
	parsedStartMS, err := strconv.ParseInt(startMS, 10, 64)
	require.NoError(t, err)
	require.Positive(t, parsedStartMS)
}

func TestOpenAIWSCodexClientMetadataStripsClientSuppliedIdentity(t *testing.T) {
	payload := map[string]any{
		"type":  "response.create",
		"model": "gpt-5",
		"client_metadata": map[string]any{
			"keep":                               "yes",
			openAICodexInstallationIDHeader:      "attacker-installation",
			openAICodexWindowIDHeader:            "attacker-window",
			openAICodexSessionIDHeader:           "attacker-session-header",
			openAICodexThreadIDHeader:            "attacker-thread-header",
			openAICodexClientRequestIDHeader:     "attacker-request-header",
			openAICodexParentThreadIDHeader:      "attacker-parent-thread",
			openAICodexTurnStateHeader:           "attacker-turn-state",
			openAICodexTurnMetadataHeader:        "attacker-turn-metadata",
			openAICodexSubagentHeader:            "attacker-subagent",
			openAICodexWSTraceparentMetadataKey:  "attacker-traceparent",
			openAICodexWSTracestateMetadataKey:   "attacker-tracestate",
			openAICodexWSStreamRequestStartMSKey: "attacker-start-ms",
			"ws_request_header_authorization":    "attacker-auth",
			"ws_request_header_cookie":           "attacker-cookie",
			"session_id":                         "attacker-session",
			"conversation_id":                    "attacker-conversation",
			"prompt_cache_key":                   "attacker-cache-key",
		},
	}
	headers := http.Header{}
	headers.Set(openAICodexInstallationIDHeader, "server-installation")
	headers.Set(openAICodexWindowIDHeader, "server-thread:3")

	setOpenAIWSCodexClientMetadata(payload, headers)

	metadata, ok := payload["client_metadata"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "yes", metadata["keep"])
	require.Equal(t, "server-installation", metadata[openAICodexInstallationIDHeader])
	require.Equal(t, "server-thread:3", metadata[openAICodexWindowIDHeader])
	require.NotContains(t, metadata, openAICodexSessionIDHeader)
	require.NotContains(t, metadata, openAICodexThreadIDHeader)
	require.NotContains(t, metadata, openAICodexClientRequestIDHeader)
	require.NotContains(t, metadata, openAICodexParentThreadIDHeader)
	require.NotContains(t, metadata, openAICodexTurnStateHeader)
	require.NotContains(t, metadata, openAICodexTurnMetadataHeader)
	require.NotContains(t, metadata, openAICodexSubagentHeader)
	require.NotContains(t, metadata, openAICodexWSTraceparentMetadataKey)
	require.NotContains(t, metadata, openAICodexWSTracestateMetadataKey)
	require.NotContains(t, metadata, "ws_request_header_authorization")
	require.NotContains(t, metadata, "ws_request_header_cookie")
	require.NotEqual(t, "attacker-start-ms", metadata[openAICodexWSStreamRequestStartMSKey])
	require.NotContains(t, metadata, "session_id")
	require.NotContains(t, metadata, "conversation_id")
	require.NotContains(t, metadata, "prompt_cache_key")
}
