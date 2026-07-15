//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type recordingTempUnscheduleRepo struct {
	*mockAccountRepoForGemini
	calls int
}

func (r *recordingTempUnscheduleRepo) SetTempUnschedulable(context.Context, int64, time.Time, string) error {
	r.calls++
	return nil
}

func TestTempUnscheduleRetryableError_StatusPolicy(t *testing.T) {
	repo := &recordingTempUnscheduleRepo{mockAccountRepoForGemini: &mockAccountRepoForGemini{}}
	service := &GatewayService{accountRepo: repo}

	service.TempUnscheduleRetryableError(context.Background(), 1, &UpstreamFailoverError{
		StatusCode:             http.StatusTooManyRequests,
		RetryableOnSameAccount: true,
	})
	require.Zero(t, repo.calls, "helper invocation must not be confused with actual unscheduling")

	service.TempUnscheduleRetryableError(context.Background(), 1, &UpstreamFailoverError{
		StatusCode:             http.StatusBadGateway,
		RetryableOnSameAccount: true,
	})
	require.Equal(t, 1, repo.calls)
}
