package service

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
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

const openAICodexTurnStateRouteOutcomeContextKey = "openai_codex_turn_state_route_outcome"

type openAICodexTurnStateRouteOutcome struct {
	accountID int64
	model     string
	stateHash string
}

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
	owner := codexAccountIdentitySource(c, account)
	if owner == nil || owner.ID <= 0 || model == "" {
		s.applyOpenAICodexInfrastructureCookies(h)
		return
	}
	pool := s.getOpenAICodexTurnStatePool()
	pool.setAccountPlan(owner.ID, OpenAICodexStatePlanType(owner))
	// Strict route binding treats the scanner State, session, and acquisition
	// exit as one account/model ticket. Never let a client-carried State bypass
	// that ticket and silently fall back to another egress.
	requireRouteBinding := pool.isRouteBindingRequired()
	if !requireRouteBinding && strings.TrimSpace(h.Get(openAICodexTurnStateHeader)) != "" &&
		(c == nil || c.GetString(openAICodexTurnStateReuseScopeContextKey) != openAICodexTurnStateReuseScopeAccountModel) {
		s.applyOpenAICodexInfrastructureCookies(h)
		return
	}
	if ticket, ok := pool.preferredRecordForBucket(owner.ID, model); ok {
		setOpenAICodexTurnStateReuse(c, h, ticket.StateValue, openAICodexTurnStateReuseScopeAccountModel)
		h.Set("session-id", ticket.SourceSessionID)
		if ticket.RouteCookie != "" {
			h.Set("Cookie", ticket.RouteCookie)
		}
		stageOpenAICodexTurnStateRouteOutcome(c, owner.ID, model, ticket.StateHash)
	} else if requireRouteBinding {
		h.Del(openAICodexTurnStateHeader)
	}
	s.applyOpenAICodexInfrastructureCookies(h)
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

func (s *OpenAIGatewayService) setOpenAICodexTurnStateScanEnqueuer(enqueue func(int64, string, bool) bool) {
	if s == nil {
		return
	}
	s.openaiCodexTurnStateScanEnqueuer = enqueue
}

func resetOpenAICodexTurnStateRouteOutcome(c *gin.Context) {
	if c != nil {
		c.Set(openAICodexTurnStateRouteOutcomeContextKey, openAICodexTurnStateRouteOutcome{})
	}
}

func stageOpenAICodexTurnStateRouteOutcome(c *gin.Context, accountID int64, model, stateHash string) {
	if c == nil || accountID <= 0 {
		return
	}
	model = normalizeOpenAICodexTurnStateModel(model)
	stateHash = strings.TrimSpace(stateHash)
	if model == "" || stateHash == "" {
		return
	}
	c.Set(openAICodexTurnStateRouteOutcomeContextKey, openAICodexTurnStateRouteOutcome{
		accountID: accountID,
		model:     model,
		stateHash: stateHash,
	})
}

func (s *OpenAIGatewayService) handleOpenAICodexTurnStateRouteOutcome(c *gin.Context, result *OpenAIForwardResult) {
	if s == nil || c == nil || result == nil {
		return
	}
	raw, exists := c.Get(openAICodexTurnStateRouteOutcomeContextKey)
	if !exists {
		return
	}
	outcome, ok := raw.(openAICodexTurnStateRouteOutcome)
	actualModel := normalizeOpenAICodexTurnStateModel(result.UpstreamResponseModel)
	if !ok || outcome.accountID <= 0 || outcome.model == "" || outcome.stateHash == "" || actualModel == "" ||
		openAICodexTurnStateModelsMatch(outcome.model, actualModel) {
		return
	}
	fasterModel := normalizeOpenAICodexTurnStateModel(result.UpstreamHeaders.Get("x-codex-safety-buffering-faster-model"))
	safetyBuffered := fasterModel != "" && openAICodexTurnStateModelsMatch(fasterModel, actualModel)
	invalidated, err := s.getOpenAICodexTurnStatePool().invalidateRouteOutcome(context.WithoutCancel(c.Request.Context()), outcome.accountID, outcome.model, outcome.stateHash)
	if err != nil {
		logger.L().Error("openai_codex_state_route_invalidation_failed",
			zap.Int64("account_id", outcome.accountID),
			zap.String("requested_model", outcome.model),
			zap.String("state_hash", outcome.stateHash),
			zap.Error(err),
		)
	}
	if !invalidated {
		return
	}
	logger.L().Warn("openai_codex_state_route_model_mismatch",
		zap.Int64("account_id", outcome.accountID),
		zap.String("requested_model", outcome.model),
		zap.String("response_model", actualModel),
		zap.String("state_hash", outcome.stateHash),
		zap.Bool("safety_buffered", safetyBuffered),
	)
	if s.openaiCodexTurnStateScanEnqueuer != nil {
		s.openaiCodexTurnStateScanEnqueuer(outcome.accountID, outcome.model, true)
	}
}

func (s *OpenAIGatewayService) hasRequiredOpenAICodexTurnState(account *Account, model string) bool {
	if account == nil || !account.IsOpenAIOAuthLike() {
		return true
	}
	pool := s.getOpenAICodexTurnStatePool()
	plan := OpenAICodexStatePlanType(account)
	// Disabling acquisition for a plan also opts that plan out of the ticket
	// gate. Otherwise a disabled plan could never acquire the ticket required
	// to become schedulable.
	if !pool.isPlanScanEnabled(plan) {
		return true
	}
	if !account.RequiresOpenAICodexStateRouting() && !pool.isRouteBindingRequired() {
		return true
	}
	if !pool.isStateRequiredBeforeRouting() {
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
	pool.setAccountPlan(accountID, plan)
	if pool.hasReusableStateBeyond(accountID, model, time.Now()) {
		return true
	}
	if s.openaiCodexTurnStateScanEnqueuer != nil {
		s.openaiCodexTurnStateScanEnqueuer(accountID, model, false)
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
