package service

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// OpenAIWSMappedRetryReason identifies a pre-output WS failure owned by the
// HTTP ingress mapped-retry budget. It must not enter WS reconnect/fallback.
const OpenAIWSMappedRetryReason GatewayFailureReason = "oauth_ws_preoutput_mapped_retry"

// OpenAIWSMappedRetryPassthroughStatus keeps rule matching consistent with the
// pre-output WS interceptor. The client-facing mapped status can differ from
// the upstream event's semantic status (for example, overload maps 503 to 502).
func OpenAIWSMappedRetryPassthroughStatus(failoverErr *UpstreamFailoverError) int {
	if failoverErr == nil {
		return 0
	}
	if failoverErr.Reason != OpenAIWSMappedRetryReason {
		return failoverErr.StatusCode
	}
	return openAIStreamFailedEventSemanticStatus(failoverErr.ResponseBody, failoverErr.ClientMessage)
}

// openAIWSMappedRetryOutputObserved is deliberately conservative about tool
// and output items, even when the streaming writer has buffered them. Metadata
// such as response.created alone does not make an attempt billable or visible.
func openAIWSMappedRetryOutputObserved(eventType string, payload []byte) bool {
	if isOpenAIWSTokenEvent(eventType) {
		return true
	}
	switch eventType {
	case "response.output_item.added", "response.output_item.done", "response.content_part.added", "response.content_part.done":
		return true
	}
	if gjson.GetBytes(payload, "response.output.#").Int() > 0 || gjson.GetBytes(payload, "output.#").Int() > 0 {
		return true
	}
	if bytes.Contains(payload, []byte(`"usage"`)) {
		if usage, ok := extractOpenAIUsageFromJSONBytes(payload); ok && openAIUsageHasTokens(&usage) {
			return true
		}
	}
	return false
}

// newOpenAIWSMappedRetryError hands an eligible WS server error to the existing
// HTTP ingress retry owner before either the error or buffered lifecycle events
// reach the client. Native WS ingress and non-OAuth accounts never use it.
func (s *OpenAIGatewayService) newOpenAIWSMappedRetryError(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	headers http.Header,
	payload []byte,
	outputObserved bool,
) *UpstreamFailoverError {
	if s == nil || account == nil || account.Platform != PlatformOpenAI || account.Type != AccountTypeOAuth ||
		c == nil || c.Request == nil || c.Writer == nil || !oauthMappedRetryActive(c) || outputObserved ||
		c.Writer.Written() || IsResponseCommitted(c) || (ctx != nil && ctx.Err() != nil) || c.Request.Context().Err() != nil ||
		GetOpsCyberPolicy(c) != nil || c.GetBool(OpsClientBusinessLimitedKey) {
		return nil
	}
	value, _ := c.Get("oauth_mapped_retry_settings")
	settings, ok := value.(OAuthRetrySettings)
	if !ok || !settings.Enabled {
		return nil
	}
	eventType := gjson.GetBytes(payload, "type").String()
	if eventType != "error" && eventType != "response.failed" {
		return nil
	}
	message := extractOpenAISSEErrorMessage(payload)
	body := openAIStreamFailedEventPassthroughBody(payload, message)
	code, errorType, _ := parseOpenAIWSErrorEventFields(body)
	// Keep request/auth/rate-limit failures and protocol-specific recovery on
	// their established paths, even if an outer envelope calls them 5xx.
	if isOpenAIUpstreamAccessStateError(message, payload) || openAIStreamCredentialAuthFailure(payload) ||
		openAIStreamFailedEventSemanticStatus(payload, message) < http.StatusInternalServerError ||
		isOpenAIWSRateLimitError(code, errorType, message) ||
		!openAIStreamFailedEventShouldFailover(payload, message) {
		return nil
	}
	if reason, fallback := classifyOpenAIWSErrorEventFromRaw(code, errorType, message); fallback && reason != "upstream_error_event" {
		return nil
	}
	serverStatus := false
	for _, path := range []string{"response.error.status_code", "response.error.status", "error.status_code", "error.status", "status_code", "status"} {
		status := int(gjson.GetBytes(payload, path).Int())
		if status != 0 && (status < http.StatusInternalServerError || status == 529) {
			return nil
		}
		serverStatus = serverStatus || status >= http.StatusInternalServerError
	}
	capacity := isOpenAIRequestScopedCapacityShed(message, payload) || isOpenAITransientCapacityError(message, body)
	serverSignal := strings.ToLower(code + " " + errorType)
	if !serverStatus && !capacity && !strings.Contains(serverSignal, "server_error") &&
		!strings.Contains(serverSignal, "internal_error") && !strings.Contains(serverSignal, "upstream_error") &&
		!strings.Contains(serverSignal, "service_unavailable") {
		return nil
	}
	// This preserves the WS gateway's mapped status (overload is 502), rather
	// than borrowing Operations' semantic display status or the HTTP fallback.
	status := openAIWSErrorHTTPStatusFromRaw(code, errorType)
	matched := false
	for _, configured := range settings.StatusCodes {
		matched = matched || configured == status
	}
	if !matched || status < http.StatusInternalServerError {
		return nil
	}
	if ruleStatus, _, _, ruleMatched := applyOpenAIStreamFailedErrorPassthroughRule(c, account.Platform, payload, message); ruleMatched && ruleStatus != status {
		return nil
	}
	message = sanitizeUpstreamErrorMessage(strings.TrimSpace(message))
	if message == "" {
		message = "Upstream websocket error"
	}
	detail := ""
	if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
		}
		detail = truncateString(string(payload), maxBytes)
	}
	setOpsUpstreamError(c, status, message, detail)
	proxyID, proxyName := opsUpstreamWSProxyAttribution(account)
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		ProxyID:              proxyID,
		ProxyName:            proxyName,
		Platform:             account.Platform,
		AccountID:            account.ID,
		AccountName:          account.Name,
		UpstreamStatusCode:   status,
		UpstreamRequestID:    headers.Get("x-request-id"),
		Kind:                 "ws_error",
		Message:              message,
		Detail:               detail,
		UpstreamResponseBody: detail,
	})
	requestID, _ := c.Request.Context().Value(ctxkey.RequestID).(string)
	slog.Info("openai.ws_mapped_retry_intercepted", "request_id", requestID,
		"account_id", account.ID, "event_type", eventType, "status", status)
	return &UpstreamFailoverError{
		StatusCode:             status,
		ClientStatusCode:       status,
		ClientMessage:          message,
		ResponseBody:           body,
		ResponseHeaders:        headers.Clone(),
		RetryableOnSameAccount: true,
		RequestScopedTransient: capacity,
		Stage:                  GatewayFailureStageInference,
		Reason:                 OpenAIWSMappedRetryReason,
		NextAccountAction:      NextAccountRetry,
	}
}
