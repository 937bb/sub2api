//go:build unit

package service

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type openAICodexInstallationIDRepoStub struct {
	AccountRepository
	updateExtraCalls int
}

func (r *openAICodexInstallationIDRepoStub) UpdateExtra(_ context.Context, _ int64, _ map[string]any) error {
	r.updateExtraCalls++
	return errors.New("database unavailable")
}

func TestNormalizeOpenAICodexInstallationIDForCreate(t *testing.T) {
	extra := normalizeOpenAICodexInstallationIDForCreate(PlatformOpenAI, AccountTypeOAuth, map[string]any{
		openAICodexInstallationIDExtraKey: "550e8400-e29b-41d4-a716-446655440000",
	})
	installationID, ok := canonicalOpenAICodexInstallationID(openAICodexInstallationIDString(extra[openAICodexInstallationIDExtraKey]))
	require.True(t, ok)
	require.NotEmpty(t, installationID)
	require.NotEqual(t, "550e8400-e29b-41d4-a716-446655440000", installationID)

	untouched := map[string]any{"keep": true}
	require.Equal(t, untouched, normalizeOpenAICodexInstallationIDForCreate(PlatformOpenAI, AccountTypeAPIKey, untouched))
}

func TestNormalizeOpenAICodexInstallationIDForUpdatePreservesServerValue(t *testing.T) {
	previous := "550e8400-e29b-41d4-a716-446655440000"
	updated := normalizeOpenAICodexInstallationIDForUpdate(
		PlatformOpenAI,
		AccountTypeOAuth,
		map[string]any{openAICodexInstallationIDExtraKey: "client-controlled", "keep": true},
		previous,
	)
	require.Equal(t, previous, updated[openAICodexInstallationIDExtraKey])
	require.Equal(t, true, updated["keep"])

	converted := normalizeOpenAICodexInstallationIDForUpdate(
		PlatformOpenAI,
		AccountTypeOAuth,
		map[string]any{openAICodexInstallationIDExtraKey: "client-controlled"},
		"",
	)
	generated, ok := canonicalOpenAICodexInstallationID(openAICodexInstallationIDString(converted[openAICodexInstallationIDExtraKey]))
	require.True(t, ok)
	require.NotEmpty(t, generated)

	apiKey := normalizeOpenAICodexInstallationIDForUpdate(
		PlatformOpenAI,
		AccountTypeAPIKey,
		map[string]any{openAICodexInstallationIDExtraKey: previous, "keep": true},
		previous,
	)
	require.NotContains(t, apiKey, openAICodexInstallationIDExtraKey)
	require.Equal(t, true, apiKey["keep"])
}

func TestWithOpenAICodexInstallationIDIsStableAndDoesNotMutateSharedAccount(t *testing.T) {
	repo := &openAICodexInstallationIDRepoStub{}
	svc := &OpenAIGatewayService{accountRepo: repo}
	account := &Account{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	first := svc.withOpenAICodexInstallationID(context.Background(), account)
	second := svc.withOpenAICodexInstallationID(context.Background(), account)

	require.Empty(t, account.GetOpenAIDeviceID())
	require.NotEmpty(t, first.GetOpenAIDeviceID())
	require.Equal(t, first.GetOpenAIDeviceID(), second.GetOpenAIDeviceID())
	require.Equal(t, 1, repo.updateExtraCalls)
}

func TestOpenAICodexInstallationIDOverridesInboundHTTPAndMatchesBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	c.Request.Header.Set("x-codex-installation-id", "client-controlled")
	installationID := "550e8400-e29b-41d4-a716-446655440000"
	account := &Account{
		ID: 8, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Extra: map[string]any{openAICodexInstallationIDExtraKey: installationID},
	}
	body := map[string]any{}
	require.True(t, applyCodexClientMetadata(body, account))

	req, err := (&OpenAIGatewayService{}).buildUpstreamRequest(
		context.Background(), c, account, []byte(`{"model":"gpt-5.5"}`), "token", true, "", true,
	)
	require.NoError(t, err)
	require.Equal(t, installationID, req.Header.Get("x-codex-installation-id"))
	require.Equal(t, installationID, body["client_metadata"].(map[string]any)["x-codex-installation-id"])
}

func TestOpenAICodexInstallationIDAppliesToSetupTokenWebSocket(t *testing.T) {
	account := &Account{ID: 9, Platform: PlatformOpenAI, Type: AccountTypeSetupToken}
	svc := &OpenAIGatewayService{}

	headers, _, err := svc.buildOpenAIWSHeaders(
		context.Background(), nil, account, "token",
		OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocket},
		false, "", "", "", "gpt-5.5", "",
	)
	require.NoError(t, err)
	installationID, ok := canonicalOpenAICodexInstallationID(headers.Get("x-codex-installation-id"))
	require.True(t, ok)

	payload := svc.buildOpenAIWSCreatePayload(map[string]any{
		"model": "gpt-5.5",
		"client_metadata": map[string]any{
			"x-codex-installation-id": "client-controlled",
		},
	}, cloneAccountWithOpenAICodexInstallationID(account, installationID))
	require.Equal(t, installationID, payload["client_metadata"].(map[string]any)["x-codex-installation-id"])
}
