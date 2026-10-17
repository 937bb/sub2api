package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type siwcCooldownRepo struct {
	AccountRepository
	calls  int
	until  time.Time
	reason string
}

func (r *siwcCooldownRepo) SetTempUnschedulable(_ context.Context, _ int64, until time.Time, reason string) error {
	r.calls++
	r.until, r.reason = until, reason
	return nil
}

func TestSIWCSharingErrorsPreserveSemanticStatus(t *testing.T) {
	for code, status := range map[string]int{
		siwcUsageLimitCode:                            429,
		"subscription_sharing_usage_unavailable":      503,
		"subscription_sharing_unsupported_capability": 400,
		"subscription_sharing_route_not_supported":    403,
	} {
		for _, shape := range []string{"error", "response.failed", "flat"} {
			t.Run(code+"/"+shape, func(t *testing.T) {
				failure := map[string]any{"type": "invalid_request_error", "code": code, "message": "Please try again later."}
				var event any = map[string]any{"type": "error", "error": failure}
				if shape == "response.failed" {
					event = map[string]any{"type": shape, "response": map[string]any{"error": failure}}
				}
				if shape == "flat" {
					event = map[string]any{"type": "error", "code": code, "message": "Please try again later."}
				}
				body, err := json.Marshal(event)
				require.NoError(t, err)
				require.Equal(t, status, openAIStreamFailureStatus(body, "Please try again later."))
				require.Equal(t, status == 429 || status == 503, openAIStreamFailedEventShouldFailover(body, "Please try again later."))
				require.Equal(t, status == 429 || status == 503, openAIStreamErrorEventShouldFailover(body, "Please try again later."))
				if status == 429 || status == 503 {
					failure := newOpenAIUpstreamFailoverError(502, nil, body, "", true)
					require.Equal(t, status, failure.ClientStatusCode)
					require.False(t, failure.RetryableOnSameAccount)
				}
			})
		}
	}
	require.Zero(t, siwcErrorStatus([]byte(`{"error":{"code":"invalid_value","message":"subscription_sharing_usage_limit_exceeded"}}`)))
}

func TestSIWCStreamQuotaFailureDoesNotReplayCommittedOutput(t *testing.T) {
	for _, committed := range []bool{false, true} {
		repo := &siwcCooldownRepo{}
		svc := newOpenAIRejectedFieldTestService(nil)
		svc.rateLimitService = &RateLimitService{accountRepo: repo}
		calls := 0
		svc.siwcTransport = siwcCatalogRoundTripper(func(*http.Request) (*http.Response, error) {
			calls++
			body := "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_test\"}}\n\n"
			if committed {
				body += "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n"
			}
			body += "data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"type\":\"invalid_request_error\",\"code\":\"subscription_sharing_usage_limit_exceeded\",\"message\":\"try again\"}}}\n\n"
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
		})
		body := []byte(`{"model":"gpt-6-astra","input":"hi","store":false,"stream":true}`)
		c := newOpenAIRejectedFieldTestContext(body)
		_, err := svc.Forward(context.Background(), c, siwcFixtureAccount(), body)
		require.Error(t, err)
		require.Equal(t, 1, calls)
		require.Equal(t, 1, repo.calls)
		var failover *UpstreamFailoverError
		require.Equal(t, !committed, errors.As(err, &failover))
		if !committed {
			require.Equal(t, 429, failover.ClientStatusCode)
		}
	}
}

func TestSIWCUsageLimitPausesOnlySharingAccountWithoutQuotaReset(t *testing.T) {
	for _, stream := range []bool{false, true} {
		repo := &siwcCooldownRepo{}
		svc := &OpenAIGatewayService{rateLimitService: &RateLimitService{accountRepo: repo}}
		account := siwcFixtureAccount()
		body := []byte(`{"error":{"type":"invalid_request_error","code":"subscription_sharing_usage_limit_exceeded","message":"The ChatGPT user has reached their Subscription Sharing usage limit. Ask the user to try again after their usage limit resets or use an API key instead."}}`)
		before := time.Now()
		if stream {
			status, disabled := svc.handleOpenAIStreamTerminalAccountSideEffects(nil, account, body, "", nil)
			require.Equal(t, http.StatusTooManyRequests, status)
			require.True(t, disabled)
		} else {
			require.True(t, svc.handleOpenAIAccountUpstreamError(context.Background(), account, 429, nil, body))
		}
		require.Equal(t, 1, repo.calls)
		require.WithinDuration(t, before.Add(time.Minute), repo.until, 2*time.Second)
		require.Contains(t, repo.reason, "Provider reset time is unknown")
		require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
		require.Nil(t, account.RateLimitResetAt)
		require.False(t, openAIStreamFailedEventRetryableOnSameAccount(account, body, "try again"))
	}
}
