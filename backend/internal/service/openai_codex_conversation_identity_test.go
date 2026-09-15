package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func conversationTestContext(keyID int64, session string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("session-id", session)
	c.Set("api_key", &APIKey{ID: keyID})
	return c
}

func conversationTestAccount(accountType, mode string) *Account {
	return &Account{ID: 77, Platform: PlatformOpenAI, Type: accountType,
		Credentials: map[string]any{"chatgpt_account_id": "upstream-77"},
		Extra:       map[string]any{codexFingerprintModeExtraKey: mode, codexFingerprintSeedExtraKey: testCodexFingerprintSeed}}
}

func TestCodexConversationIdentity_IsolationAndStability(t *testing.T) {
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeSetupToken} {
		for _, mode := range []string{"session", "full"} {
			t.Run(accountType+"/"+mode, func(t *testing.T) {
				svc := &OpenAIGatewayService{}
				account := conversationTestAccount(accountType, mode)
				a := svc.resolveCodexIsolatedFingerprintForRequest(context.Background(), conversationTestContext(1, "chat-a"), account, nil)
				again := svc.resolveCodexIsolatedFingerprintForRequest(context.Background(), conversationTestContext(1, "chat-a"), account, nil)
				chatB := svc.resolveCodexIsolatedFingerprintForRequest(context.Background(), conversationTestContext(1, "chat-b"), account, nil)
				tenantB := svc.resolveCodexIsolatedFingerprintForRequest(context.Background(), conversationTestContext(2, "chat-a"), account, nil)
				require.NotNil(t, a)
				require.Equal(t, a.sessionID, again.sessionID)
				require.Equal(t, a.threadID, again.threadID)
				require.NotEqual(t, a.turnID, again.turnID)
				for _, other := range []*codexFingerprintIDs{chatB, tenantB} {
					require.Equal(t, a.installationID, other.installationID)
					require.NotEqual(t, a.sessionID, other.sessionID)
					require.NotEqual(t, a.threadID, other.threadID)
				}
				otherAccount := *account
				otherAccount.ID = 78
				otherAccount.Credentials = map[string]any{"chatgpt_account_id": "upstream-78"}
				failover := svc.resolveCodexIsolatedFingerprintForRequest(context.Background(), conversationTestContext(1, "chat-a"), &otherAccount, nil)
				require.NotEqual(t, a.sessionID, failover.sessionID)
				require.NotEqual(t, a.threadID, failover.threadID)
			})
		}
	}
}

func TestCodexConversationIdentity_BodyHeaderAndMapRawParity(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := conversationTestAccount(AccountTypeOAuth, "full")
	c := conversationTestContext(1, "chat-a")
	c.Request.Header.Set("thread-id", "thread-a")
	body := []byte(`{"model":"gpt-5.5","prompt_cache_key":"chat-a","client_metadata":{"session_id":"chat-a","thread_id":"thread-a"},"input":[{"role":"user","content":[{"type":"input_text","text":"Asia/Shanghai sub2api"},{"type":"input_image","image_url":"data:image/png;base64,AAAA"}]}]}`)
	scoped, _, err := applyCodexAccountIdentityClientMetadataRaw(body, account, 1)
	require.NoError(t, err)
	rawIDs := svc.resolveCodexIsolatedFingerprintForRequest(context.Background(), c, account, scoped)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(scoped, &decoded))
	mapIDs := svc.resolveCodexIsolatedFingerprintForRequest(context.Background(), c, account, decoded)
	headerIDs := svc.resolveCodexIsolatedFingerprintForRequest(context.Background(), c, account, nil)
	for _, ids := range []*codexFingerprintIDs{mapIDs, headerIDs} {
		require.Equal(t, rawIDs.sessionID, ids.sessionID)
		require.Equal(t, rawIDs.threadID, ids.threadID)
	}
	next, _, err := applyCodexFingerprintClientMetadataRaw(scoped, rawIDs)
	require.NoError(t, err)
	require.Equal(t, gjson.GetBytes(body, "input").Raw, gjson.GetBytes(next, "input").Raw)
	require.Equal(t, rawIDs.sessionID, gjson.GetBytes(next, "prompt_cache_key").String())
	regular, err := svc.buildUpstreamRequest(context.Background(), c, account, next, "test-token", true, "chat-a", true)
	require.NoError(t, err)
	passthrough, err := svc.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, account, next, "test-token")
	require.NoError(t, err)
	stageCodexFingerprintIDs(c, rawIDs)
	ws, _, err := svc.buildOpenAIWSHeaders(context.Background(), c, account, "test-token", OpenAIWSProtocolDecision{}, true, "", "", "chat-a", "gpt-5.5", "")
	require.NoError(t, err)
	for _, h := range []http.Header{regular.Header, passthrough.Header, ws} {
		applyStagedCodexFingerprintHeaders(c, account, h)
		applyCodexNormalizedRequestIdentityHeaders(c, account, h, next)
		require.Equal(t, rawIDs.sessionID, h.Get("session_id"))
		require.Equal(t, rawIDs.threadID, h.Get("thread-id"))
		require.Equal(t, rawIDs.turnID, gjson.Get(h.Get("x-codex-turn-metadata"), "turn_id").String())
	}
}

