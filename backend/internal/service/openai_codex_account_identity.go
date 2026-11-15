package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const codexAccountIdentitySourceContextKey = "openai_codex_account_identity_source"

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
		if upstreamUserID := strings.TrimSpace(account.GetCredential("chatgpt_user_id")); upstreamUserID != "" {
			return "chatgpt:" + upstreamAccountID + ":user:" + upstreamUserID
		}
		return "chatgpt:" + upstreamAccountID
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

// codexAccountInstallationID belongs to the credential, never to a downstream
// API key, client installation, session or access-token refresh. Existing explicit
// devices and persisted convergence seeds retain their identities. Otherwise the
// durable upstream account namespace makes all replicas derive the same device.
func codexAccountInstallationID(account *Account) string {
	if account == nil || !account.IsOpenAIOAuthLike() {
		return ""
	}
	if deviceID := account.GetOpenAIDeviceID(); deviceID != "" {
		return deviceID
	}
	if seed, ok := codexFingerprintSeed(account.Extra); ok {
		return resolveConvergedInstallationID(account, seed)
	}
	if namespace := codexAccountIdentityNamespace(account); namespace != "" {
		return deriveStableUUIDv7("sub2api:codex-account-installation:v2:" + namespace)
	}
	return ""
}

// Convergence remains a per-row policy, but a shadow must use its credential
// owner's device just like the HTTP headers and native WS payloads do.
func resolveCodexFingerprintIDsForAttempt(c *gin.Context, account *Account, headers http.Header) *codexFingerprintIDs {
	ids := resolveCodexFingerprintIDsFromRequest(account, headers)
	if ids != nil {
		if installationID := codexAccountInstallationID(codexAccountIdentitySource(c, account)); installationID != "" {
			ids.installationID = installationID
		}
	}
	return ids
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
	sum := sha256.Sum256([]byte(fmt.Sprintf("u%d:a%s:%s", apiKeyID, namespace, raw)))
	return fmt.Sprintf("%x", sum[:8])
}

func scopeCodexAccountIdentityValue(account *Account, apiKeyID int64, kind, raw string) string {
	if kind == "installation" {
		if installationID := codexAccountInstallationID(account); installationID != "" {
			return installationID
		}
	}
	raw = strings.TrimSpace(raw)
	namespace := codexAccountIdentityNamespace(account)
	if raw == "" || namespace == "" {
		return raw
	}
	scope := fmt.Sprintf(
		"sub2api:codex-account-identity:v2:user:%d:account:%s",
		apiKeyID,
		namespace,
	)
	if kind == "window" {
		if separator := strings.LastIndexByte(raw, ':'); separator > 0 {
			base, suffix := raw[:separator], raw[separator:]
			if _, err := uuid.Parse(base); err == nil {
				return deriveScopedUUIDv7(scope, base) + suffix
			}
		}
	}
	// The kind is deliberately absent from the derivation. Official Desktop
	// root requests reuse one UUID for session-id, thread-id and request-id; the
	// account namespace changes the identity without breaking that relation.
	return deriveScopedUUIDv7(scope, raw)
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
	{name: "parent_thread_id", kind: "thread"},
	{name: codexParentThreadIDHeader, kind: "thread"},
	{name: "forked_from_thread_id", kind: "thread"},
	{name: "turn_id", kind: "turn"},
	{name: "turn-id", kind: "turn"},
	{name: "root_turn_id", kind: "turn"},
	{name: "parent_turn_id", kind: "turn"},
	{name: "context_window_id", kind: "context-window"},
	{name: "window_id", kind: "window"},
	{name: "x-codex-window-id", kind: "window"},
	{name: "x-client-request-id", kind: "request"},
}

