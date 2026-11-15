package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// openAICodexTurnStateHeader 是 Codex 的回合状态头。上游在响应头中铸造该
// 不透明 blob，客户端在同一回合的后续请求中原样回带（codex-rs 侧从
// /responses SSE、/responses/compact JSON 与 WS 握手三种响应中捕获，见
// codex-api/src/sse/responses.rs 与 endpoint/compact.rs）。
const openAICodexTurnStateHeader = "x-codex-turn-state"

// Provenance is attached to an opaque state value, not the last account to use
// a session. Only digests and owner identifiers are stored in the shared cache.
type openAICodexTurnStateOrigin struct {
	accountID int64 // local diagnostic identity; ownership uses the credential namespace
	owner     string
	expiresAt time.Time
}

const openAICodexTurnStateMaxOrigins = 4096

// A bounded L1 cache also works when Redis is unavailable. All operations are
// serialized so concurrent inserts cannot exceed the cap.
type openAICodexTurnStateOrigins struct {
	mu      sync.Mutex
	entries map[string]openAICodexTurnStateOrigin
}

func (m *openAICodexTurnStateOrigins) Load(key string) (openAICodexTurnStateOrigin, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.entries[key]
	return value, ok
}
func (m *openAICodexTurnStateOrigins) Store(key string, value openAICodexTurnStateOrigin) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.entries == nil {
		m.entries = make(map[string]openAICodexTurnStateOrigin)
	}
	if _, exists := m.entries[key]; !exists && len(m.entries) >= openAICodexTurnStateMaxOrigins {
		for victim := range m.entries {
			delete(m.entries, victim)
			break
		}
	}
	m.entries[key] = value
}
func (m *openAICodexTurnStateOrigins) Delete(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.entries, key)
}

// OpenAICodexTurnStateCache is optional so existing GatewayCache implementations
// remain compatible. Redis implementations share provenance across HTTP/WS replicas.
type OpenAICodexTurnStateCache interface {
	GetOpenAICodexTurnStateOwner(context.Context, string) (string, error)
	BindOpenAICodexTurnStateOwner(context.Context, string, string, time.Duration) error
}

func openAICodexTurnStateOwner(c *gin.Context, account *Account) string {
	source := codexAccountIdentitySource(c, account)
	if source == nil {
		return ""
	}
	namespace := codexAccountIdentityNamespace(source)
	if namespace == "" {
		if source.ID <= 0 {
			return ""
		}
		namespace = fmt.Sprintf("local-account:%d", source.ID)
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(namespace)))
}

func openAICodexTurnStateKey(c *gin.Context, state string) string {
	// Tenant + blob also protects a known blob if the client changes its thread.
	// No raw state, session identifier, token or prompt is stored in Redis keys.
	if c == nil || getAPIKeyIDFromContext(c) <= 0 || strings.TrimSpace(state) == "" {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s", getAPIKeyIDFromContext(c), strings.TrimSpace(state)))))
}

func openAICodexTurnStateContext(c *gin.Context) (context.Context, context.CancelFunc) {
	ctx := context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}
	return context.WithTimeout(ctx, 100*time.Millisecond)
}

// relayOpenAICodexTurnState 将上游响应中的 turn-state 显式写入下游响应头，
// 并记录铸造账号。必须在响应头提交点调用（WriteHeader 之前、且确认本次
// 上游响应就是将要写回客户端的响应之后）。上游无该头时主动清除 writer 上
// 可能残留的上一 failover attempt 的值——否则换号后旧账号的 blob 会粘到
// 新账号的响应上，这正是本文件要防止的跨账号矛盾。
func (s *OpenAIGatewayService) relayOpenAICodexTurnState(c *gin.Context, account *Account, upstream http.Header) {
	if c == nil || c.Writer == nil {
		return
	}
	canonical := http.CanonicalHeaderKey(openAICodexTurnStateHeader)
	state := extractOpenAICodexTurnState(upstream)
	if state == "" {
		c.Writer.Header().Del(canonical)
		return
	}
	c.Writer.Header().Set(canonical, state)
	s.noteOpenAICodexTurnStateProvenance(c, account, state)
}

// stageOpenAICodexTurnState 将上游 turn-state 暂存到延迟提交的响应头集合
// （首输出守卫路径先缓存头、见到首个输出事件才提交）。此处**不**记录铸造
// 账号：该 attempt 仍可能在首输出超时后 failover，暂存头会被整体丢弃，
// 客户端从未收到该 blob。溯源必须在真正提交时记录，见
// noteStagedOpenAICodexTurnStateCommitted。
func stageOpenAICodexTurnState(dst *http.Header, upstream http.Header) {
	if dst == nil {
		return
	}
	canonical := http.CanonicalHeaderKey(openAICodexTurnStateHeader)
	state := extractOpenAICodexTurnState(upstream)
	if state == "" {
		if *dst != nil {
			dst.Del(canonical)
		}
		return
	}
	if *dst == nil {
		*dst = http.Header{}
	}
	dst.Set(canonical, state)
}

