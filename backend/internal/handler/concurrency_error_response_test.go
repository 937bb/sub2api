package handler

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGatewayCompatibilityConcurrencyErrorResponses(t *testing.T) {
	h := &GatewayHandler{}

	t.Run("chat completions keeps OpenAI envelope", func(t *testing.T) {
		c, rec := newHelperTestContext(http.MethodPost, "/v1/chat/completions")

		h.chatCompletionsConcurrencyErrorResponse(c, &WaitQueueFullError{SlotType: "user"}, "user", false)

		require.Equal(t, http.StatusTooManyRequests, rec.Code)
		require.JSONEq(t, `{"error":{"type":"rate_limit_error","message":"Too many pending requests, please retry later"}}`, rec.Body.String())
	})

	t.Run("responses keeps Responses envelope", func(t *testing.T) {
		c, rec := newHelperTestContext(http.MethodPost, "/v1/responses")

		h.responsesConcurrencyErrorResponse(c, &WaitQueueFullError{SlotType: "user"}, "user", false)

		require.Equal(t, http.StatusTooManyRequests, rec.Code)
		require.JSONEq(t, `{"error":{"code":"rate_limit_error","message":"Too many pending requests, please retry later"}}`, rec.Body.String())
	})
}

func TestConcurrencyErrorResponse(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		slotType    string
		wantStatus  int
		wantType    string
		wantMessage string
	}{
		{
			name:        "true concurrency timeout remains rate limit",
			err:         &ConcurrencyError{SlotType: "account", IsTimeout: true},
			slotType:    "user",
			wantStatus:  http.StatusTooManyRequests,
			wantType:    "rate_limit_error",
			wantMessage: "Concurrency limit exceeded for account, please retry later",
		},
		{
			name:        "full wait queue is rate limited",
			err:         &WaitQueueFullError{SlotType: "user"},
			slotType:    "user",
			wantStatus:  http.StatusTooManyRequests,
			wantType:    "rate_limit_error",
			wantMessage: "Too many pending requests, please retry later",
		},
		{
			name:        "client cancellation is not classified as concurrency limit",
			err:         context.Canceled,
			slotType:    "user",
			wantStatus:  statusClientClosedRequest,
			wantType:    "api_error",
			wantMessage: "context canceled",
		},
		{
			name:        "deadline exceeded is service unavailable",
			err:         context.DeadlineExceeded,
			slotType:    "user",
			wantStatus:  http.StatusServiceUnavailable,
			wantType:    "api_error",
			wantMessage: "Service temporarily unavailable, please retry later",
		},
		{
			name:        "redis acquire error is service unavailable",
			err:         errors.New("redis unavailable"),
			slotType:    "user",
			wantStatus:  http.StatusServiceUnavailable,
			wantType:    "api_error",
			wantMessage: "Service temporarily unavailable, please retry later",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, errType, message := concurrencyErrorResponse(tt.err, tt.slotType)
			require.Equal(t, tt.wantStatus, status)
			require.Equal(t, tt.wantType, errType)
			require.Equal(t, tt.wantMessage, message)
		})
	}
}
