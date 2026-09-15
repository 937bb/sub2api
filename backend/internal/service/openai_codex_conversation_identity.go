package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const codexAnonymousConversationContextKey = "codex_anonymous_conversation"

// resolveCodexIsolatedFingerprintForRequest receives an already account-scoped
// body. Device identity remains stable, but session/full modes must never merge
// separate conversations or tenants into one upstream session/thread.
func (s *OpenAIGatewayService) resolveCodexIsolatedFingerprintForRequest(ctx context.Context, c *gin.Context, account *Account, body any, explicitCompatSession ...string) *codexFingerprintIDs {
	var headers http.Header
	if c != nil && c.Request != nil {
		headers = c.Request.Header
	}
	ids := s.resolveCodexFingerprintIDsForRequest(ctx, account, headers)
	if ids == nil || ids.mode == codexFingerprintDevice {
		return ids
	}
	apiKeyID := getAPIKeyIDFromContext(c)
	sessionID, threadID := codexScopedBodyConversation(body)
	headerMetadata := gjson.Parse(headers.Get(openAIWSTurnMetadataHeader))
	headerSessionID, headerThreadID := codexMetadataConversation(func(key string) string {
		return codexStringField(headerMetadata.Get(key))
	})
	// Namespace helpers leave values unchanged when credential metadata is
	// unavailable. Use the managed fingerprint seed in that case as well.
	if codexAccountIdentityNamespace(account) == "" {
		sessionID = scopeCodexConversationFallback(account, apiKeyID, "session", sessionID)
		threadID = scopeCodexConversationFallback(account, apiKeyID, "thread", threadID)
	}
	if sessionID == "" {
		resolution := resolveOpenAIWSSessionHeaders(c, "")
		sessionID = scopeCodexConversationFallback(account, apiKeyID, "session", resolution.SessionID)
	}
	if sessionID == "" {
		sessionID = scopeCodexConversationFallback(account, apiKeyID, "session", headerSessionID)
	}
	if sessionID == "" && len(explicitCompatSession) > 0 {
		// The Messages bridge maintains a legacy conversation binding derived
		// from source metadata or digest lineage. Preserve that contract here;
		// ordinary Responses cache keys are never passed as this fallback.
		sessionID = scopeCodexConversationFallback(account, apiKeyID, "session", explicitCompatSession[0])
	}
	if threadID == "" {
		threadID = scopeCodexConversationFallback(account, apiKeyID, "thread", headers.Get("thread-id"))
	}
	if threadID == "" {
		threadID = scopeCodexConversationFallback(account, apiKeyID, "thread", headerThreadID)
	}
	if sessionID == "" {
		sessionID = threadID
	}
	if sessionID == "" {
		// A prompt-cache key can intentionally be shared by unrelated chats.
		// It is not evidence that their conversational state may be shared.
		sessionID = scopeCodexConversationFallback(account, apiKeyID, "session", codexAnonymousConversation(c))
	}
	if threadID == "" {
		threadID = sessionID
	}
	ids.sessionID = sessionID
	ids.threadID = threadID
	ids.windowID = threadID + ":0"
	return ids
}

func scopeCodexConversationFallback(account *Account, apiKeyID int64, kind, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if codexAccountIdentityNamespace(account) != "" {
		return scopeCodexAccountIdentityValue(account, apiKeyID, kind, value)
	}
	seed, ok := codexFingerprintDerivationSeed(account)
	if !ok {
		return ""
	}
	return deriveStableUUIDv4(fmt.Sprintf("codex-conversation:v1:%q:%d:%q:%q", seed, apiKeyID, kind, value))
}

// Inspect only the small metadata object, never decode image or tool payloads.
func codexScopedBodyConversation(body any) (string, string) {
	switch value := body.(type) {
	case []byte:
		metadata := gjson.GetBytes(value, "client_metadata")
		return codexMetadataConversation(func(key string) string { return codexStringField(metadata.Get(key)) })
	case map[string]any:
		return codexMapMetadataConversation(value["client_metadata"])
	}
	return "", ""
}

func codexMapMetadataConversation(value any) (string, string) {
	return codexMetadataConversation(func(key string) string {
		switch metadata := value.(type) {
		case map[string]any:
			text, _ := metadata[key].(string)
			return text
		case map[string]string:
			return metadata[key]
		}
		return ""
	})
}

func codexMetadataConversation(get func(string) string) (string, string) {
	first := func(a, b string) string {
		if a = strings.TrimSpace(a); a != "" {
			return a
		}
		return strings.TrimSpace(b)
	}
	sessionID := first(get("session_id"), get("session-id"))
	threadID := first(get("thread_id"), get("thread-id"))
	if embedded := get(openAIWSTurnMetadataHeader); embedded != "" && (sessionID == "" || threadID == "") {
		metadata := gjson.Parse(embedded)
		sessionID = first(sessionID, first(codexStringField(metadata.Get("session_id")), codexStringField(metadata.Get("session-id"))))
		threadID = first(threadID, first(codexStringField(metadata.Get("thread_id")), codexStringField(metadata.Get("thread-id"))))
	}
	return sessionID, threadID
}

