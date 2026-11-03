package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func accountDeviceFixture(id int64, upstream string) *Account {
	return &Account{ID: id, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Credentials: map[string]any{"chatgpt_account_id": upstream, "access_token": "initial-token"}}
}

func TestAccountDeviceIndependentOfDownstreamClientAndTokenRefresh(t *testing.T) {
	a := accountDeviceFixture(11, "upstream-a")
	want := codexAccountInstallationID(a)
	_, err := uuid.Parse(want)
	require.NoError(t, err)
	for _, apiKeyID := range []int64{0, 77, 99} {
		for _, input := range []string{"", "client-a", "client-b", want} {
			require.Equal(t, want, scopeCodexAccountIdentityValue(a, apiKeyID, "installation", input))
		}
	}
	// Reconstructing a database row and refreshing its bearer cannot rotate it.
	reloaded := accountDeviceFixture(11, "upstream-a")
	reloaded.Credentials["access_token"] = "refreshed-token"
	require.Equal(t, want, codexAccountInstallationID(reloaded))
	require.Equal(t, want, codexAccountInstallationID(accountDeviceFixture(22, "upstream-a")))
	require.NotEqual(t, want, codexAccountInstallationID(accountDeviceFixture(33, "upstream-b")))
	require.NotEqual(t, scopeCodexAccountIdentityValue(a, 77, "session", "same"), scopeCodexAccountIdentityValue(a, 99, "session", "same"))
	require.NotEqual(t, scopeCodexAccountIdentityValue(a, 77, "thread", "a"), scopeCodexAccountIdentityValue(a, 77, "thread", "b"))
}

