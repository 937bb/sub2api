package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestSuccessfulRateLimiterCountsOnlySuccessfulResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mini := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	router := successfulRateLimitTestRouter(NewRateLimiter(rdb), 1)

	require.Equal(t, http.StatusBadRequest, performSuccessfulRateLimitRequest(router, "192.0.2.1:1001", "fail").Code)
	require.Equal(t, http.StatusBadRequest, performSuccessfulRateLimitRequest(router, "192.0.2.1:1002", "fail").Code)
	require.Equal(t, http.StatusCreated, performSuccessfulRateLimitRequest(router, "192.0.2.1:1003", "uncommitted").Code)
	require.Equal(t, http.StatusCreated, performSuccessfulRateLimitRequest(router, "192.0.2.1:1003", "success").Code)
	require.Equal(t, http.StatusTooManyRequests, performSuccessfulRateLimitRequest(router, "192.0.2.1:1004", "success").Code)
}

func TestSuccessfulRateLimiterGroupsIPv6By64(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mini := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	router := successfulRateLimitTestRouter(NewRateLimiter(rdb), 1)

	require.Equal(t, http.StatusCreated, performSuccessfulRateLimitRequest(router, "[2001:db8:1:2::1]:1001", "success").Code)
	require.Equal(t, http.StatusTooManyRequests, performSuccessfulRateLimitRequest(router, "[2001:db8:1:2::ffff]:1002", "success").Code)
	require.Equal(t, http.StatusCreated, performSuccessfulRateLimitRequest(router, "[2001:db8:1:3::1]:1003", "success").Code)
}

func TestSuccessfulRateLimiterFailsClosedForInvalidIP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mini := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	router := successfulRateLimitTestRouter(NewRateLimiter(rdb), 1)
	require.Equal(t, http.StatusTooManyRequests, performSuccessfulRateLimitRequest(router, "invalid", "success").Code)
}

func TestSuccessfulDeviceRateLimiterSurvivesProxyRotation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mini := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	limiter := NewRateLimiter(rdb)
	router := gin.New()
	router.POST("/register",
		limiter.LimitSuccessfulByDeviceWithOptions("register-device", 1, time.Hour, RateLimitOptions{
			FailureMode: RateLimitFailClose,
		}),
		func(c *gin.Context) {
			MarkSuccessfulRateLimit(c)
			c.Status(http.StatusCreated)
		},
	)

	first := httptest.NewRequest(http.MethodPost, "/register", nil)
	first.RemoteAddr = "192.0.2.1:1001"
	first.Header.Set(ClientDeviceHeader, "device-installation-aaaaaaaa")
	firstRecorder := httptest.NewRecorder()
	router.ServeHTTP(firstRecorder, first)
	require.Equal(t, http.StatusCreated, firstRecorder.Code)

	rotatedProxy := httptest.NewRequest(http.MethodPost, "/register", nil)
	rotatedProxy.RemoteAddr = "198.51.100.9:1002"
	rotatedProxy.Header.Set(ClientDeviceHeader, "device-installation-aaaaaaaa")
	rotatedRecorder := httptest.NewRecorder()
	router.ServeHTTP(rotatedRecorder, rotatedProxy)
	require.Equal(t, http.StatusTooManyRequests, rotatedRecorder.Code)

	differentDevice := httptest.NewRequest(http.MethodPost, "/register", nil)
	differentDevice.RemoteAddr = "198.51.100.9:1003"
	differentDevice.Header.Set(ClientDeviceHeader, "device-installation-bbbbbbbb")
	differentRecorder := httptest.NewRecorder()
	router.ServeHTTP(differentRecorder, differentDevice)
	require.Equal(t, http.StatusCreated, differentRecorder.Code)
}

func TestClientDeviceIdentityFallsBackToCookieAndMissingBucket(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	ctx.Request.Header.Set("User-Agent", "test-browser")
	ctx.Request.AddCookie(&http.Cookie{Name: ClientDeviceCookie, Value: "device-installation-aaaaaaaa"})
	fromCookie := ClientDeviceIdentity(ctx)

	ctx.Request.Header.Set(ClientDeviceHeader, "device-installation-aaaaaaaa")
	require.Equal(t, fromCookie, ClientDeviceIdentity(ctx))

	ctx.Request.Header.Del(ClientDeviceHeader)
	ctx.Request.Header.Del("Cookie")
	require.Equal(t, "missing", ClientDeviceIdentity(ctx))
}

func successfulRateLimitTestRouter(limiter *RateLimiter, limit int) *gin.Engine {
	router := gin.New()
	router.POST("/register",
		limiter.LimitSuccessfulWithOptions("register-success", limit, time.Hour, RateLimitOptions{
			FailureMode: RateLimitFailClose,
		}),
		func(c *gin.Context) {
			if c.Query("result") == "success" {
				MarkSuccessfulRateLimit(c)
				c.Status(http.StatusCreated)
				return
			}
			if c.Query("result") == "uncommitted" {
				c.Status(http.StatusCreated)
				return
			}
			c.Status(http.StatusBadRequest)
		},
	)
	return router
}

func performSuccessfulRateLimitRequest(router *gin.Engine, remoteAddr, result string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/register?result="+result, nil)
	req.RemoteAddr = remoteAddr
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}