// Keep only the connection's conversation binding, never a mutable turn object.
// Credential and tenant checks prevent reuse after an account failover.
const codexWSConversationContextKey = "codex_ws_conversation"

type codexWSConversationBinding struct {
	accountID, apiKeyID int64
	namespace           string
	mode                codexFingerprintMode
	sessionID, threadID string
}

func inheritCodexWSConversation(c *gin.Context, account *Account, body []byte, ids *codexFingerprintIDs) {
	if c == nil || ids == nil || ids.sessionID == "" {
		return
	}
	value, _ := c.Get(codexWSConversationContextKey)
	previous, ok := value.(codexWSConversationBinding)
	if !ok || previous.accountID != account.ID || previous.apiKeyID != getAPIKeyIDFromContext(c) || previous.namespace != codexAccountIdentityNamespace(account) || previous.mode != ids.mode {
		return
	}
	sessionID, threadID := codexScopedBodyConversation(body)
	if codexAccountIdentityNamespace(account) == "" {
		sessionID = scopeCodexConversationFallback(account, previous.apiKeyID, "session", sessionID)
		threadID = scopeCodexConversationFallback(account, previous.apiKeyID, "thread", threadID)
	}
	// An explicit new conversation wins. Missing metadata on a continuing
	// connection inherits its accepted frame, not stale handshake headers.
	if sessionID == "" && (threadID == "" || threadID == previous.threadID) {
		ids.sessionID = previous.sessionID
	}
	if threadID == "" && ids.sessionID == previous.sessionID {
		ids.threadID = previous.threadID
	}
	ids.windowID = ids.threadID + ":0"
}

func stageCodexWSConversation(c *gin.Context, account *Account, ids *codexFingerprintIDs) {
	if c == nil {
		return
	}
	if account == nil || ids == nil || ids.sessionID == "" {
		c.Set(codexWSConversationContextKey, nil)
		return
	}
	c.Set(codexWSConversationContextKey, codexWSConversationBinding{
		accountID: account.ID, apiKeyID: getAPIKeyIDFromContext(c),
		namespace: codexAccountIdentityNamespace(account), mode: ids.mode,
		sessionID: ids.sessionID, threadID: ids.threadID,
	})
}

// Copy header-only metadata after resolving frame identity so a stale
// handshake cannot override a continuing conversation or explicit frame data.
func inheritCodexWSHeaderMetadata(c *gin.Context, account *Account, body []byte) ([]byte, error) {
	if c == nil || c.Request == nil || account == nil || !account.IsOpenAIOAuthLike() || !gjson.ParseBytes(body).IsObject() {
		return body, nil
	}
	path := "client_metadata." + openAIWSTurnMetadataHeader
	if gjson.GetBytes(body, path).Exists() {
		return body, nil
	}
	raw := strings.TrimSpace(c.GetHeader(openAIWSTurnMetadataHeader))
	if raw == "" {
		return body, nil
	}
	metadata := map[string]any{openAIWSTurnMetadataHeader: raw}
	applyCodexAccountIdentityEmbeddedMetadata(metadata, account, getAPIKeyIDFromContext(c))
	next, err := sjson.SetBytes(body, path, metadata[openAIWSTurnMetadataHeader])
	if err != nil {
		return body, fmt.Errorf("merge websocket header metadata: %w", err)
	}
	return next, nil
}

// Canonical fields win, but update recognized aliases only when present.
// Do not add extra metadata or change arbitrary user-supplied fields.
func synchronizeCodexMetadataAliases(metadata, fields map[string]any) {
	for _, group := range [][2]string{
		{"installation_id", "x-codex-installation-id"},
		{"session_id", "session-id"},
		{"thread_id", "thread-id"},
		{"turn_id", "turn-id"},
		{"window_id", "x-codex-window-id"},
		{"thread_id", "x-client-request-id"},
	} {
		if value, ok := fields[group[0]]; ok {
			for _, key := range group {
				if _, exists := metadata[key]; exists {
					metadata[key] = value
				}
			}
		}
	}
}

func codexStringField(value gjson.Result) string {
	if value.Type != gjson.String {
		return ""
	}
	return value.String()
}

func codexAnonymousConversation(c *gin.Context) string {
	if c != nil {
		if value := c.GetString(codexAnonymousConversationContextKey); value != "" {
			return value
		}
	}
	value := uuid.NewString()
	if c != nil {
		// Reuse for retry attempts and frames on this inbound WS connection,
		// never for another inbound HTTP request or another user's connection.
		c.Set(codexAnonymousConversationContextKey, value)
	}
	return value
}
