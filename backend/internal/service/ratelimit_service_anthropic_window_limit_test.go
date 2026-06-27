//go:build unit

package service

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type anthropicWindowLimitRepo struct {
	mockAccountRepoForGemini
	rateLimitCalls          int
	tempUnschedCalls        int
	sessionWindowCalls      int
	lastRateLimitReset      time.Time
	lastSessionWindowStart  *time.Time
	lastSessionWindowEnd    *time.Time
	lastSessionWindowStatus string
}

func (r *anthropicWindowLimitRepo) SetRateLimited(_ context.Context, _ int64, resetAt time.Time) error {
	r.rateLimitCalls++
	r.lastRateLimitReset = resetAt
	return nil
}

func (r *anthropicWindowLimitRepo) SetTempUnschedulable(_ context.Context, _ int64, _ time.Time, _ string) error {
	r.tempUnschedCalls++
	return nil
}

func (r *anthropicWindowLimitRepo) UpdateSessionWindow(_ context.Context, _ int64, start, end *time.Time, status string) error {
	r.sessionWindowCalls++
	r.lastSessionWindowStart = start
	r.lastSessionWindowEnd = end
	r.lastSessionWindowStatus = status
	return nil
}

func TestHandleUpstreamError_AnthropicWindowLimitPreemptsTempUnschedRule(t *testing.T) {
	resetAt := time.Unix(time.Now().Add(3*time.Hour).Unix(), 0)
	headers := http.Header{}
	headers.Set("anthropic-ratelimit-unified-5h-utilization", "1.02")
	headers.Set("anthropic-ratelimit-unified-5h-reset", strconv.FormatInt(resetAt.Unix(), 10))

	repo := &anthropicWindowLimitRepo{}
	svc := NewRateLimitService(repo, nil, nil, nil, nil)
	account := &Account{
		ID:       42,
		Type:     AccountTypeOAuth,
		Platform: PlatformAnthropic,
		Credentials: map[string]any{
			"temp_unschedulable_enabled": true,
			"temp_unschedulable_rules": []any{
				map[string]any{
					"error_code":       float64(http.StatusTooManyRequests),
					"keywords":         []any{"rate limit"},
					"duration_minutes": float64(10),
				},
			},
		},
	}

	shouldDisable := svc.HandleUpstreamError(
		context.Background(),
		account,
		http.StatusTooManyRequests,
		headers,
		[]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"This request would exceed your account's rate limit. Please try again later."}}`),
	)

	require.False(t, shouldDisable)
	require.Zero(t, repo.tempUnschedCalls, "official Anthropic window limits should not be shortened by local temp-unsched rules")
	require.Equal(t, 1, repo.rateLimitCalls)
	require.Equal(t, resetAt, repo.lastRateLimitReset)
	require.Equal(t, 1, repo.sessionWindowCalls)
	require.NotNil(t, repo.lastSessionWindowStart)
	require.NotNil(t, repo.lastSessionWindowEnd)
	require.Equal(t, resetAt, *repo.lastSessionWindowEnd)
	require.Equal(t, "rejected", repo.lastSessionWindowStatus)
}

func TestSelectAnthropicExhaustedWindowPrefersSevenDayWindow(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	reset5h := now.Add(2 * time.Hour)
	reset7d := now.Add(4 * 24 * time.Hour)
	headers := http.Header{}
	headers.Set("anthropic-ratelimit-unified-5h-utilization", "1.01")
	headers.Set("anthropic-ratelimit-unified-5h-reset", strconv.FormatInt(reset5h.Unix(), 10))
	headers.Set("anthropic-ratelimit-unified-7d-surpassed-threshold", "true")
	headers.Set("anthropic-ratelimit-unified-7d-reset", strconv.FormatInt(reset7d.UnixMilli(), 10))

	limit := selectAnthropicExhaustedWindow(headers, now)
	require.NotNil(t, limit)
	require.Equal(t, "7d", limit.window)
	require.Equal(t, reset7d, limit.resetAt)
	require.Equal(t, "anthropic_7d_window_exhausted", limit.reason)
}

func TestShouldPersistAnthropicWindowLimitKeepsLongerExistingCooldown(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	existing := now.Add(4 * time.Hour)
	limit := &anthropicWindowLimit{window: "5h", resetAt: now.Add(2 * time.Hour)}
	account := &Account{RateLimitResetAt: &existing}

	require.False(t, shouldPersistAnthropicWindowLimit(account, limit, now))

	limit.resetAt = now.Add(5 * time.Hour)
	require.True(t, shouldPersistAnthropicWindowLimit(account, limit, now))
}
