package service

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// ShouldRecordOpenAIUsage separates a no-replay result from a usage record.
// A forwarder may return an empty result after observing output solely to stop
// retries. That failed/cancelled attempt must not become an ordinary zero-usage
// request. Successful zero-usage responses and every measured billing unit keep
// their existing accounting behavior, including partial results and cyber logs.
func ShouldRecordOpenAIUsage(result *OpenAIForwardResult, forwardErr error, cyberBlocked bool) bool {
	if result == nil {
		return false
	}
	if cyberBlocked || openAIForwardResultHasMeasuredUsage(result) {
		return true
	}
	if forwardErr != nil || result.ClientDisconnect {
		return false
	}
	return result.SucceededForScheduling()
}

func openAIForwardResultHasMeasuredUsage(result *OpenAIForwardResult) bool {
	if result == nil {
		return false
	}
	return openAIUsageHasTokens(&result.Usage) || result.ImageCount > 0 ||
		result.VideoCount > 0 || result.WebSearchCalls > 0 || result.SearchCount > 0 ||
		(result.AudioUsage != nil && result.AudioUsage.DurationOrUnits > 0)
}

// MarkOpenAIForwardTerminalFailure is called only when the handler has decided
// to end the request/turn, never between retry or account-failover attempts. It
// records a failed/cancelled outcome even when no client error frame can be
// written, so an earlier hidden upstream error cannot be labelled recovered.
// Existing first-wins stream markers preserve the upstream's original message.
func MarkOpenAIForwardTerminalFailure(c *gin.Context, result *OpenAIForwardResult, forwardErr error) {
	if c == nil {
		return
	}
	clientDisconnected := (result != nil && result.ClientDisconnect) || errors.Is(forwardErr, context.Canceled)
	if c.Request != nil && errors.Is(c.Request.Context().Err(), context.Canceled) {
		clientDisconnected = true
	}
	if clientDisconnected {
		MarkOpsStreamFailure(c, "client_disconnected", "client_disconnected",
			"Client disconnected before the response was delivered: context canceled", 499)
		return
	}
	terminal := ""
	if result != nil && result.OpenAIWSMode {
		terminal = strings.TrimSpace(result.UpstreamTerminalEvent)
	}
	if forwardErr == nil && (terminal == "" || terminal == "response.completed" || terminal == "response.done") {
		return
	}
	status := http.StatusBadGateway
	code := "upstream_response_incomplete"
	message := "Upstream response ended before completion"
	if terminal != "" {
		code = strings.ReplaceAll(terminal, ".", "_")
		switch terminal {
		case "response.failed":
			message = "Upstream response failed"
		case "response.cancelled", "response.canceled":
			message = "Upstream response was cancelled"
		}
	}
	if forwardErr != nil {
		if detail := sanitizeUpstreamErrorMessage(strings.TrimSpace(forwardErr.Error())); detail != "" {
			message = detail
		}
		var failoverErr *UpstreamFailoverError
		if errors.As(forwardErr, &failoverErr) {
			if failoverErr.ClientStatusCode >= 400 {
				status = failoverErr.ClientStatusCode
			} else if failoverErr.StatusCode >= 400 {
				status = failoverErr.StatusCode
			}
			if detail := sanitizeUpstreamErrorMessage(strings.TrimSpace(failoverErr.ClientMessage)); detail != "" {
				message = detail
			}
		}
	}
	MarkOpsStreamFailure(c, "upstream_error", code, message, status)
}