func TestCodexConversationIdentity_NoConversationNeverUsesSharedCache(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := conversationTestAccount(AccountTypeOAuth, "full")
	body := []byte(`{"prompt_cache_key":"shared-prefix-cache"}`)
	c := conversationTestContext(1, "")
	a := svc.resolveCodexIsolatedFingerprintForRequest(context.Background(), c, account, body)
	retry := svc.resolveCodexIsolatedFingerprintForRequest(context.Background(), c, account, body)
	otherRequest := svc.resolveCodexIsolatedFingerprintForRequest(context.Background(), conversationTestContext(1, ""), account, body)
	require.Equal(t, a.sessionID, retry.sessionID)
	require.NotEqual(t, a.sessionID, otherRequest.sessionID)
	next, _, err := applyCodexFingerprintClientMetadataRaw(body, a)
	require.NoError(t, err)
	require.Equal(t, "shared-prefix-cache", gjson.GetBytes(next, "prompt_cache_key").String())
}

func TestCodexConversationIdentity_EmbeddedMetadataAndModes(t *testing.T) {
	svc := &OpenAIGatewayService{}
	c := conversationTestContext(1, "")
	account := conversationTestAccount(AccountTypeOAuth, "full")
	body := []byte(`{"client_metadata":{"x-codex-turn-metadata":"{\"session_id\":\"chat-a\",\"thread_id\":\"thread-a\"}"}}`)
	scoped, _, err := applyCodexAccountIdentityClientMetadataRaw(body, account, 1)
	require.NoError(t, err)
	ids := svc.resolveCodexIsolatedFingerprintForRequest(context.Background(), c, account, scoped)
	require.Equal(t, scopeCodexAccountIdentityValue(account, 1, "session", "chat-a"), ids.sessionID)
	require.Equal(t, scopeCodexAccountIdentityValue(account, 1, "thread", "thread-a"), ids.threadID)
	for _, mode := range []string{"off", "device"} {
		account.Extra[codexFingerprintModeExtraKey] = mode
		got := svc.resolveCodexIsolatedFingerprintForRequest(context.Background(), c, account, scoped)
		if mode == "off" {
			require.Nil(t, got)
		} else {
			require.Empty(t, got.sessionID)
			require.Empty(t, got.threadID)
		}
	}
	account = conversationTestAccount(AccountTypeAPIKey, "full")
	require.Nil(t, svc.resolveCodexIsolatedFingerprintForRequest(context.Background(), c, account, body))
}

