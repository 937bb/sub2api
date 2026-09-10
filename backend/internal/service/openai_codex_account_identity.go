package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const codexAccountIdentityNamespaceVersion = "v1"

const codexAccountIdentitySourceContextKey = "openai_codex_account_identity_source"

var codexAccountMemberIdentityKeys = [...]string{
	"chatgpt_user_id",
	"chatgpt_member_id",
	"chatgpt_membership_id",
	"member_id",
	"membership_id",
}

var codexAccountEmailIdentityKeys = [...]string{
	"email",
	"chatgpt_email",
	"user_email",
	"account_email",
}

func codexAccountMemberIdentity(account *Account) string {
	if account == nil {
		return ""
	}
	for _, key := range codexAccountMemberIdentityKeys {
		if value := strings.TrimSpace(account.GetCredential(key)); value != "" {
			return value
		}
	}
	return ""
}

func codexAccountEmailIdentity(account *Account) string {
	if account == nil {
		return ""
	}
	for _, key := range codexAccountEmailIdentityKeys {
		if value := strings.ToLower(strings.TrimSpace(account.GetCredential(key))); value != "" {
			return value
		}
	}
	return ""
}

// prepareCodexAccountIdentitySource resolves credential shadows once per selected
// attempt. The handler reuses gin.Context across failover attempts, so every entry
// point overwrites the staged source before projecting outbound identity.
func (s *OpenAIGatewayService) prepareCodexAccountIdentitySource(ctx context.Context, c *gin.Context, account *Account) (*Account, error) {
	source := account
	if account != nil && account.IsShadow() {
		resolved, err := resolveCredentialAccount(ctx, s.accountRepo, account)
		if err != nil {
			return nil, err
		}
		source = resolved
	}
	if c != nil {
		c.Set(codexAccountIdentitySourceContextKey, source)
	}
	return source, nil
}

func codexAccountIdentitySource(c *gin.Context, fallback *Account) *Account {
	if c != nil {
		if staged, ok := c.Get(codexAccountIdentitySourceContextKey); ok {
			if source, ok := staged.(*Account); ok && source != nil {
				return source
			}
		}
	}
	return fallback
}

// codexAccountIdentityNamespace returns a stable, credential-scoped namespace.
// Multiple local rows that use the same ChatGPT account intentionally share the
// same namespace. Setup tokens use an irreversible bearer fingerprint because
// they have no refresh lifecycle or imported account metadata. Refreshable OAuth
// otherwise falls back only to a persistent fingerprint seed: local row IDs are
// deployment-relative and must never become upstream identity.
func codexAccountIdentityNamespace(account *Account) string {
	if account == nil || !account.IsOpenAIOAuthLike() {
		return ""
	}
	if upstreamAccountID := strings.TrimSpace(account.GetChatGPTAccountID()); upstreamAccountID != "" {
		if member := codexAccountMemberIdentity(account); member != "" {
			return "chatgpt:" + upstreamAccountID + ":member:" + member
		}
		if email := codexAccountEmailIdentity(account); email != "" {
			return "chatgpt:" + upstreamAccountID + ":email:" + email
		}
		return "chatgpt:" + upstreamAccountID
	}
	if member := codexAccountMemberIdentity(account); member != "" {
		return "member:" + member
	}
	if email := codexAccountEmailIdentity(account); email != "" {
		return "email:" + email
	}
	if seed, ok := codexFingerprintSeed(account.Extra); ok {
		return "seed:" + seed
	}
	if account.Type == AccountTypeSetupToken {
		if token := strings.TrimSpace(account.GetOpenAIAccessToken()); token != "" {
			sum := sha256.Sum256([]byte("openai-setup-token:" + token))
			return fmt.Sprintf("setup-token:%x", sum[:16])
		}
	}
	return ""
}

// isolateOpenAIUpstreamSessionID preserves the existing API-key isolation while
// adding the selected OAuth credential namespace. A scheduler failover therefore
// cannot send the same session/conversation identity through two upstream accounts.
func isolateOpenAIUpstreamSessionID(apiKeyID int64, account *Account, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	namespace := codexAccountIdentityNamespace(account)
	if namespace == "" {
		return isolateOpenAISessionID(apiKeyID, raw)
	}
	return scopeCodexAccountIdentityValue(account, apiKeyID, "session", raw)
}

func scopeCodexAccountIdentityValue(account *Account, apiKeyID int64, kind, raw string) string {
	raw = strings.TrimSpace(raw)
	namespace := codexAccountIdentityNamespace(account)
	if raw == "" || namespace == "" {
		return raw
	}
	// The stored device ID is already account-scoped. Hashing it again makes
	// body metadata disagree with the final HTTP/WS installation header.
	if kind == "installation" {
		if deviceID := account.GetOpenAIDeviceID(); deviceID != "" {
			return deviceID
		}
	}
	return deriveStableUUIDv4(fmt.Sprintf(
		"sub2api:codex-account-identity:%s:user:%d:account:%s:kind:%s:value:%s",
		codexAccountIdentityNamespaceVersion,
		apiKeyID,
		namespace,
		kind,
		raw,
	))
}

