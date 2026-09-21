package service

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const openAICodexTurnStateHeader = "x-codex-turn-state"

const (
	openAICodexTurnStateLength292 = 292
	openAICodexTurnStateLength332 = 332
)

const openAICodexTurnStateSessionHashContextKey = "openai_codex_turn_state_session_hash"

const openAICodexTurnStateModelContextKey = "openai_codex_turn_state_model"

const openAICodexTurnStateReuseScopeContextKey = "openai_codex_turn_state_reuse_scope"

const openAICodexTurnStateReuseScopeAccountModel = "account_model"

type openAICodexTurnStateOrigin struct {
	accountID int64
	expiresAt time.Time
}

func openAICodexTurnStateSeed(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	sessionID := extractClientSessionID(c.Request.Header)
	if sessionID == "" {
		return ""
	}
	return strconv.FormatInt(getAPIKeyIDFromContext(c), 10) + "\x00" + sessionID
}

func openAICodexTurnStateSessionHash(c *gin.Context) string {
	if c != nil {
		if staged := strings.TrimSpace(c.GetString(openAICodexTurnStateSessionHashContextKey)); staged != "" {
			return staged
		}
	}
	seed := openAICodexTurnStateSeed(c)
	if seed == "" {
		return ""
	}
	return hashOpenAICodexTurnState(seed)
}

func stageOpenAICodexTurnStateSessionHash(c *gin.Context, h http.Header) string {
	sessionID := ""
	if h != nil {
		sessionID = firstNonEmptyCodexHeader(h, "session-id", "session_id", "conversation_id")
	}
	if sessionID == "" {
		return openAICodexTurnStateSessionHash(c)
	}
	seed := strconv.FormatInt(getAPIKeyIDFromContext(c), 10) + "\x00" + sessionID
	sessionHash := hashOpenAICodexTurnState(seed)
	if c != nil {
		c.Set(openAICodexTurnStateSessionHashContextKey, sessionHash)
	}
	return sessionHash
}

func stageOpenAICodexTurnStateModel(c *gin.Context, model string) string {
	model = normalizeOpenAICodexTurnStateModel(model)
	if c != nil {
		c.Set(openAICodexTurnStateModelContextKey, model)
	}
	return model
}

func openAICodexTurnStateModel(c *gin.Context) string {
	if c == nil {
		return ""
	}
	return normalizeOpenAICodexTurnStateModel(c.GetString(openAICodexTurnStateModelContextKey))
}

func openAICodexTurnStateModelFromBody(body []byte) string {
	return normalizeOpenAICodexTurnStateModel(gjson.GetBytes(body, "model").String())
}

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
	s.observeOpenAICodexTurnState(c, account, state, "http")
	s.noteOpenAICodexTurnStateProvenance(c, account)
}

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

func (s *OpenAIGatewayService) noteStagedOpenAICodexTurnStateCommitted(c *gin.Context, account *Account, staged http.Header) {
	if staged == nil {
		return
	}
	state := strings.TrimSpace(staged.Get(openAICodexTurnStateHeader))
	if state == "" {
		return
	}
	s.observeOpenAICodexTurnState(c, account, state, "http")
	s.noteOpenAICodexTurnStateProvenance(c, account)
}

func extractOpenAICodexTurnState(upstream http.Header) string {
	if upstream == nil {
		return ""
	}
	return strings.TrimSpace(upstream.Get(openAICodexTurnStateHeader))
}

func (s *OpenAIGatewayService) observeOpenAICodexTurnState(c *gin.Context, account *Account, state, transport string) {
	if s == nil || account == nil || !account.UsesOpenAICodexProtocol() {
		return
	}
	owner := codexAccountIdentitySource(c, account)
	if owner == nil || owner.ID <= 0 {
		return
	}
	accountID := owner.ID
	sessionHash := openAICodexTurnStateSessionHash(c)
	model := openAICodexTurnStateModel(c)
	pool := s.getOpenAICodexTurnStatePool()
	pool.setAccountPlan(accountID, OpenAICodexStatePlanType(owner))
	pool.observe(state, &accountID, sessionHash, model, transport)
}

// guardOpenAICodexTurnStateEcho preserves a client's same-turn state, but when
// the gateway starts a turn without one it applies a model-confirmed route
// ticket minted for this credential. The state and its harvest session are a
// pair; replaying only the state lets upstream re-evaluate the route.
func (s *OpenAIGatewayService) guardOpenAICodexTurnStateEcho(c *gin.Context, account *Account, h http.Header, model string) {
	if s == nil || h == nil || account == nil || !account.UsesOpenAICodexProtocol() {
		return
	}
	model = stageOpenAICodexTurnStateModel(c, model)
	if requestScope := codexCacheOnlyHTTPExecutionScope(c, account); requestScope != "" {
		// Cache affinity is not a conversation. This must also hold if a caller
		// reapplies the guard after the final HTTP routing header was installed.
		c.Set(openAICodexTurnStateSessionHashContextKey, requestScope)
	} else {
		stageOpenAICodexTurnStateSessionHash(c, h)
	}
	s.stripForeignOpenAICodexTurnState(c, account, h)
	if strings.TrimSpace(h.Get(openAICodexTurnStateHeader)) != "" &&
		(c == nil || c.GetString(openAICodexTurnStateReuseScopeContextKey) != openAICodexTurnStateReuseScopeAccountModel) {
		return
	}
	owner := codexAccountIdentitySource(c, account)
	if owner == nil || owner.ID <= 0 || model == "" {
		return
	}
	pool := s.getOpenAICodexTurnStatePool()
	pool.setAccountPlan(owner.ID, OpenAICodexStatePlanType(owner))
	if ticket, ok := pool.preferredRecordForBucket(owner.ID, model); ok {
		setOpenAICodexTurnStateReuse(c, h, ticket.StateValue, openAICodexTurnStateReuseScopeAccountModel)
		h.Set("session-id", ticket.SourceSessionID)
	}
}

