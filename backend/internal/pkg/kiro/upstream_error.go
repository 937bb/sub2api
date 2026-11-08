package kiro

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
)

// UpstreamError contains no response body, URL, proxy address or credentials.
// Only allowlisted reasons may cross the admin diagnostic boundary.
type UpstreamError struct {
	Operation Operation
	Status    int
	Reason    string
}

func (e *UpstreamError) Error() string {
	stage := "operation"
	switch e.Operation {
	case OperationGeneration:
		stage = "generation"
	case OperationRefresh:
		stage = "credential refresh"
	case OperationModels:
		stage = "model discovery"
	case OperationUsage:
		stage = "usage discovery"
	}
	reason := "upstream request failed"
	switch e.Reason {
	case "invalid_model_fields":
		reason = "model control parameters were rejected; check thinking, output_config and max_tokens for the selected model"
	case "invalid_request_body":
		reason = "request body failed upstream validation"
	case "invalid_grant":
		reason = "invalid_grant; sign in to Kiro again and import fresh credentials"
	case "unauthorized":
		reason = "credentials rejected; sign in to Kiro again and import fresh credentials"
	case "forbidden":
		reason = "upstream denied access"
	case "rate_limited":
		reason = "upstream rate limited the request"
	case "proxy_auth":
		reason = "assigned static proxy requires valid authentication"
	case "timeout":
		reason = "request timed out through the assigned static proxy"
	case "canceled":
		reason = "request canceled"
	case "dns":
		reason = "DNS lookup failed through the assigned static proxy"
	case "transport":
		reason = "transport failed through the assigned static proxy"
	case "invalid_response":
		reason = "upstream returned an invalid response"
	}
	if e.Status != 0 {
		return fmt.Sprintf("Kiro %s HTTP %d: %s", stage, e.Status, reason)
	}
	return fmt.Sprintf("Kiro %s: %s", stage, reason)
}

// GenerationResponseError exposes only fixed diagnostics. Upstream messages can
// contain request data, so never return their raw text to clients or logs.
func GenerationResponseError(status int, raw []byte) *UpstreamError {
	out := &UpstreamError{Operation: OperationGeneration, Status: status}
	if status != http.StatusBadRequest {
		return out
	}
	var body struct {
		Reason  string `json:"reason"`
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &body) != nil || body.Reason != "REQUEST_BODY_INVALID" {
		return out
	}
	out.Reason = "invalid_request_body"
	if strings.HasPrefix(body.Message, "Invalid additionalModelRequestFields:") {
		out.Reason = "invalid_model_fields"
	}
	return out
}

func ResponseError(operation Operation, response *http.Response) *UpstreamError {
	out := &UpstreamError{Operation: operation, Status: response.StatusCode}
	switch response.StatusCode {
	case http.StatusUnauthorized:
		out.Reason = "unauthorized"
	case http.StatusForbidden:
		out.Reason = "forbidden"
	case http.StatusProxyAuthRequired:
		out.Reason = "proxy_auth"
	case http.StatusTooManyRequests:
		out.Reason = "rate_limited"
	}
	// Preserve invalid_grant for the shared refresh-race recovery path without
	// retaining or exposing upstream error_description (which can echo secrets).
	if operation == OperationRefresh && response.StatusCode == http.StatusBadRequest {
		var body struct {
			Error string `json:"error"`
		}
		data, err := io.ReadAll(io.LimitReader(response.Body, (32<<10)+1))
		if err == nil && len(data) <= 32<<10 && json.Unmarshal(data, &body) == nil && strings.EqualFold(body.Error, "invalid_grant") {
			out.Reason = "invalid_grant"
		}
	}
	return out
}

func TransportError(operation Operation, err error) *UpstreamError {
	out := &UpstreamError{Operation: operation, Reason: "transport"}
	var network net.Error
	var dns *net.DNSError
	switch {
	case errors.Is(err, context.Canceled):
		out.Reason = "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		out.Reason = "timeout"
	case errors.As(err, &dns):
		out.Reason = "dns"
	case errors.As(err, &network) && network.Timeout():
		out.Reason = "timeout"
	}
	return out
}