// noteStagedOpenAICodexTurnStateCommitted 在暂存响应头真正写入下游时记录
// 铸造账号——只有此刻客户端才确定收到了该 blob，溯源表才与客户端持有的
// 值一致（否则被 failover 丢弃的 attempt 会污染溯源，导致后续误剥离）。
func (s *OpenAIGatewayService) noteStagedOpenAICodexTurnStateCommitted(c *gin.Context, account *Account, staged http.Header) {
	if staged == nil || strings.TrimSpace(staged.Get(openAICodexTurnStateHeader)) == "" {
		return
	}
	s.noteOpenAICodexTurnStateProvenance(c, account, extractOpenAICodexTurnState(staged))
}

func extractOpenAICodexTurnState(upstream http.Header) string {
	if upstream == nil {
		return ""
	}
	return strings.TrimSpace(upstream.Get(openAICodexTurnStateHeader))
}

// noteOpenAICodexTurnStateProvenance records the actual value issued by the
// selected upstream. Access-token refreshes and credential shadows retain ownership.
func (s *OpenAIGatewayService) noteOpenAICodexTurnStateProvenance(c *gin.Context, account *Account, state string) {
	if s == nil || account == nil {
		return
	}
	key, owner := openAICodexTurnStateKey(c, state), openAICodexTurnStateOwner(c, account)
	if key == "" || owner == "" {
		return
	}
	ttl := s.openAIWSSessionStickyTTL()
	if ttl <= 0 {
		ttl = time.Hour
	}
	s.openaiCodexTurnStateOrigins.Store(key, openAICodexTurnStateOrigin{accountID: account.ID, owner: owner, expiresAt: time.Now().Add(min(ttl, time.Minute))})
	s.sweepOpenAICodexTurnStateOrigins()
	if shared, ok := s.cache.(OpenAICodexTurnStateCache); ok {
		ctx, cancel := openAICodexTurnStateContext(c)
		defer cancel()
		_ = shared.BindOpenAICodexTurnStateOwner(ctx, key, owner, ttl)
	}
}

func (s *OpenAIGatewayService) isForeignOpenAICodexTurnState(c *gin.Context, account *Account, state string) bool {
	if s == nil || account == nil {
		return false
	}
	key, owner := openAICodexTurnStateKey(c, state), openAICodexTurnStateOwner(c, account)
	if key == "" || owner == "" {
		return false
	}
	if origin, ok := s.openaiCodexTurnStateOrigins.Load(key); ok {
		if time.Now().Before(origin.expiresAt) {
			return origin.owner != owner
		}
		s.openaiCodexTurnStateOrigins.Delete(key)
	}
	if shared, ok := s.cache.(OpenAICodexTurnStateCache); ok {
		ctx, cancel := openAICodexTurnStateContext(c)
		defer cancel()
		origin, err := shared.GetOpenAICodexTurnStateOwner(ctx, key)
		if err == nil && origin != "" {
			// Do not extend a Redis record's lifetime in L1 on reads.
			return origin != owner
		}
	}
	// Unknown provenance (including cache failure) is not proof of a mismatch.
	return false
}

func (s *OpenAIGatewayService) guardOpenAICodexTurnStateEcho(c *gin.Context, account *Account, h http.Header) {
	if s.isForeignOpenAICodexTurnState(c, account, h.Get(openAICodexTurnStateHeader)) {
		h.Del(openAICodexTurnStateHeader)
	}
}

func (s *OpenAIGatewayService) guardOpenAICodexTurnStateBody(c *gin.Context, account *Account, body []byte) ([]byte, error) {
	const path = "client_metadata." + openAICodexTurnStateHeader
	state := gjson.GetBytes(body, path)
	if state.Type != gjson.String || !s.isForeignOpenAICodexTurnState(c, account, state.String()) {
		return body, nil
	}
	return sjson.DeleteBytes(body, path)
}