func TestCodexConversationIdentity_TurnStateCannotCrossChats(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := conversationTestAccount(AccountTypeOAuth, "full")
	a := conversationTestContext(1, "chat-a")
	b := conversationTestContext(1, "chat-b")
	for _, c := range []*gin.Context{a, b} {
		stageCodexFingerprintIDs(c, svc.resolveCodexIsolatedFingerprintForRequest(context.Background(), c, account, nil))
	}
	svc.bindOpenAICompatSessionTurnState(context.Background(), a, account, "shared-prefix", "opaque-state-a")
	require.Equal(t, "opaque-state-a", svc.getOpenAICompatSessionTurnState(context.Background(), a, account, "shared-prefix"))
	require.Empty(t, svc.getOpenAICompatSessionTurnState(context.Background(), b, account, "shared-prefix"))
	again := conversationTestContext(1, "chat-a")
	stageCodexFingerprintIDs(again, svc.resolveCodexIsolatedFingerprintForRequest(context.Background(), again, account, nil))
	require.Equal(t, "opaque-state-a", svc.getOpenAICompatSessionTurnState(context.Background(), again, account, "shared-prefix"))
}

func TestCodexConversationIdentity_ExplicitCompatSessionAndLegacyAccount(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := conversationTestAccount(AccountTypeSetupToken, "full")
	account.Credentials = nil
	a := svc.resolveCodexIsolatedFingerprintForRequest(context.Background(), conversationTestContext(1, ""), account, nil, "source-protocol-session")
	b := svc.resolveCodexIsolatedFingerprintForRequest(context.Background(), conversationTestContext(1, ""), account, nil, "source-protocol-session")
	other := svc.resolveCodexIsolatedFingerprintForRequest(context.Background(), conversationTestContext(2, ""), account, nil, "source-protocol-session")
	require.Equal(t, a.sessionID, b.sessionID)
	require.NotEqual(t, a.sessionID, other.sessionID)
	require.NotNil(t, svc.resolveCodexIsolatedFingerprintForRequest(context.Background(), nil, account, nil))
	require.Nil(t, svc.resolveCodexIsolatedFingerprintForRequest(context.Background(), nil, nil, nil))
}

