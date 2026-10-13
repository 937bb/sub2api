package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/tidwall/gjson"
)

func openAIRetryReadError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		strings.Contains(strings.ToLower(err.Error()), "connection reset") ||
		openAIRetryStreamDisconnect("OpenAI stream disconnected before completion: "+err.Error())
}

// Retry only before output, with no observed billable work. Explicit access,
// quota and capacity signals retain their existing scheduler policy.
func openAIRetryPayloadSafe(payload []byte) bool {
	if !gjson.ValidBytes(payload) {
		return false
	}
	for _, path := range []string{"output", "response.output"} {
		output := gjson.GetBytes(payload, path)
		if output.Exists() && output.Raw != "null" && (!output.IsArray() || len(output.Array()) != 0) {
			return false
		}
	}
	if usage, ok := extractOpenAIUsageFromJSONBytes(payload); ok && usage != (OpenAIUsage{}) {
		return false
	}
	message := extractOpenAISSEErrorMessage(payload)
	if openAIStreamCredentialAuthFailure(payload) || isOpenAIUpstreamAccessStateError(message, payload) || isOpenAIUpstreamCapacityShedEvent(payload) {
		return false
	}
	for _, path := range []string{"error.code", "error.type", "error.message", "response.error.code", "response.error.type", "response.error.message"} {
		value := strings.ToLower(gjson.GetBytes(payload, path).String())
		for _, marker := range []string{"account_ineligible", "rate_limit", "quota", "insufficient", "unauthorized", "authentication", "invalid_request", "permission", "policy", "content_filter", "concurrency", "no_capacity", "credential busy", "basispoints_image_request_busy", "websocket_connection_limit_reached"} {
			if strings.Contains(value, marker) {
				return false
			}
		}
	}
	status := openAIStreamFailedEventSemanticStatus(payload, message)
	return status >= 500 && status != 529
}

func isOpenAIPreOutputRetryFailure(payload []byte) bool {
	if !openAIRetryPayloadSafe(payload) {
		return false
	}
	if isOpenAIProcessingFailure(payload) {
		return true
	}
	for _, path := range []string{"error.code", "error.type", "response.error.code", "response.error.type"} {
		switch strings.ToLower(gjson.GetBytes(payload, path).String()) {
		case "server_error", "internal_error", "internal_server_error", "upstream_error", "service_unavailable":
			return true
		}
	}
	return false
}

// API-key pools retain their existing generic failure and retry policy.
func openAIAccountAllowsTransientRetry(account *Account) bool {
	return account != nil && account.Platform == PlatformOpenAI && (account.IsOAuth() || account.IsOpenAISiwc())
}

func openAIAccountRetryFailure(account *Account, payload []byte) bool {
	return isOpenAIProcessingFailure(payload) || (openAIAccountAllowsTransientRetry(account) && isOpenAIPreOutputRetryFailure(payload))
}

func openAIRetryHTTPStatus(status int) bool {
	return status == http.StatusBadGateway || status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
}

// These messages originate in our readers, never in a supplied error payload.
func openAIRetryStreamDisconnect(message string) bool {
	if message == "OpenAI stream ended before a terminal event" {
		return true
	}
	if !strings.HasPrefix(message, "OpenAI stream disconnected before completion: ") {
		return false
	}
	message = strings.ToLower(message)
	return strings.Contains(message, "eof") || strings.Contains(message, "connection reset") ||
		strings.Contains(message, "http2: stream") || strings.Contains(message, "stream error: stream id")
}