var codexAccountIdentityFields = []struct {
	name string
	kind string
}{
	{name: "installation_id", kind: "installation"},
	{name: "x-codex-installation-id", kind: "installation"},
	{name: "session_id", kind: "session"},
	{name: "session-id", kind: "session"},
	{name: "thread_id", kind: "thread"},
	{name: "thread-id", kind: "thread"},
	{name: "turn_id", kind: "turn"},
	{name: "turn-id", kind: "turn"},
	{name: "window_id", kind: "window"},
	{name: "x-codex-window-id", kind: "window"},
	{name: "x-client-request-id", kind: "request"},
}

func applyCodexAccountIdentityFields(values map[string]any, account *Account, apiKeyID int64) bool {
	if values == nil || codexAccountIdentityNamespace(account) == "" {
		return false
	}
	changed := false
	for _, field := range codexAccountIdentityFields {
		raw, ok := values[field.name].(string)
		if !ok || strings.TrimSpace(raw) == "" {
			continue
		}
		next := scopeCodexAccountIdentityValue(account, apiKeyID, field.kind, raw)
		if next != raw {
			values[field.name] = next
			changed = true
		}
	}
	return changed
}

func applyCodexAccountIdentityEmbeddedMetadata(values map[string]any, account *Account, apiKeyID int64) bool {
	raw, ok := values[openAIWSTurnMetadataHeader].(string)
	if !ok || strings.TrimSpace(raw) == "" {
		return false
	}
	metadata := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil || metadata == nil {
		return false
	}
	if !applyCodexAccountIdentityFields(metadata, account, apiKeyID) {
		return false
	}
	rebuilt, err := json.Marshal(metadata)
	if err != nil {
		return false
	}
	values[openAIWSTurnMetadataHeader] = string(rebuilt)
	return true
}

func applyCodexAccountIdentityClientMetadataMap(requestBody map[string]any, account *Account, apiKeyID int64) bool {
	if requestBody == nil || codexAccountIdentityNamespace(account) == "" {
		return false
	}
	changed := false
	clientMetadata, _ := requestBody["client_metadata"].(map[string]any)
	originalBodySessionID := ""
	if clientMetadata != nil {
		originalBodySessionID, _ = clientMetadata["session_id"].(string)
		if applyCodexAccountIdentityFields(clientMetadata, account, apiKeyID) {
			changed = true
		}
		if applyCodexAccountIdentityEmbeddedMetadata(clientMetadata, account, apiKeyID) {
			changed = true
		}
	}
	if raw, ok := requestBody["prompt_cache_key"].(string); ok && strings.TrimSpace(raw) != "" {
		kind := "prompt-cache"
		if strings.TrimSpace(originalBodySessionID) != "" && raw == originalBodySessionID {
			kind = "session"
		}
		next := scopeCodexAccountIdentityValue(account, apiKeyID, kind, raw)
		if next != raw {
			requestBody["prompt_cache_key"] = next
			changed = true
		}
	}
	return changed
}

// applyCodexAccountIdentityClientMetadataRaw scopes only the small identity
// subobjects with gjson/sjson. The passthrough hot path never unmarshals the
// potentially multi-megabyte request body.
func applyCodexAccountIdentityClientMetadataRaw(body []byte, account *Account, apiKeyID int64) ([]byte, bool, error) {
	if len(body) == 0 || codexAccountIdentityNamespace(account) == "" {
		return body, false, nil
	}
	root := gjson.ParseBytes(body)
	if !root.IsObject() {
		return body, false, nil
	}

	next := body
	changed := false
	originalBodySessionID := ""
	if cm := gjson.GetBytes(body, "client_metadata"); cm.IsObject() {
		clientMetadata := map[string]any{}
		if err := json.Unmarshal([]byte(cm.Raw), &clientMetadata); err != nil {
			return body, false, fmt.Errorf("decode client_metadata for account identity: %w", err)
		}
		originalBodySessionID, _ = clientMetadata["session_id"].(string)
		metadataChanged := applyCodexAccountIdentityFields(clientMetadata, account, apiKeyID)
		if applyCodexAccountIdentityEmbeddedMetadata(clientMetadata, account, apiKeyID) {
			metadataChanged = true
		}
		if metadataChanged {
			raw, err := json.Marshal(clientMetadata)
			if err != nil {
				return body, false, fmt.Errorf("encode account-scoped client_metadata: %w", err)
			}
			var setErr error
			next, setErr = sjson.SetRawBytes(next, "client_metadata", raw)
			if setErr != nil {
				return body, false, fmt.Errorf("splice account-scoped client_metadata: %w", setErr)
			}
			changed = true
		}
	}
	if promptCacheKey := gjson.GetBytes(body, "prompt_cache_key"); promptCacheKey.Type == gjson.String && strings.TrimSpace(promptCacheKey.String()) != "" {
		raw := promptCacheKey.String()
		kind := "prompt-cache"
		if strings.TrimSpace(originalBodySessionID) != "" && raw == originalBodySessionID {
			kind = "session"
		}
		scoped := scopeCodexAccountIdentityValue(account, apiKeyID, kind, raw)
		if scoped != raw {
			rewritten, err := sjson.SetBytes(next, "prompt_cache_key", scoped)
			if err != nil {
				return body, false, fmt.Errorf("splice account-scoped prompt_cache_key: %w", err)
			}
			next = rewritten
			changed = true
		}
	}
	return next, changed, nil
}

