package service

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

const openAICodexTurnStateHeader = "x-codex-turn-state"

const openAICodexPreferredTurnStateLength = 292

const openAICodexTurnStateSessionHashContextKey = "openai_codex_turn_state_session_hash"

const openAICodexTurnStateReuseScopeContextKey = "openai_codex_turn_state_reuse_scope"

const (
	openAICodexTurnStateReuseScopeSession = "session"
	openAICodexTurnStateReuseScopeAccount = "account"
	openAICodexTurnStateReuseScopeGlobal  = "global"
)

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
	s.getOpenAICodexTurnStatePool().observe(state, &accountID, sessionHash, transport)
}

// guardOpenAICodexTurnStateEcho prefers an unexpired 292-byte state from the
// same credential owner and client session, then falls back to the same owner
// and finally the global pool. API-key accounts keep their original header.
func (s *OpenAIGatewayService) guardOpenAICodexTurnStateEcho(c *gin.Context, account *Account, h http.Header) {
	if s == nil || h == nil || account == nil || !account.UsesOpenAICodexProtocol() {
		return
	}
	owner := codexAccountIdentitySource(c, account)
	if owner == nil {
		h.Del(openAICodexTurnStateHeader)
		return
	}
	sessionHash := stageOpenAICodexTurnStateSessionHash(c, h)
	pool := s.getOpenAICodexTurnStatePool()
	if sessionHash != "" {
		if state, ok := pool.preferredForSession(owner.ID, sessionHash); ok {
			setOpenAICodexTurnStateReuse(c, h, state, openAICodexTurnStateReuseScopeSession)
			return
		}
	}
	if state, ok := pool.preferredForAccount(owner.ID); ok {
		setOpenAICodexTurnStateReuse(c, h, state, openAICodexTurnStateReuseScopeAccount)
		return
	}
	if state, ok := pool.preferredAcrossAccounts(); ok {
		setOpenAICodexTurnStateReuse(c, h, state, openAICodexTurnStateReuseScopeGlobal)
		return
	}
	h.Del(openAICodexTurnStateHeader)
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

func (s *OpenAIGatewayService) getOpenAICodexTurnStatePool() *openAICodexTurnStatePool {
	if s == nil {
		return nil
	}
	s.openaiCodexTurnStatePoolOnce.Do(func() {
		s.openaiCodexTurnStatePool = newOpenAICodexTurnStatePool()
	})
	return s.openaiCodexTurnStatePool
}