func TestCodexConversationIdentity_FallbackNamespace(t *testing.T) {
	legacy := &Account{ID: 77, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	require.Empty(t, codexAccountIdentityNamespace(legacy))
	a := scopeCodexConversationFallback(legacy, 1, "session", "same-client-id")
	require.NotEmpty(t, a)
	require.NotEqual(t, "same-client-id", a)
	require.Equal(t, a, scopeCodexConversationFallback(legacy, 1, "session", "same-client-id"))
	require.NotEqual(t, a, scopeCodexConversationFallback(legacy, 2, "session", "same-client-id"))
	require.NotEqual(t, a, scopeCodexConversationFallback(legacy, 1, "thread", "same-client-id"))
	require.Empty(t, scopeCodexConversationFallback(nil, 1, "session", "value"))
	require.Empty(t, scopeCodexConversationFallback(legacy, 1, "session", " "))
	account := conversationTestAccount(AccountTypeOAuth, "full")
	c := conversationTestContext(1, "")
	c.Request.Header.Set("x-codex-turn-metadata", `{"session_id":"embedded-chat","thread_id":"embedded-thread"}`)
	svc := &OpenAIGatewayService{}
	ids := svc.resolveCodexIsolatedFingerprintForRequest(context.Background(), c, account, nil)
	require.Equal(t, scopeCodexAccountIdentityValue(account, 1, "session", "embedded-chat"), ids.sessionID)
	require.Equal(t, scopeCodexAccountIdentityValue(account, 1, "thread", "embedded-thread"), ids.threadID)
}

func TestCodexConversationIdentity_MetadataAliasesStayConsistent(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := conversationTestAccount(AccountTypeOAuth, "full")
	c := conversationTestContext(1, "header-session")
	body := []byte(`{"prompt_cache_key":"body-session","client_metadata":{"session-id":"body-session","thread-id":"body-thread","installation_id":"old-install","turn-id":"old-turn","window_id":"old-window","x-client-request-id":"old-request","custom":"preserved","x-codex-turn-metadata":"{\"session-id\":\"embedded-session\",\"thread-id\":\"embedded-thread\",\"turn-id\":\"old-turn\",\"x-codex-installation-id\":\"old-install\",\"x-codex-window-id\":\"old-window\",\"x-client-request-id\":\"old-request\"}"}}`)
	scoped, _, err := applyCodexAccountIdentityClientMetadataRaw(body, account, 1)
	require.NoError(t, err)
	ids := svc.resolveCodexIsolatedFingerprintForRequest(context.Background(), c, account, scoped)
	require.Equal(t, scopeCodexAccountIdentityValue(account, 1, "session", "body-session"), ids.sessionID)
	require.Equal(t, scopeCodexAccountIdentityValue(account, 1, "thread", "body-thread"), ids.threadID)
	next, _, err := applyCodexFingerprintClientMetadataRaw(scoped, ids)
	require.NoError(t, err)
	metadata := gjson.GetBytes(next, "client_metadata")
	require.Equal(t, "preserved", metadata.Get("custom").String())
	for _, m := range []gjson.Result{metadata, gjson.Parse(metadata.Get(openAIWSTurnMetadataHeader).String())} {
		for name, want := range map[string]string{
			"session_id": ids.sessionID, "session-id": ids.sessionID,
			"thread_id": ids.threadID, "thread-id": ids.threadID,
			"turn_id": ids.turnID, "turn-id": ids.turnID,
			"installation_id": ids.installationID, "x-codex-installation-id": ids.installationID,
			"window_id": ids.windowID, "x-codex-window-id": ids.windowID,
			"x-client-request-id": ids.threadID,
		} {
			if value := m.Get(name); value.Exists() {
				require.Equal(t, want, value.String(), name)
			}
		}
	}
	require.Equal(t, ids.sessionID, gjson.GetBytes(next, "prompt_cache_key").String())
}

func TestCodexConversationIdentity_WSInheritsOmittedConversation(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := conversationTestAccount(AccountTypeOAuth, "full")
	c := conversationTestContext(1, "header-session")
	first, _, err := applyCodexAccountIdentityClientMetadataRaw([]byte(`{"type":"response.create","client_metadata":{"session_id":"body-session","thread_id":"body-thread"},"input":"hi"}`), account, 1)
	require.NoError(t, err)
	_, err = svc.applyCodexFingerprintToWebSocketPayload(context.Background(), c, account, first)
	require.NoError(t, err)
	initial := *stagedCodexFingerprintIDs(c, account)
	c.Request.Header.Set(openAIWSTurnMetadataHeader, `{"session_id":"header-session","thread_id":"header-thread","sandbox":"preserved"}`)
	followup := []byte(`{"type":"response.create","previous_response_id":"resp_one","input":[{"type":"function_call_output","call_id":"call_one","output":"done"}]}`)
	for range 2 {
		next, err := svc.applyCodexFingerprintToWebSocketPayload(context.Background(), c, account, followup)
		require.NoError(t, err)
		ids := stagedCodexFingerprintIDs(c, account)
		require.Equal(t, initial.sessionID, ids.sessionID)
		require.Equal(t, initial.threadID, ids.threadID)
		require.NotEqual(t, initial.turnID, ids.turnID)
		require.Equal(t, gjson.GetBytes(followup, "input").Raw, gjson.GetBytes(next, "input").Raw)
		metadata := gjson.Parse(gjson.GetBytes(next, "client_metadata."+openAIWSTurnMetadataHeader).String())
		require.Equal(t, "preserved", metadata.Get("sandbox").String())
		require.Equal(t, initial.sessionID, metadata.Get("session_id").String())
	}
}

func TestCodexConversationIdentity_WSBindingBoundaries(t *testing.T) {
	for _, change := range []string{"account", "credential", "tenant", "off", "device", "explicit-session", "explicit-thread", "malformed"} {
		t.Run(change, func(t *testing.T) {
			svc := &OpenAIGatewayService{}
			account := conversationTestAccount(AccountTypeSetupToken, "full")
			c := conversationTestContext(1, "")
			ctx := context.Background()
			_, err := svc.applyCodexFingerprintToWebSocketPayload(ctx, c, account, []byte(`{"type":"response.create"}`))
			require.NoError(t, err)
			first := *stagedCodexFingerprintIDs(c, account)
			body := []byte(`{"type":"response.create"}`)
			switch change {
			case "account":
				account.ID++
				account.Credentials["chatgpt_account_id"] = "other-account"
			case "credential":
				account.Credentials["chatgpt_account_id"] = "other-credential"
			case "tenant":
				c.Set("api_key", &APIKey{ID: 2})
			case "off", "device":
				account.Extra[codexFingerprintModeExtraKey] = change
			case "explicit-session":
				body = []byte(`{"type":"response.create","client_metadata":{"session_id":"new-session"}}`)
			case "explicit-thread":
				body = []byte(`{"type":"response.create","client_metadata":{"thread_id":"new-thread"}}`)
			case "malformed":
				body = []byte(`{"type":"response.create","client_metadata":{"invalid":}}`)
			}
			_, err = svc.applyCodexFingerprintToWebSocketPayload(ctx, c, account, body)
			if change == "malformed" {
				require.Error(t, err)
				require.Equal(t, first, *stagedCodexFingerprintIDs(c, account))
				return
			}
			require.NoError(t, err)
			ids := stagedCodexFingerprintIDs(c, account)
			if change == "off" || change == "device" {
				binding, _ := c.Get(codexWSConversationContextKey)
				require.Nil(t, binding)
				return
			}
			require.NotEqual(t, first.sessionID, ids.sessionID)
			require.NotEqual(t, first.threadID, ids.threadID)
		})
	}
}

func TestCodexConversationIdentity_AliasCanonicalPrecedenceAndDeviceMode(t *testing.T) {
	account := conversationTestAccount(AccountTypeOAuth, "device")
	body := []byte(`{"client_metadata":{"session_id":"canonical","session-id":"alias","thread_id":"canonical-thread","thread-id":"alias-thread","installation_id":"old","x-codex-installation-id":"older","x-codex-turn-metadata":"{\"session_id\":\"embedded\",\"installation_id\":\"old\",\"x-codex-installation-id\":\"older\"}"}}`)
	session, thread := codexScopedBodyConversation(body)
	require.Equal(t, "canonical", session)
	require.Equal(t, "canonical-thread", thread)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(body, &decoded))
	mapSession, mapThread := codexScopedBodyConversation(decoded)
	require.Equal(t, session, mapSession)
	require.Equal(t, thread, mapThread)
	ids := (&OpenAIGatewayService{}).resolveCodexIsolatedFingerprintForRequest(context.Background(), conversationTestContext(1, ""), account, body)
	next, _, err := applyCodexFingerprintClientMetadataRaw(body, ids)
	require.NoError(t, err)
	require.Equal(t, "canonical", gjson.GetBytes(next, "client_metadata.session_id").String())
	require.Equal(t, "alias", gjson.GetBytes(next, "client_metadata.session-id").String())
	require.Equal(t, ids.installationID, gjson.GetBytes(next, "client_metadata.installation_id").String())
}

func TestCodexConversationIdentity_HeaderMetadataAliases(t *testing.T) {
	account := conversationTestAccount(AccountTypeOAuth, "full")
	c := conversationTestContext(1, "")
	c.Request.Header.Set(openAIWSTurnMetadataHeader, `{"session-id":"header-session","thread-id":"header-thread"}`)
	ids := (&OpenAIGatewayService{}).resolveCodexIsolatedFingerprintForRequest(context.Background(), c, account, nil)
	require.Equal(t, scopeCodexAccountIdentityValue(account, 1, "session", "header-session"), ids.sessionID)
	require.Equal(t, scopeCodexAccountIdentityValue(account, 1, "thread", "header-thread"), ids.threadID)
}