func TestAccountDevicePreservesPersistedAndExplicitIdentity(t *testing.T) {
	a := accountDeviceFixture(11, "upstream-a")
	a.Extra = map[string]any{codexFingerprintSeedExtraKey: testCodexFingerprintSeed}
	require.Equal(t, resolveConvergedInstallationID(a, testCodexFingerprintSeed), codexAccountInstallationID(a))
	a.Extra["openai_device_id"] = "explicit-installation"
	require.Equal(t, "explicit-installation", codexAccountInstallationID(a))
	for _, account := range []*Account{nil, {ID: 11, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, {ID: 11, Platform: PlatformOpenAI, Type: AccountTypeOAuth}} {
		require.Empty(t, codexAccountInstallationID(account), "never synthesize identity from a local row ID alone")
	}
}

func TestAccountDeviceShadowConvergenceUsesCredentialOwner(t *testing.T) {
	parent := accountDeviceFixture(11, "upstream-a")
	parent.Extra = map[string]any{"openai_device_id": "parent-device"}
	shadow := accountDeviceFixture(22, "upstream-a")
	shadow.ParentAccountID = &parent.ID
	shadow.Extra = map[string]any{codexFingerprintModeExtraKey: "session", codexFingerprintSeedExtraKey: testCodexFingerprintSeed}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	svc := &OpenAIGatewayService{accountRepo: &codexAccountIdentityRepoStub{account: parent}}
	_, err := svc.prepareCodexAccountIdentitySource(context.Background(), c, shadow)
	require.NoError(t, err)
	ids := resolveCodexFingerprintIDsForAttempt(c, shadow, make(http.Header))
	require.NotNil(t, ids)
	require.Equal(t, shadow.ID, ids.accountID)
	require.Equal(t, "parent-device", ids.installationID)
	stageCodexFingerprintIDs(c, ids)
	h := make(http.Header)
	applyCodexAccountIdentityHeaders(h, parent, 77)
	applyStagedCodexFingerprintHeaders(c, shadow, h)
	require.Equal(t, "parent-device", h.Get("x-codex-installation-id"))
}

func TestAccountDeviceHeaderAndMetadataParity(t *testing.T) {
	a := accountDeviceFixture(11, "upstream-a")
	want := codexAccountInstallationID(a)
	for _, input := range []string{
		`{"input":[],"client_metadata":{}}`,
		`{"input":[],"client_metadata":{"installation_id":"client-a","x-codex-installation-id":"client-b","session_id":"session-a","x-codex-turn-metadata":"{\"installation_id\":\"client-c\",\"thread_id\":\"thread-a\"}"}}`,
	} {
		var body map[string]any
		require.NoError(t, json.Unmarshal([]byte(input), &body))
		require.True(t, applyCodexAccountIdentityClientMetadataMap(body, a, 77))
		raw, changed, err := applyCodexAccountIdentityClientMetadataRaw([]byte(input), a, 77)
		require.NoError(t, err)
		require.True(t, changed)
		encoded, err := json.Marshal(body)
		require.NoError(t, err)
		require.JSONEq(t, string(encoded), string(raw))
		metadata := body["client_metadata"].(map[string]any)
		require.Equal(t, want, metadata["installation_id"])
		require.Equal(t, want, metadata["x-codex-installation-id"])
		if embedded, ok := metadata[openAIWSTurnMetadataHeader].(string); ok {
			var fields map[string]any
			require.NoError(t, json.Unmarshal([]byte(embedded), &fields))
			require.Equal(t, want, fields["installation_id"])
		}
	}
	h := http.Header{"x-codex-installation-id": {"client-a"}, "X-CODEX-INSTALLATION-ID": {"client-b"}}
	applyCodexAccountIdentityHeaders(h, a, 99)
	require.Len(t, h, 1)
	require.Equal(t, want, h.Get("x-codex-installation-id"))
	applyCodexAccountIdentityHeaders(h, a, 77)
	require.Equal(t, want, h.Get("x-codex-installation-id"))
	// Compact and other payloads without metadata retain their body shape.
	raw, changed, err := applyCodexAccountIdentityClientMetadataRaw([]byte(`{"input":[]}`), a, 77)
	require.NoError(t, err)
	require.False(t, changed)
	require.JSONEq(t, `{"input":[]}`, string(raw))
}

func TestAccountDeviceHTTPAndWSBuildersIncludingPrewarm(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a := accountDeviceFixture(11, "upstream-a")
	svc := &OpenAIGatewayService{}
	for _, apiKeyID := range []int64{77, 99} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		c.Set("api_key", &APIKey{ID: apiKeyID})
		c.Request.Header.Set("x-codex-installation-id", uuid.NewString())
		req, err := svc.buildUpstreamRequest(context.Background(), c, a, []byte(`{"input":[]}`), "token", true, "session", true)
		require.NoError(t, err)
		require.Equal(t, codexAccountInstallationID(a), req.Header.Get("x-codex-installation-id"))
		for _, ctx := range []*gin.Context{c, nil} {
			h, _, err := svc.buildOpenAIWSHeaders(context.Background(), ctx, a, "token", OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2}, true, "", "", "", "", "")
			require.NoError(t, err)
			require.Equal(t, req.Header.Get("x-codex-installation-id"), h.Get("x-codex-installation-id"))
		}
	}
}

func TestAccountDeviceChangePreventsWSConnectionReuseWithoutSessionConvergence(t *testing.T) {
	pool := newOpenAIWSConnPool(nil)
	t.Cleanup(pool.Close)
	dialer := &openAIWSCountingDialer{}
	pool.setClientDialerForTest(dialer)
	a := accountDeviceFixture(11, "upstream-a")
	acquire := func(device string) *openAIWSConnLease {
		h := make(http.Header)
		h.Set("x-codex-installation-id", device)
		lease, err := pool.Acquire(context.Background(), openAIWSAcquireRequest{Account: a, WSURL: "wss://example.com/v1/responses", Headers: h})
		require.NoError(t, err)
		return lease
	}
	first := acquire(codexAccountInstallationID(a))
	firstID := first.ConnID()
	first.Release()
	second := acquire(codexAccountInstallationID(a))
	require.Equal(t, firstID, second.ConnID())
	second.Release()
	a.Extra = map[string]any{"openai_device_id": "replacement-device"}
	third := acquire(codexAccountInstallationID(a))
	require.NotEqual(t, firstID, third.ConnID())
	third.Release()
}
