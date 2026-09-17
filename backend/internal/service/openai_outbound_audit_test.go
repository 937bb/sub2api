package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Verify the outbound environment policy without rewriting user content.
func TestOutboundAudit_HeaderBoundary(t *testing.T) {
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeSetupToken} {
		t.Run(accountType, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			blocked := []string{"X-Sub2API-Quota-Bypass", "X-Forwarded-For", "Forwarded", "Via", "X-Real-IP", "X-Stainless-Lang", "X-Stainless-Package-Version", "Referer", "Cookie"}
			for _, key := range blocked {
				c.Request.Header.Set(key, "audit-sentinel")
			}
			c.Request.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
			account := &Account{ID: 77, Platform: PlatformOpenAI, Type: accountType,
				Credentials: map[string]any{"chatgpt_account_id": "audit-account"},
				Extra:       map[string]any{codexFingerprintModeExtraKey: "off", openAICodexInstallationIDExtraKey: "550e8400-e29b-41d4-a716-446655440000"}}
			svc := &OpenAIGatewayService{}
			body := []byte(`{"model":"gpt-5.5","input":[{"role":"user","content":"hello"}]}`)
			regular, err := svc.buildUpstreamRequest(context.Background(), c, account, body, "dummy-token", true, "", true)
			require.NoError(t, err)
			passthrough, err := svc.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, account, body, "dummy-token")
			require.NoError(t, err)
			ws, _, err := svc.buildOpenAIWSHeaders(context.Background(), c, account, "dummy-token", OpenAIWSProtocolDecision{}, true, "", "", "", "gpt-5.5", "")
			require.NoError(t, err)
			normalizeCodexResponsesTransportHeaders(regular, account)
			normalizeCodexResponsesTransportHeaders(passthrough, account)
			for _, headers := range []http.Header{regular.Header, passthrough.Header, ws} {
				for _, key := range blocked {
					require.Empty(t, headers.Get(key), key)
				}
				require.Empty(t, headers.Get("Accept-Language"))
				require.Empty(t, headers.Get("version"))
				require.Empty(t, headers.Get("session_id"))
				require.Empty(t, headers.Get("conversation_id"))
			}
		})
	}
}

func TestOutboundAudit_UnrecognizedMetadataIsPreserved(t *testing.T) {
	account := &Account{ID: 77, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Credentials: map[string]any{"chatgpt_account_id": "audit-account"},
		Extra:       map[string]any{codexFingerprintModeExtraKey: "full", codexFingerprintSeedExtraKey: testCodexFingerprintSeed}}
	body := []byte(`{"model":"gpt-5.5","instructions":"Keep user instructions","input":[{"role":"user","content":"<environment_context>timezone: Asia/Shanghai; cwd: /workspace/sub2api</environment_context>"}],"client_metadata":{"session_id":"client-a","timezone":"Asia/Shanghai","client_name":"audit-client","x-codex-turn-metadata":"{\"thread_source\":\"audit-source\",\"timezone\":\"Asia/Shanghai\"}"}}`)
	ids := resolveCodexFingerprintIDs(account, "client-a", codexFingerprintFull)
	require.NotNil(t, ids)
	next, _, err := applyCodexFingerprintClientMetadataRaw(body, ids)
	require.NoError(t, err)
	next, _, err = applyCodexAccountIdentityClientMetadataRaw(next, account, 91)
	require.NoError(t, err)
	require.Equal(t, codexClientTimezone, gjson.GetBytes(next, "client_metadata.timezone").String())
	require.Equal(t, "audit-client", gjson.GetBytes(next, "client_metadata.client_name").String())
	embedded := gjson.Parse(gjson.GetBytes(next, "client_metadata.x-codex-turn-metadata").String())
	require.Equal(t, codexClientTimezone, embedded.Get("timezone").String())
	require.Equal(t, "audit-source", embedded.Get("thread_source").String())
	require.Equal(t, gjson.GetBytes(body, "input").Raw, gjson.GetBytes(next, "input").Raw)
}

// The legacy primitive still supports deterministic account-test probes. Real
// forwarding must use resolveCodexIsolatedFingerprintForRequest instead.
func TestOutboundAudit_LegacyFullPrimitiveRequiresConversationIsolation(t *testing.T) {
	account := &Account{ID: 77, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Extra: map[string]any{codexFingerprintModeExtraKey: "full", codexFingerprintSeedExtraKey: testCodexFingerprintSeed}}
	a := resolveCodexFingerprintIDs(account, "client-a", codexFingerprintFull)
	b := resolveCodexFingerprintIDs(account, "client-b", codexFingerprintFull)
	require.NotNil(t, a)
	require.NotNil(t, b)
	require.Equal(t, a.sessionID, b.sessionID)
	require.Equal(t, a.threadID, b.threadID)
	require.NotEqual(t, a.turnID, b.turnID)
}