func (s *OpenAIGatewayService) guardOpenAICodexTurnStatePayload(c *gin.Context, account *Account, payload map[string]any) {
	switch metadata := payload["client_metadata"].(type) {
	case map[string]any:
		state, _ := metadata[openAICodexTurnStateHeader].(string)
		if s.isForeignOpenAICodexTurnState(c, account, state) {
			cloned := make(map[string]any, len(metadata))
			for k, v := range metadata {
				if k != openAICodexTurnStateHeader {
					cloned[k] = v
				}
			}
			payload["client_metadata"] = cloned
		}
	case map[string]string:
		if s.isForeignOpenAICodexTurnState(c, account, metadata[openAICodexTurnStateHeader]) {
			cloned := make(map[string]string, len(metadata))
			for k, v := range metadata {
				if k != openAICodexTurnStateHeader {
					cloned[k] = v
				}
			}
			payload["client_metadata"] = cloned
		}
	}
}

// applyOpenAICodexTurnStatePayload follows the native WS contract: turn state
// belongs to response.create.client_metadata, not the websocket upgrade. A
// frame's explicit value wins; a state known to belong to another credential
// is removed before the owned fallback is considered.
func (s *OpenAIGatewayService) applyOpenAICodexTurnStatePayload(c *gin.Context, account *Account, payload map[string]any, state string) {
	s.guardOpenAICodexTurnStatePayload(c, account, payload)
	state = strings.TrimSpace(state)
	if state == "" || s.isForeignOpenAICodexTurnState(c, account, state) {
		return
	}
	switch metadata := payload["client_metadata"].(type) {
	case map[string]any:
		if _, exists := metadata[openAICodexTurnStateHeader]; exists {
			return
		}
		cloned := make(map[string]any, len(metadata)+1)
		for key, value := range metadata {
			cloned[key] = value
		}
		cloned[openAICodexTurnStateHeader] = state
		payload["client_metadata"] = cloned
	case map[string]string:
		if _, exists := metadata[openAICodexTurnStateHeader]; exists {
			return
		}
		cloned := make(map[string]any, len(metadata)+1)
		for key, value := range metadata {
			cloned[key] = value
		}
		cloned[openAICodexTurnStateHeader] = state
		payload["client_metadata"] = cloned
	case nil:
		payload["client_metadata"] = map[string]any{openAICodexTurnStateHeader: state}
	}
}

func (s *OpenAIGatewayService) applyOpenAICodexTurnStateBody(c *gin.Context, account *Account, body []byte, state string) ([]byte, error) {
	guarded, err := s.guardOpenAICodexTurnStateBody(c, account, body)
	if err != nil {
		return body, err
	}
	state = strings.TrimSpace(state)
	if state == "" || s.isForeignOpenAICodexTurnState(c, account, state) {
		return guarded, nil
	}
	metadata := gjson.GetBytes(guarded, "client_metadata")
	if metadata.Exists() && !metadata.IsObject() {
		return guarded, nil
	}
	if gjson.GetBytes(guarded, "client_metadata."+openAICodexTurnStateHeader).Exists() {
		return guarded, nil
	}
	updated, err := sjson.SetBytes(guarded, "client_metadata."+openAICodexTurnStateHeader, state)
	if err != nil {
		return guarded, err
	}
	return updated, nil
}

// Bound cleanup work per write as well as total retained memory.
func (s *OpenAIGatewayService) sweepOpenAICodexTurnStateOrigins() {
	if s.openaiCodexTurnStateWrites.Add(1)%256 != 0 {
		return
	}
	m := &s.openaiCodexTurnStateOrigins
	m.mu.Lock()
	defer m.mu.Unlock()
	now, scanned := time.Now(), 0
	for key, origin := range m.entries {
		if !now.Before(origin.expiresAt) {
			delete(m.entries, key)
		}
		scanned++
		if scanned >= 128 {
			break
		}
	}
}

// Cache injection is stricter than echoing unknown client history: only inject a
// state whose issuer matches the selected credential. Legacy unowned entries miss.
type openAIWSOwnedTurnStateStore interface {
	BindSessionTurnStateForOwner(int64, string, string, string, time.Duration)
	GetSessionTurnStateForOwner(int64, string, string) (string, bool)
}

func (s *OpenAIGatewayService) bindOpenAIWSTurnState(c *gin.Context, account *Account, store OpenAIWSStateStore, groupID int64, scope, state string) {
	if owned, ok := store.(openAIWSOwnedTurnStateStore); ok {
		owned.BindSessionTurnStateForOwner(groupID, scope, state, openAICodexTurnStateOwner(c, account), s.openAIWSSessionStickyTTL())
	}
}
func getOwnedOpenAIWSTurnState(c *gin.Context, account *Account, store OpenAIWSStateStore, groupID int64, scope string) (string, bool) {
	if owned, ok := store.(openAIWSOwnedTurnStateStore); ok {
		return owned.GetSessionTurnStateForOwner(groupID, scope, openAICodexTurnStateOwner(c, account))
	}
	return "", false
}
