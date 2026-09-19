package service

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

const codexCacheOnlyHTTPIdentityContextKey = "codex_cache_only_http_identity"

type codexCacheOnlyHTTPIdentity struct {
	accountID      int64
	apiKeyID       int64
	namespace      string
	cacheRoutingID string
}

// A prompt-cache key may be shared by unrelated conversations. It may supply
// the HTTP cache-affinity header, but never conversation metadata or execution
// state. Native WebSocket connections retain their connection-local identity.
func stageCodexCacheOnlyHTTPIdentity(c *gin.Context, account *Account, body any, explicitCompatSession ...string) bool {
	if c == nil {
		return false
	}
	c.Set(codexCacheOnlyHTTPIdentityContextKey, nil)
	if c.Request == nil || c.Request.URL == nil || account == nil || !account.UsesOpenAICodexProtocol() ||
		c.Request.Method != http.MethodPost || isOpenAIResponsesCompactPath(c) ||
		!strings.HasSuffix(strings.TrimRight(c.Request.URL.Path, "/"), "/responses") ||
		strings.EqualFold(strings.TrimSpace(c.GetHeader("Upgrade")), "websocket") {
		return false
	}
	if len(explicitCompatSession) > 0 && strings.TrimSpace(explicitCompatSession[0]) != "" {
		return false
	}
	var cacheKey, eventType string
	switch value := body.(type) {
	case []byte:
		cacheKey = codexStringField(gjson.GetBytes(value, "prompt_cache_key"))
		eventType = codexStringField(gjson.GetBytes(value, "type"))
	case map[string]any:
		cacheKey, _ = value["prompt_cache_key"].(string)
		eventType, _ = value["type"].(string)
	}
	if strings.TrimSpace(cacheKey) == "" || strings.HasPrefix(strings.TrimSpace(eventType), "response.") {
		return false
	}
	sessionID, threadID := codexScopedBodyConversation(body)
	if sessionID != "" || threadID != "" || explicitOpenAIHeaderSessionID(c) != "" {
		return false
	}
	headerMetadata := gjson.Parse(c.GetHeader(openAIWSTurnMetadataHeader))
	headerSessionID, headerThreadID := codexMetadataConversation(func(key string) string {
		return codexStringField(headerMetadata.Get(key))
	})
	if headerSessionID != "" || headerThreadID != "" ||
		strings.TrimSpace(c.GetHeader("thread-id")) != "" ||
		strings.TrimSpace(c.GetHeader("x-codex-window-id")) != "" {
		return false
	}
	identity := codexCacheOnlyHTTPIdentity{
		accountID: account.ID, apiKeyID: getAPIKeyIDFromContext(c), namespace: codexAccountIdentityNamespace(account),
	}
	// The caller has already scoped the body to this credential and API key.
	// Legacy accounts without a namespace retain the raw PCK; never promote
	// that unscoped value to an upstream routing header.
	if identity.namespace != "" && identity.apiKeyID > 0 {
		if parsed, err := uuid.Parse(cacheKey); err == nil && parsed.Version() == 4 && parsed.String() == cacheKey {
			identity.cacheRoutingID = cacheKey
		}
	}
	c.Set(codexCacheOnlyHTTPIdentityContextKey, identity)
	return true
}

func isCodexCacheOnlyHTTPIdentity(c *gin.Context, account *Account) bool {
	if c == nil || account == nil || !account.UsesOpenAICodexProtocol() {
		return false
	}
	value, _ := c.Get(codexCacheOnlyHTTPIdentityContextKey)
	identity, ok := value.(codexCacheOnlyHTTPIdentity)
	if !ok || identity.apiKeyID != getAPIKeyIDFromContext(c) {
		return false
	}
	source := codexAccountIdentitySource(c, account)
	return source != nil && identity.accountID == source.ID && identity.namespace == codexAccountIdentityNamespace(source)
}

func codexCacheOnlyHTTPPromptCacheSession(c *gin.Context, account *Account, cacheKey string) string {
	if isCodexCacheOnlyHTTPIdentity(c, account) {
		return ""
	}
	return cacheKey
}

// Remove transport aliases that could promote cache/request identifiers to
// conversation identifiers at the final transport normalization boundary.
func omitCodexCacheOnlyHTTPConversationHeaders(c *gin.Context, account *Account, headers http.Header) {
	if headers == nil || !isCodexCacheOnlyHTTPIdentity(c, account) {
		return
	}
	for _, name := range [...]string{"session-id", "session_id", "conversation_id", "thread-id", "x-client-request-id", "x-codex-window-id"} {
		headers.Del(name)
	}
}

// Codex uses the HTTP session-id header for prompt-cache affinity, separately
// from actual conversation metadata. Apply it only at the HTTP send boundary:
// shared builders and WS normalization would otherwise derive a shared thread.
func applyCodexCacheOnlyHTTPRoutingHeaders(c *gin.Context, account *Account, request *http.Request) {
	if request == nil || request.URL == nil || request.Method != http.MethodPost ||
		(request.URL.Scheme != "https" && request.URL.Scheme != "http") ||
		!strings.EqualFold(request.URL.Hostname(), "chatgpt.com") ||
		strings.TrimRight(request.URL.Path, "/") != "/backend-api/codex/responses" ||
		strings.EqualFold(strings.TrimSpace(request.Header.Get("Upgrade")), "websocket") ||
		!isCodexCacheOnlyHTTPIdentity(c, account) {
		return
	}
	value, _ := c.Get(codexCacheOnlyHTTPIdentityContextKey)
	identity, ok := value.(codexCacheOnlyHTTPIdentity)
	if !ok || identity.cacheRoutingID == "" {
		return
	}
	if request.Header == nil {
		request.Header = make(http.Header)
	}
	omitCodexCacheOnlyHTTPConversationHeaders(c, account, request.Header)
	request.Header.Set("session-id", identity.cacheRoutingID)
}

// HTTP-to-WS adaptation may need an execution key for retries and connection
// state, but a shared cache key cannot supply it. This value never goes upstream.
func codexCacheOnlyHTTPExecutionScope(c *gin.Context, account *Account) string {
	if !isCodexCacheOnlyHTTPIdentity(c, account) {
		return ""
	}
	source := codexAccountIdentitySource(c, account)
	scope, _ := deriveOpenAISessionHashes(fmt.Sprintf("codex-cache-only-http:%d:%d:%s:%s", source.ID,
		getAPIKeyIDFromContext(c), codexAccountIdentityNamespace(source), codexAnonymousConversation(c)))
	return scope
}
