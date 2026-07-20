package middleware

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	ippkg "github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RateLimitFailureMode Redis 故障策略
type RateLimitFailureMode int

const (
	RateLimitFailOpen RateLimitFailureMode = iota
	RateLimitFailClose
)

// RateLimitOptions 限流可选配置
type RateLimitOptions struct {
	FailureMode RateLimitFailureMode
}

var rateLimitScript = redis.NewScript(`
local current = redis.call('INCR', KEYS[1])
local ttl = redis.call('PTTL', KEYS[1])
local repaired = 0
if current == 1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
elseif ttl == -1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
  repaired = 1
end
return {current, repaired}
`)

var successfulRateLimitReserveScript = redis.NewScript(`
local current = tonumber(redis.call('GET', KEYS[1]) or '0')
if current >= tonumber(ARGV[2]) then
  return {current, 0}
end
current = redis.call('INCR', KEYS[1])
local ttl = redis.call('PTTL', KEYS[1])
if current == 1 or ttl == -1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
return {current, 1}
`)

var successfulRateLimitReleaseScript = redis.NewScript(`
local current = tonumber(redis.call('GET', KEYS[1]) or '0')
if current <= 0 then
  return 0
end
return redis.call('DECR', KEYS[1])
`)

const successfulRateLimitCommittedKey = "rate_limit_successful_committed"

const (
	ClientDeviceHeader = "X-Sub2API-Device-ID"
	ClientDeviceCookie = "sub2api_device_id"
)

// MarkSuccessfulRateLimit tells successful-only limiters that the protected
// operation actually committed. HTTP status alone is insufficient for OAuth
// flows, where a 2xx response may only represent an intermediate state.
func MarkSuccessfulRateLimit(c *gin.Context) {
	if c != nil {
		c.Set(successfulRateLimitCommittedKey, true)
	}
}

// ClientDeviceIdentity returns a server-side digest of the browser signal.
// Missing or malformed signals share one intentionally strict bucket instead
// of bypassing device limits.
func ClientDeviceIdentity(c *gin.Context) string {
	if c == nil {
		return "missing"
	}
	raw := strings.TrimSpace(c.GetHeader(ClientDeviceHeader))
	if raw == "" {
		raw, _ = c.Cookie(ClientDeviceCookie)
		raw = strings.TrimSpace(raw)
	}
	if len(raw) < 16 || len(raw) > 256 {
		return "missing"
	}
	digest := sha256.Sum256([]byte(raw + "|" + c.GetHeader("User-Agent")))
	return fmt.Sprintf("%x", digest[:])
}

// rateLimitRun 允许测试覆写脚本执行逻辑
var rateLimitRun = func(ctx context.Context, client *redis.Client, key string, windowMillis int64) (int64, bool, error) {
	values, err := rateLimitScript.Run(ctx, client, []string{key}, windowMillis).Slice()
	if err != nil {
		return 0, false, err
	}
	if len(values) < 2 {
		return 0, false, fmt.Errorf("rate limit script returned %d values", len(values))
	}
	count, err := parseInt64(values[0])
	if err != nil {
		return 0, false, err
	}
	repaired, err := parseInt64(values[1])
	if err != nil {
		return 0, false, err
	}
	return count, repaired == 1, nil
}

// RateLimiter Redis 速率限制器
type RateLimiter struct {
	redis  *redis.Client
	prefix string
}

// NewRateLimiter 创建速率限制器实例
func NewRateLimiter(redisClient *redis.Client) *RateLimiter {
	return &RateLimiter{
		redis:  redisClient,
		prefix: "rate_limit:",
	}
}

// Limit 返回速率限制中间件
// key: 限制类型标识
// limit: 时间窗口内最大请求数
// window: 时间窗口
func (r *RateLimiter) Limit(key string, limit int, window time.Duration) gin.HandlerFunc {
	return r.LimitWithOptions(key, limit, window, RateLimitOptions{})
}

