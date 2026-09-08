package service

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/google/uuid"
)

type oauthRetryLogContextKey struct{}

type oauthRetryLogContext struct {
	RequestID string
	AccountID int64
	RetryID   string
}

func withOAuthRetryLogContext(upstream, downstream context.Context, accountID int64) context.Context {
	requestID, _ := downstream.Value(ctxkey.RequestID).(string)
	return context.WithValue(upstream, oauthRetryLogContextKey{}, oauthRetryLogContext{
		RequestID: requestID, AccountID: accountID, RetryID: uuid.NewString(),
	})
}

// Only identifiers and retry decisions are logged, never bodies, tokens or headers.
func slogOAuthRetry(r *http.Request, event string, status, retry, max int, reason string) {
	meta, _ := r.Context().Value(oauthRetryLogContextKey{}).(oauthRetryLogContext)
	slog.Info("oauth retry event", "component", "oauth_retry", "event", event,
		"status", status, "retry", retry, "max_retries", max, "source", "http", "reason", reason,
		"request_id", meta.RequestID, "account_id", meta.AccountID, "platform", PlatformOpenAI,
		"retry_id", meta.RetryID)
}