func applyCodexAccountIdentityFields(values map[string]any, account *Account, apiKeyID int64) bool {
	if values == nil || (codexAccountIdentityNamespace(account) == "" && codexAccountInstallationID(account) == "") {
		return false
	}
	changed := false
	for _, field := range codexAccountIdentityFields {
		raw, ok := values[field.name].(string)
		if (!ok || strings.TrimSpace(raw) == "") && field.kind != "installation" {
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
	if err := decodeCodexIdentityMetadata(raw, &metadata); err != nil || metadata == nil {
		return false
	}
	if !applyCodexAccountIdentityFields(metadata, account, apiKeyID) {
		return false
	}
	rebuilt, err := marshalCodexTurnMetadata(metadata)
	if err != nil {
		return false
	}
	values[openAIWSTurnMetadataHeader] = string(rebuilt)
	return true
}

func applyCodexAccountIdentityClientMetadataMap(requestBody map[string]any, account *Account, apiKeyID int64) bool {
	if requestBody == nil || (codexAccountIdentityNamespace(account) == "" && codexAccountInstallationID(account) == "") {
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
	if len(body) == 0 || (codexAccountIdentityNamespace(account) == "" && codexAccountInstallationID(account) == "") {
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
		if err := decodeCodexIdentityMetadata(cm.Raw, &clientMetadata); err != nil {
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
	if headers == nil || (codexAccountIdentityNamespace(account) == "" && codexAccountInstallationID(account) == "") {
		return
	}
	for _, field := range codexAccountIdentityFields {
		// Underscore session/conversation headers are rebuilt separately from the
		// prompt cache key by each request builder.
		if field.name == "session_id" {
			continue
		}
		raw := strings.TrimSpace(codexIdentityHeaderValue(headers, field.name))
		if raw != "" {
			setCodexIdentityHeader(headers, field.name, scopeCodexAccountIdentityValue(account, apiKeyID, field.kind, raw))
		}
	}
	// Include the account device even when SDKs omit client installation headers;
	// collapse case aliases so a downstream header cannot survive beside it.
	if installationID := codexAccountInstallationID(account); installationID != "" {
		setCodexIdentityHeader(headers, "x-codex-installation-id", installationID)
		for key := range headers {
			if strings.EqualFold(key, "installation_id") {
				setCodexIdentityHeader(headers, "installation_id", installationID)
				break
			}
		}
	}
	if raw := strings.TrimSpace(codexIdentityHeaderValue(headers, openAIWSTurnMetadataHeader)); raw != "" {
		metadata := map[string]any{}
		if err := decodeCodexIdentityMetadata(raw, &metadata); err == nil && metadata != nil && applyCodexAccountIdentityFields(metadata, account, apiKeyID) {
			if rebuilt, err := marshalCodexTurnMetadata(metadata); err == nil {
				setCodexIdentityHeader(headers, openAIWSTurnMetadataHeader, string(rebuilt))
			}
		}
	}
}

// applyCodexOutboundIdentityHeaders applies one complete identity policy.
// Session/full convergence owns the full namespace. Device mode owns only the
// installation ID, so the remaining session/thread fields still need account
// and API-key isolation before the device override is applied.
func applyCodexOutboundIdentityHeaders(c *gin.Context, account *Account, headers http.Header) {
	if ids := stagedCodexFingerprintIDs(c, account); ids != nil {
		if ids.mode == codexFingerprintDevice {
			applyCodexAccountIdentityHeaders(headers, codexAccountIdentitySource(c, account), getAPIKeyIDFromContext(c))
		}
		applyCodexFingerprintHeaders(headers, ids)
		return
	}
	applyCodexAccountIdentityHeaders(headers, codexAccountIdentitySource(c, account), getAPIKeyIDFromContext(c))
}

func applyCodexOutboundIdentityClientMetadataMap(c *gin.Context, account *Account, requestBody map[string]any) bool {
	if ids := stagedCodexFingerprintIDs(c, account); ids != nil {
		changed := false
		if ids.mode == codexFingerprintDevice {
			changed = applyCodexAccountIdentityClientMetadataMap(requestBody, codexAccountIdentitySource(c, account), getAPIKeyIDFromContext(c))
		}
		return applyCodexFingerprintClientMetadata(requestBody, ids) || changed
	}
	return applyCodexAccountIdentityClientMetadataMap(requestBody, codexAccountIdentitySource(c, account), getAPIKeyIDFromContext(c))
}

func applyCodexOutboundIdentityClientMetadataRaw(c *gin.Context, account *Account, body []byte) ([]byte, bool, error) {
	if ids := stagedCodexFingerprintIDs(c, account); ids != nil {
		if ids.mode == codexFingerprintDevice {
			next, changed, err := applyCodexAccountIdentityClientMetadataRaw(body, codexAccountIdentitySource(c, account), getAPIKeyIDFromContext(c))
			if err != nil {
				return body, false, err
			}
			fingerprinted, fingerprintChanged, fingerprintErr := applyCodexFingerprintClientMetadataRaw(next, ids)
			return fingerprinted, changed || fingerprintChanged, fingerprintErr
		}
		return applyCodexFingerprintClientMetadataRaw(body, ids)
	}
	return applyCodexAccountIdentityClientMetadataRaw(body, codexAccountIdentitySource(c, account), getAPIKeyIDFromContext(c))
}

// stageCodexOutboundIdentityForExistingMetadata enables configured fingerprint
// convergence only for a request that already carries a Desktop metadata
// envelope. Compatibility bridges must not invent Desktop identity for plain
// Chat Completions or Anthropic clients.
func stageCodexOutboundIdentityForExistingMetadata(c *gin.Context, account *Account, requestBody map[string]any) {
	stageCodexFingerprintIDs(c, nil)
	if requestBody == nil {
		return
	}
	switch requestBody["client_metadata"].(type) {
	case map[string]any:
		// Existing Desktop envelope: fingerprint convergence may normalize it.
	default:
		return
	}
	var clientHeaders http.Header
	if c != nil && c.Request != nil {
		clientHeaders = c.Request.Header
	}
	stageCodexFingerprintIDs(c, resolveCodexFingerprintIDsForAttempt(c, account, clientHeaders))
}

// applyCodexOutboundIdentityTurnMetadata scopes one metadata blob without
// touching an already-normalized request body. HTTP -> WS forwarding uses this
// when the body omitted metadata and the handshake header supplies it later.
func applyCodexOutboundIdentityTurnMetadata(c *gin.Context, account *Account, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	metadata := map[string]any{}
	if err := decodeCodexIdentityMetadata(raw, &metadata); err != nil || metadata == nil {
		return raw
	}
	if ids := stagedCodexFingerprintIDs(c, account); ids != nil {
		if ids.mode == codexFingerprintDevice {
			applyCodexAccountIdentityFields(metadata, codexAccountIdentitySource(c, account), getAPIKeyIDFromContext(c))
			metadata["installation_id"] = ids.installationID
		} else {
			rewriteCodexFingerprintLinkedFields(metadata, ids)
			replaceCodexTurnMetadataFields(metadata, map[string]any{
				"installation_id":         ids.installationID,
				"session_id":              ids.sessionID,
				"thread_id":               ids.threadID,
				"turn_id":                 ids.turnID,
				"window_id":               ids.windowID,
				"turn_started_at_unix_ms": ids.turnStartedAtUnixMs,
			})
		}
	} else {
		applyCodexAccountIdentityFields(metadata, codexAccountIdentitySource(c, account), getAPIKeyIDFromContext(c))
	}
	rebuilt, err := marshalCodexTurnMetadata(metadata)
	if err != nil {
		return raw
	}
	return string(rebuilt)
}

// Preserve opaque numeric metadata while rewriting only known identity fields.
func decodeCodexIdentityMetadata(raw string, out *map[string]any) error {
	if !json.Valid([]byte(raw)) {
		return fmt.Errorf("invalid Codex identity metadata JSON")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	return decoder.Decode(out)
}
