package service

import (
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

func clearCodexCacheOnlyHTTPIdentity(c *gin.Context) {
	if c != nil {
		c.Set(codexCacheOnlyHTTPIdentityContextKey, nil)
	}
}

// A prompt-cache key may be shared by unrelated conversations. It may supply
// the HTTP cache-affinity header, but never conversation metadata or execution
// state. Native WebSocket connections retain their connection-local identity.
func stageCodexCacheOnlyHTTPIdentity(c *gin.Context, account *Account, body any) bool {
	if c == nil {
		return false
	}
	clearCodexCacheOnlyHTTPIdentity(c)
	if c.Request == nil || c.Request.URL == nil || account == nil || !account.UsesOpenAICodexProtocol() ||
		c.Request.Method != http.MethodPost || isOpenAIResponsesCompactPath(c) ||
		!strings.HasSuffix(strings.TrimRight(c.Request.URL.Path, "/"), "/responses") ||
		strings.EqualFold(strings.TrimSpace(c.GetHeader("Upgrade")), "websocket") {
		return false
	}

	var cacheKey, eventType string
	switch value := body.(type) {
	case []byte:
		cacheKey = codexCacheStringField(gjson.GetBytes(value, "prompt_cache_key"))
		eventType = codexCacheStringField(gjson.GetBytes(value, "type"))
	case map[string]any:
		cacheKey, _ = value["prompt_cache_key"].(string)
		eventType, _ = value["type"].(string)
	}
	if strings.TrimSpace(cacheKey) == "" || strings.HasPrefix(strings.TrimSpace(eventType), "response.") {
		return false
	}

	sessionID, threadID := codexCacheBodyConversation(body)
	if sessionID != "" || threadID != "" || explicitOpenAIHeaderSessionID(c) != "" {
		return false
	}
	headerMetadata := gjson.Parse(c.GetHeader(openAIWSTurnMetadataHeader))
	headerSessionID, headerThreadID := codexCacheMetadataConversation(func(key string) string {
		return codexCacheStringField(headerMetadata.Get(key))
	})
	if headerSessionID != "" || headerThreadID != "" ||
		strings.TrimSpace(c.GetHeader("thread-id")) != "" ||
		strings.TrimSpace(c.GetHeader("x-codex-window-id")) != "" {
		return false
	}

	source := codexAccountIdentitySource(c, account)
	if source == nil {
		return false
	}
	identity := codexCacheOnlyHTTPIdentity{
		accountID: source.ID,
		apiKeyID:  getAPIKeyIDFromContext(c),
		namespace: codexAccountIdentityNamespace(source),
	}
	// The caller has already scoped the body to this credential and API key.
	// Never promote a raw, unscoped client value to an upstream routing header.
	if identity.namespace != "" && identity.apiKeyID > 0 {
		if parsed, err := uuid.Parse(cacheKey); err == nil && parsed.Version() == 4 && parsed.String() == cacheKey {
			identity.cacheRoutingID = cacheKey
		}
	}
	c.Set(codexCacheOnlyHTTPIdentityContextKey, identity)
	return true
}

func resolveCodexCacheAwareFingerprintIDs(c *gin.Context, account *Account, body any) *codexFingerprintIDs {
	cacheOnlyHTTP := stageCodexCacheOnlyHTTPIdentity(c, account, body)
	var headers http.Header
	if c != nil && c.Request != nil {
		headers = c.Request.Header
	}
	ids := resolveCodexFingerprintIDsFromRequest(account, headers)
	if !cacheOnlyHTTP || ids == nil || ids.mode == codexFingerprintDevice {
		return ids
	}
	// Keep the stable device without inventing conversation state for a client
	// that supplied only an independent prompt-cache routing key.
	ids.mode = codexFingerprintDevice
	ids.sessionID, ids.threadID, ids.turnID, ids.windowID = "", "", "", ""
	ids.turnStartedAtUnixMs = 0
	return ids
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

// Remove aliases that could promote cache/request identifiers to conversation
// identifiers before the final transport routing header is installed.
func omitCodexCacheOnlyHTTPConversationHeaders(c *gin.Context, account *Account, headers http.Header) {
	if headers == nil || !isCodexCacheOnlyHTTPIdentity(c, account) {
		return
	}
	for _, name := range [...]string{"session-id", "session_id", "conversation_id", "thread-id", "x-client-request-id", "x-codex-window-id"} {
		headers.Del(name)
	}
}

// Codex uses the HTTP session-id header for prompt-cache affinity, separately
// from actual conversation metadata. Apply it only at the final HTTP boundary.
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

// Inspect only the small metadata object; large image and tool payloads remain untouched.
func codexCacheBodyConversation(body any) (string, string) {
	switch value := body.(type) {
	case []byte:
		metadata := gjson.GetBytes(value, "client_metadata")
		return codexCacheMetadataConversation(func(key string) string {
			return codexCacheStringField(metadata.Get(key))
		})
	case map[string]any:
		return codexCacheMapMetadataConversation(value["client_metadata"])
	}
	return "", ""
}

func codexCacheMapMetadataConversation(value any) (string, string) {
	return codexCacheMetadataConversation(func(key string) string {
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

func codexCacheMetadataConversation(get func(string) string) (string, string) {
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
		sessionID = first(sessionID, first(codexCacheStringField(metadata.Get("session_id")), codexCacheStringField(metadata.Get("session-id"))))
		threadID = first(threadID, first(codexCacheStringField(metadata.Get("thread_id")), codexCacheStringField(metadata.Get("thread-id"))))
	}
	return sessionID, threadID
}

func codexCacheStringField(value gjson.Result) string {
	if value.Type != gjson.String {
		return ""
	}
	return value.String()
}