func applyCodexAccountIdentityHeaders(headers http.Header, account *Account, apiKeyID int64) {
	if headers == nil || codexAccountIdentityNamespace(account) == "" {
		return
	}
	for _, field := range codexAccountIdentityFields {
		// Underscore session/conversation headers are rebuilt separately from the
		// prompt cache key by each request builder.
		if field.name == "session_id" {
			continue
		}
		raw := strings.TrimSpace(headers.Get(field.name))
		if raw != "" {
			headers.Set(field.name, scopeCodexAccountIdentityValue(account, apiKeyID, field.kind, raw))
		}
	}
	if raw := strings.TrimSpace(headers.Get(openAIWSTurnMetadataHeader)); raw != "" {
		metadata := map[string]any{}
		if err := json.Unmarshal([]byte(raw), &metadata); err == nil && metadata != nil && applyCodexAccountIdentityFields(metadata, account, apiKeyID) {
			if rebuilt, err := json.Marshal(metadata); err == nil {
				headers.Set(openAIWSTurnMetadataHeader, string(rebuilt))
			}
		}
	}
}

// applyCodexNormalizedRequestIdentityHeaders projects already-normalized body
// metadata onto the transport. It must run after body scoping/convergence and
// header construction; never hash these values again. The raw client headers
// remain untouched for retries, scheduling and another account's failover.
func applyCodexNormalizedRequestIdentityHeaders(c *gin.Context, account *Account, headers http.Header, body []byte) {
	source := codexAccountIdentitySource(c, account)
	if headers == nil || codexAccountIdentityNamespace(source) == "" {
		return
	}
	metadata := gjson.GetBytes(body, "client_metadata")
	cacheKey := ""
	if metadata.Get("session_id").String() == "" && resolveOpenAIWSSessionHeaders(c, "").SessionID == "" {
		cacheKey = gjson.GetBytes(body, "prompt_cache_key").String()
	}
	applyCodexNormalizedIdentityMetadata(c, source, headers, metadata, cacheKey)
}

func applyCodexNormalizedRequestIdentityHeadersMap(c *gin.Context, account *Account, headers http.Header, body map[string]any) {
	source := codexAccountIdentitySource(c, account)
	if headers == nil || codexAccountIdentityNamespace(source) == "" {
		return
	}
	// Serialize only metadata, never the potentially multi-megabyte input.
	metadata, err := json.Marshal(body["client_metadata"])
	if err != nil {
		return
	}
	cacheKey, _ := body["prompt_cache_key"].(string)
	applyCodexNormalizedIdentityMetadata(c, source, headers, gjson.ParseBytes(metadata), cacheKey)
}

func applyCodexNormalizedIdentityMetadata(c *gin.Context, source *Account, headers http.Header, metadata gjson.Result, cacheKey string) {
	sessionID := strings.TrimSpace(metadata.Get("session_id").String())
	if sessionID == "" {
		// A cache key may be the only identity supplied by an API-compatible
		// client. At this point the body key is already credential-scoped.
		if resolveOpenAIWSSessionHeaders(c, "").SessionID == "" {
			sessionID = strings.TrimSpace(cacheKey)
		}
	}
	if sessionID == "" {
		sessionID = strings.TrimSpace(headers.Get("session-id"))
	}
	if sessionID == "" {
		sessionID = strings.TrimSpace(headers.Get("session_id"))
	}
	if sessionID != "" {
		headers.Set("session-id", sessionID)
		headers.Set("session_id", sessionID)
		// Preserve an explicit conversation ID. Only update the compatibility
		// alias that a builder synthesized from the same session/cache key.
		if (c == nil || strings.TrimSpace(c.GetHeader("conversation_id")) == "") && headers.Get("conversation_id") != "" {
			headers.Set("conversation_id", sessionID)
		}
	}
	for _, projection := range [...]struct{ field, header string }{
		{"thread_id", "thread-id"},
		{"x-codex-window-id", "x-codex-window-id"},
		{"x-codex-installation-id", "x-codex-installation-id"},
	} {
		if value := strings.TrimSpace(metadata.Get(projection.field).String()); value != "" {
			headers.Set(projection.header, value)
		}
	}
	if deviceID := source.GetOpenAIDeviceID(); deviceID != "" {
		headers.Set("x-codex-installation-id", deviceID)
	}
}