func (s *OpenAIGatewayService) openAICodexTurnStateRouteProxyURL(account *Account, h http.Header, fallback string) string {
	proxyURL, _ := s.openAICodexTurnStateRoute(account, h, fallback)
	return proxyURL
}

func (s *OpenAIGatewayService) openAICodexTurnStateRoute(account *Account, h http.Header, fallback string) (string, string) {
	if s == nil || account == nil || h == nil || !account.UsesOpenAICodexProtocol() {
		return fallback, ""
	}
	accountID := account.ID
	if account.ParentAccountID != nil && *account.ParentAccountID > 0 {
		accountID = *account.ParentAccountID
	}
	state := strings.TrimSpace(h.Get(openAICodexTurnStateHeader))
	if proxyURL, routeIPv6, ok := s.getOpenAICodexTurnStatePool().routeForState(accountID, state); ok {
		if routeIPv6 != "" {
			return "", routeIPv6
		}
		if proxyURL != "" {
			return proxyURL, ""
		}
	}
	return fallback, ""
}

func (s *OpenAIGatewayService) noteOpenAICodexTurnStateProvenance(c *gin.Context, account *Account) {
	if s == nil || account == nil {
		return
	}
	owner := codexAccountIdentitySource(c, account)
	if owner == nil || owner.ID <= 0 {
		return
	}
	seed := openAICodexTurnStateSeed(c)
	if seed == "" {
		return
	}
	s.openaiCodexTurnStateOrigins.Store(seed, openAICodexTurnStateOrigin{
		accountID: owner.ID,
		expiresAt: time.Now().Add(s.openAIWSSessionStickyTTL()),
	})
	s.sweepOpenAICodexTurnStateOrigins()
}

func (s *OpenAIGatewayService) stripForeignOpenAICodexTurnState(c *gin.Context, account *Account, h http.Header) {
	if strings.TrimSpace(h.Get(openAICodexTurnStateHeader)) == "" {
		return
	}
	seed := openAICodexTurnStateSeed(c)
	if seed == "" {
		return
	}
	raw, ok := s.openaiCodexTurnStateOrigins.Load(seed)
	if !ok {
		return
	}
	origin, ok := raw.(openAICodexTurnStateOrigin)
	if !ok || (!origin.expiresAt.IsZero() && time.Now().After(origin.expiresAt)) {
		s.openaiCodexTurnStateOrigins.Delete(seed)
		return
	}
	owner := codexAccountIdentitySource(c, account)
	if owner != nil && owner.ID > 0 && origin.accountID != owner.ID {
		h.Del(openAICodexTurnStateHeader)
	}
}

func (s *OpenAIGatewayService) sweepOpenAICodexTurnStateOrigins() {
	if s.openaiCodexTurnStateWrites.Add(1)%256 != 0 {
		return
	}
	now := time.Now()
	s.openaiCodexTurnStateOrigins.Range(func(key, value any) bool {
		origin, ok := value.(openAICodexTurnStateOrigin)
		if !ok || (!origin.expiresAt.IsZero() && now.After(origin.expiresAt)) {
			s.openaiCodexTurnStateOrigins.Delete(key)
		}
		return true
	})
}

func isReusableOpenAICodexTurnStateLength(length int) bool {
	return length == openAICodexTurnStateLength292 || length == openAICodexTurnStateLength332
}

func setOpenAICodexTurnStateReuse(c *gin.Context, h http.Header, state, scope string) {
	h.Set(openAICodexTurnStateHeader, state)
	if c != nil {
		c.Set(openAICodexTurnStateReuseScopeContextKey, scope)
	}
}

func (s *OpenAIGatewayService) setOpenAICodexTurnStateRepository(repo OpenAICodexTurnStateStore) {
	if s == nil || repo == nil {
		return
	}
	s.getOpenAICodexTurnStatePool().setRepository(context.Background(), repo)
}

func (s *OpenAIGatewayService) setOpenAICodexTurnStateScanEnqueuer(enqueue func(int64, string) bool) {
	if s == nil {
		return
	}
	s.openaiCodexTurnStateScanEnqueuer = enqueue
}

func (s *OpenAIGatewayService) hasRequiredOpenAICodexTurnState(account *Account, model string) bool {
	if account == nil || !account.RequiresOpenAICodexStateRouting() {
		return true
	}
	model = openAICodexTurnStateUpstreamModel(account, model)
	if model == "" {
		return false
	}
	accountID := account.ID
	if account.ParentAccountID != nil && *account.ParentAccountID > 0 {
		accountID = *account.ParentAccountID
	}
	if s.getOpenAICodexTurnStatePool().hasReusableStateBeyond(accountID, model, time.Now()) {
		return true
	}
	if s.openaiCodexTurnStateScanEnqueuer != nil {
		s.openaiCodexTurnStateScanEnqueuer(accountID, model)
	}
	return false
}

func (s *OpenAIGatewayService) getOpenAICodexTurnStatePool() *openAICodexTurnStatePool {
	if s == nil {
		return nil
	}
	s.openaiCodexTurnStatePoolOnce.Do(func() {
		s.openaiCodexTurnStatePool = newOpenAICodexTurnStatePool()
	})
	return s.openaiCodexTurnStatePool
}
