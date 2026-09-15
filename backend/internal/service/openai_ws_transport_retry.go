package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
)

const oauthMappedWSTransportRetryStateKey = "oauth_mapped_ws_transport_retry_state"

// This state belongs to one account's mapped-retry invocation. Once transport
// recovery chooses HTTP, the remaining attempts must not start another WS/HTTP
// cycle. A subsequent account gets its own WS decision and retry budget.
type oauthMappedWSTransportRetryState struct {
	accountID      int64
	settings       OAuthRetrySettings
	attempt        int
	wsRecoveryUsed bool
	forceNewConn   bool
	forceHTTP      bool
	fallbackReason string
}

func oauthMappedWSTransportState(c *gin.Context, accountID int64) *oauthMappedWSTransportRetryState {
	if !oauthMappedRetryActive(c) {
		return nil
	}
	value, _ := c.Get(oauthMappedWSTransportRetryStateKey)
	state, ok := value.(*oauthMappedWSTransportRetryState)
	if !ok || state == nil || state.accountID != accountID || !state.settings.Enabled {
		return nil
	}
	return state
}

func openAIWSMappedTransportCanReconnect(reason string) bool {
	switch strings.TrimPrefix(reason, "prewarm_") {
	case "read_event", "write_request", "write", "dial_failed", "acquire_timeout", "acquire_conn",
		"conn_queue_full", "upstream_5xx", "missing_final_response", "ws_connection_limit_reached":
		return true
	default:
		return false
	}
}

// prepareOpenAIWSMappedTransportRetry schedules recovery through the existing
// owner instead of nesting a WS reconnect loop inside every mapped attempt.
// Reserve the last available attempt for HTTP. If the budget is already empty,
// retain the existing single protocol fallback; forceHTTP prevents repetition.
func (s *OpenAIGatewayService) prepareOpenAIWSMappedTransportRetry(ctx context.Context, c *gin.Context, account *Account, reason string, wsErr error) (error, bool) {
	if account == nil || c == nil || c.Request == nil || c.Writer == nil || OpenAIStreamHasCommittedOutput(c) ||
		IsResponseCommitted(c) || (ctx != nil && ctx.Err() != nil) || c.Request.Context().Err() != nil ||
		GetOpsCyberPolicy(c) != nil || c.GetBool(OpsClientBusinessLimitedKey) {
		return nil, false
	}
	state := oauthMappedWSTransportState(c, account.ID)
	if state == nil || state.forceHTTP || !shouldFallbackOpenAIWSToHTTP(reason) {
		return nil, false
	}
	matched := false
	for _, code := range state.settings.StatusCodes {
		matched = matched || code == http.StatusBadGateway
	}
	if !matched {
		return nil, false
	}
	remaining := state.settings.MaxRetries - state.attempt
	nextTransport := OpenAIUpstreamTransportHTTPSSE
	if remaining >= 2 && !state.wsRecoveryUsed && openAIWSMappedTransportCanReconnect(reason) {
		state.wsRecoveryUsed = true
		state.forceNewConn = true
		nextTransport = OpenAIUpstreamTransportResponsesWebsocketV2
	} else {
		state.forceHTTP = true
		state.forceNewConn = false
		state.fallbackReason = reason
		s.markOpenAIWSFallbackCooling(account.ID, reason)
	}
	requestID, _ := c.Request.Context().Value(ctxkey.RequestID).(string)
	if remaining <= 0 {
		slog.Info("openai.ws_transport_http_fallback", "request_id", requestID,
			"account_id", account.ID, "reason", reason, "attempt", state.attempt,
			"max_retries", state.settings.MaxRetries, "budget_exhausted", true)
		return nil, false
	}
	message := "Upstream websocket connection failed before completing the response"
	if wsErr != nil {
		if upstreamMessage := sanitizeUpstreamErrorMessage(strings.TrimSpace(wsErr.Error())); upstreamMessage != "" {
			message = upstreamMessage
		}
	}
	body, _ := json.Marshal(map[string]any{"error": map[string]string{"type": "upstream_error", "message": message}})
	setOpsUpstreamError(c, http.StatusBadGateway, message, "")
	proxyID, proxyName := opsUpstreamWSProxyAttribution(account)
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		ProxyID: proxyID, ProxyName: proxyName, Platform: account.Platform,
		AccountID: account.ID, AccountName: account.Name,
		UpstreamStatusCode: http.StatusBadGateway, Kind: "ws_transport", Message: message,
	})
	slog.Info("openai.ws_transport_retry_scheduled", "request_id", requestID,
		"account_id", account.ID, "reason", reason, "next_transport", string(nextTransport),
		"retry", state.attempt+1, "max_retries", state.settings.MaxRetries)
	return &UpstreamFailoverError{
		StatusCode: http.StatusBadGateway, ClientStatusCode: http.StatusBadGateway,
		ClientMessage: message, ResponseBody: body, RetryableOnSameAccount: true,
		RequestScopedTransient: true, Stage: GatewayFailureStageInference,
		Reason: OpenAIWSMappedRetryReason, NextAccountAction: NextAccountRetry,
	}, true
}
