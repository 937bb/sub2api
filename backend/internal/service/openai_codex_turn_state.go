package service

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const openAICodexTurnStateHeader = "x-codex-turn-state"

const openAICodexPreferredTurnStateLength = 292

const openAICodexTurnStateSessionHashContextKey = "openai_codex_turn_state_session_hash"

const openAICodexTurnStateModelContextKey = "openai_codex_turn_state_model"

const openAICodexTurnStateReuseScopeContextKey = "openai_codex_turn_state_reuse_scope"

const openAICodexTurnStateReuseScopeAccountModel = "account_model"

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
	model := openAICodexTurnStateModel(c)
	s.getOpenAICodexTurnStatePool().observe(state, &accountID, sessionHash, model, transport)
}

// guardOpenAICodexTurnStateEcho only replaces a 312-byte state with an
// unexpired 292-byte state minted for the same credential owner and model.
func (s *OpenAIGatewayService) guardOpenAICodexTurnStateEcho(c *gin.Context, account *Account, h http.Header, model string) {
	if s == nil || h == nil || account == nil || !account.UsesOpenAICodexProtocol() {
		return
	}
	model = stageOpenAICodexTurnStateModel(c, model)
	stageOpenAICodexTurnStateSessionHash(c, h)
	incoming := strings.TrimSpace(h.Get(openAICodexTurnStateHeader))
	if len(incoming) != 312 {
		return
	}
	owner := codexAccountIdentitySource(c, account)
	if owner == nil || owner.ID <= 0 || model == "" {
		return
	}
	if state, ok := s.getOpenAICodexTurnStatePool().preferredForBucket(owner.ID, model); ok {
		setOpenAICodexTurnStateReuse(c, h, state, openAICodexTurnStateReuseScopeAccountModel)
	}
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
