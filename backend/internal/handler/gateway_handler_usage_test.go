package handler

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUsageUnrestrictedIncludesWeeklyWindowStart(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/usage", nil)

	weeklyWindowStart := time.Date(2026, time.July, 13, 0, 30, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	c.Set(string(middleware.ContextKeySubscription), &service.UserSubscription{
		WeeklyWindowStart: &weeklyWindowStart,
	})

	handler := &GatewayHandler{}
	resp, err := handler.usageUnrestricted(
		c,
		context.Background(),
		&service.APIKey{Group: &service.Group{
			Name:             "Weekly plan",
			SubscriptionType: service.SubscriptionTypeSubscription,
		}},
		middleware.AuthSubject{},
	)

	require.NoError(t, err)
	c.JSON(http.StatusOK, resp)
	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Subscription struct {
			WeeklyWindowStart *time.Time `json:"weekly_window_start"`
		} `json:"subscription"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.NotNil(t, response.Subscription.WeeklyWindowStart)
	require.True(t, weeklyWindowStart.Equal(*response.Subscription.WeeklyWindowStart))
}

type usageTimeoutUserRepo struct {
	service.UserRepository
	calls atomic.Int64
}

func (r *usageTimeoutUserRepo) GetByID(ctx context.Context, id int64) (*service.User, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.calls.Add(1)
	return &service.User{ID: id, Balance: 12.34}, nil
}

func (r *usageTimeoutUserRepo) GetUserAvatar(context.Context, int64) (*service.UserAvatar, error) {
	return nil, nil
}

type usageTimeoutStatsRepo struct {
	service.UsageLogRepository
	user     *usageTimeoutUserRepo
	release  chan struct{}
	finished chan struct{}
}

func (r *usageTimeoutStatsRepo) GetAPIKeyDashboardStats(ctx context.Context, id int64) (*usagestats.UserDashboardStats, error) {
	defer close(r.finished)
	if r.user.calls.Load() != 1 {
		return nil, errors.New("statistics ran before balance")
	}
	select {
	case <-r.release:
		return &usagestats.UserDashboardStats{TotalRequests: 42}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func TestUsageSlowStatisticsPreservesFreshBalance(t *testing.T) {
	gin.SetMode(gin.TestMode)
	users := &usageTimeoutUserRepo{}
	stats := &usageTimeoutStatsRepo{user: users, release: make(chan struct{}), finished: make(chan struct{})}
	defer func() {
		close(stats.release)
		select {
		case <-stats.finished:
		case <-time.After(time.Second):
		}
	}()
	h := &GatewayHandler{userService: service.NewUserService(users, nil, nil, nil), usageService: service.NewUsageService(stats, nil, nil, nil)}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/usage", nil)
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: 11})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 21})
	h.Usage(c)
	require.Equal(t, http.StatusOK, recorder.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, 12.34, body["balance"])
	require.Equal(t, int64(1), users.calls.Load())
	require.NotContains(t, body, "usage", "timed-out statistics must not become zero totals")
	require.NotContains(t, body, "daily_usage")
	require.NotContains(t, body, "model_stats")
}