// LimitWithOptions 返回速率限制中间件（带可选配置）
func (r *RateLimiter) LimitWithOptions(key string, limit int, window time.Duration, opts RateLimitOptions) gin.HandlerFunc {
	failureMode := opts.FailureMode
	if failureMode != RateLimitFailClose {
		failureMode = RateLimitFailOpen
	}

	return func(c *gin.Context) {
		identity := ippkg.AbuseIdentity(c.ClientIP())
		if identity == "" {
			log.Printf("[RateLimit] invalid client IP: mode=%s", failureModeLabel(failureMode))
			if failureMode == RateLimitFailClose {
				abortRateLimit(c)
				return
			}
			c.Next()
			return
		}
		redisKey := r.prefix + key + ":" + identity

		ctx := c.Request.Context()

		windowMillis := windowTTLMillis(window)

		// 使用 Lua 脚本原子操作增加计数并设置过期
		count, repaired, err := rateLimitRun(ctx, r.redis, redisKey, windowMillis)
		if err != nil {
			log.Printf("[RateLimit] redis error: key=%s mode=%s err=%v", redisKey, failureModeLabel(failureMode), err)
			if failureMode == RateLimitFailClose {
				abortRateLimit(c)
				return
			}
			// Redis 错误时放行，避免影响正常服务
			c.Next()
			return
		}
		if repaired {
			log.Printf("[RateLimit] ttl repaired: key=%s window_ms=%d", redisKey, windowMillis)
		}

		// 超过限制
		if count > int64(limit) {
			abortRateLimit(c)
			return
		}

		c.Next()
	}
}

// LimitSuccessfulWithOptions reserves capacity before the handler runs and
// releases it unless the handler explicitly commits it. The reservation
// makes concurrent account creations obey the same hard limit without letting
// invalid requests permanently exhaust a shared NAT address's long window.
func (r *RateLimiter) LimitSuccessfulWithOptions(key string, limit int, window time.Duration, opts RateLimitOptions) gin.HandlerFunc {
	return r.limitSuccessfulByIdentity(key, limit, window, opts, func(c *gin.Context) string {
		return ippkg.AbuseIdentity(c.ClientIP())
	})
}

// LimitSuccessfulByDeviceWithOptions applies the successful-only limiter to a
// browser installation signal rather than the network address.
func (r *RateLimiter) LimitSuccessfulByDeviceWithOptions(key string, limit int, window time.Duration, opts RateLimitOptions) gin.HandlerFunc {
	return r.limitSuccessfulByIdentity(key, limit, window, opts, ClientDeviceIdentity)
}

func (r *RateLimiter) limitSuccessfulByIdentity(key string, limit int, window time.Duration, opts RateLimitOptions, identityFor func(*gin.Context) string) gin.HandlerFunc {
	failureMode := opts.FailureMode
	if failureMode != RateLimitFailClose {
		failureMode = RateLimitFailOpen
	}
	return func(c *gin.Context) {
		identity := identityFor(c)
		if identity == "" {
			log.Printf("[RateLimit] invalid client IP: mode=%s", failureModeLabel(failureMode))
			if failureMode == RateLimitFailClose {
				abortRateLimit(c)
				return
			}
			c.Next()
			return
		}
		redisKey := r.prefix + key + ":" + identity
		values, err := successfulRateLimitReserveScript.Run(
			c.Request.Context(), r.redis, []string{redisKey}, windowTTLMillis(window), limit,
		).Slice()
		if err != nil || len(values) < 2 {
			log.Printf("[RateLimit] successful-only reserve error: key=%s mode=%s err=%v", redisKey, failureModeLabel(failureMode), err)
			if failureMode == RateLimitFailClose {
				abortRateLimit(c)
				return
			}
			c.Next()
			return
		}
		reserved, parseErr := parseInt64(values[1])
		if parseErr != nil {
			log.Printf("[RateLimit] successful-only reserve parse error: key=%s mode=%s err=%v", redisKey, failureModeLabel(failureMode), parseErr)
			if failureMode == RateLimitFailOpen {
				c.Next()
				return
			}
			abortRateLimit(c)
			return
		}
		if reserved != 1 {
			abortRateLimit(c)
			return
		}

		c.Next()
		committed, _ := c.Get(successfulRateLimitCommittedKey)
		if committed != true {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := successfulRateLimitReleaseScript.Run(ctx, r.redis, []string{redisKey}).Err(); err != nil {
				log.Printf("[RateLimit] successful-only release error: key=%s err=%v", redisKey, err)
			}
		}
	}
}

func windowTTLMillis(window time.Duration) int64 {
	ttl := window.Milliseconds()
	if ttl < 1 {
		return 1
	}
	return ttl
}

func abortRateLimit(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
		"error":   "rate limit exceeded",
		"message": "Too many requests, please try again later",
	})
}

func failureModeLabel(mode RateLimitFailureMode) string {
	if mode == RateLimitFailClose {
		return "fail-close"
	}
	return "fail-open"
}

func parseInt64(value any) (int64, error) {
	switch v := value.(type) {
	case int64:
		return v, nil
	case int:
		return int64(v), nil
	case string:
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, err
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("unexpected value type %T", value)
	}
}
