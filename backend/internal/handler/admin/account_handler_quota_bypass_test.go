package admin

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type quotaBypassRecoveryAccountRepo struct {
	service.AccountRepository
	account                  *service.Account
	clearRateLimitCalls      int
	clearQuotaScopeCalls     int
	clearModelRateLimitCalls int
	clearTempUnschedCalls    int
}

func (r *quotaBypassRecoveryAccountRepo) GetByID(_ context.Context, id int64) (*service.Account, error) {
	if r.account != nil && r.account.ID == id {
		return r.account, nil
	}
	return nil, service.ErrAccountNotFound
}

func (r *quotaBypassRecoveryAccountRepo) ClearRateLimit(_ context.Context, _ int64) error {
	r.clearRateLimitCalls++
	r.account.RateLimitedAt = nil
	r.account.RateLimitResetAt = nil
	r.account.OverloadUntil = nil
	return nil
}

func (r *quotaBypassRecoveryAccountRepo) ClearAntigravityQuotaScopes(_ context.Context, _ int64) error {
	r.clearQuotaScopeCalls++
	return nil
}

func (r *quotaBypassRecoveryAccountRepo) ClearModelRateLimits(_ context.Context, _ int64) error {
	r.clearModelRateLimitCalls++
	return nil
}

func (r *quotaBypassRecoveryAccountRepo) ClearTempUnschedulable(_ context.Context, _ int64) error {
	r.clearTempUnschedCalls++
	return nil
}

type quotaBypassSuccessUpstream struct {
	requestBody []byte
}

func (u *quotaBypassSuccessUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.requestBody, _ = io.ReadAll(req.Body)
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(
			"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_quota_bypass\"}}\n\n",
		)),
	}, nil
}

func (u *quotaBypassSuccessUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

func TestAccountHandler_ManualTestsPreserveExisting429State(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		path string
		body string
		run  func(*AccountHandler, *gin.Context)
	}{
		{
			name: "test endpoint used by the account modal",
			path: "/api/v1/admin/accounts/77/test",
			body: `{"model_id":"gpt-5.4","mode":"quota-bypass"}`,
			run:  func(handler *AccountHandler, c *gin.Context) { handler.Test(c) },
		},
		{
			name: "quota bypass convenience endpoint",
			path: "/api/v1/admin/accounts/77/test-quota-bypass",
			run:  func(handler *AccountHandler, c *gin.Context) { handler.TestQuotaBypass(c) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := time.Now()
			resetAt := now.Add(24 * time.Hour)
			repo := &quotaBypassRecoveryAccountRepo{account: &service.Account{
				ID:               77,
				Name:             "quota-bypass-oauth",
				Platform:         service.PlatformOpenAI,
				Type:             service.AccountTypeOAuth,
				Status:           service.StatusActive,
				Schedulable:      true,
				Concurrency:      1,
				RateLimitedAt:    &now,
				RateLimitResetAt: &resetAt,
				Credentials:      map[string]any{"access_token": "oauth-token"},
				Extra:            map[string]any{"quota_bypass_enabled": true},
			}}
			upstream := &quotaBypassSuccessUpstream{}
			accountTestService := service.NewAccountTestService(repo, nil, nil, nil, nil, upstream, &config.Config{}, nil)
			rateLimitService := service.NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
			handler := NewAccountHandler(nil, nil, nil, nil, nil, nil, rateLimitService, nil, accountTestService, nil, nil, nil, nil, nil)

			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Params = gin.Params{{Key: "id", Value: "77"}}
			c.Request = httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(tt.body))
			c.Request.Header.Set("Content-Type", "application/json")

			tt.run(handler, c)

			require.Zero(t, repo.clearRateLimitCalls)
			require.Zero(t, repo.clearQuotaScopeCalls)
			require.Zero(t, repo.clearModelRateLimitCalls)
			require.Zero(t, repo.clearTempUnschedCalls)
			require.Equal(t, &resetAt, repo.account.RateLimitResetAt)
			input := gjson.GetBytes(upstream.requestBody, "input").Array()
			require.GreaterOrEqual(t, len(input), 2)
			require.Equal(t, "custom_tool_call", input[len(input)-2].Get("type").String())
			require.Equal(t, "custom_tool_call_output", input[len(input)-1].Get("type").String())
		})
	}
}
