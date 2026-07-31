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

// AllowResult 单次固定窗口限流判定结果。
type AllowResult struct {
	// Allowed 是否放行
	Allowed bool
	// Count 当前窗口内累计请求数（含本次）
	Count int64
	// RetryAfter 超限时距窗口重置的剩余时间（尽力而为；PTTL 不可用时回退为完整窗口）
	RetryAfter time.Duration
}

// Allow 对给定 key（不含 "rate_limit:" 前缀）执行一次固定窗口计数判定。
// 供需要自定义限流维度（如按用户 ID）的调用方使用；Redis 错误由调用方决定 fail-open/close。
func (r *RateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (AllowResult, error) {
	redisKey := r.prefix + key
	windowMillis := windowTTLMillis(window)

	count, repaired, err := rateLimitRun(ctx, r.redis, redisKey, windowMillis)
	if err != nil {
		return AllowResult{}, err
	}
	if repaired {
		log.Printf("[RateLimit] ttl repaired: key=%s window_ms=%d", redisKey, windowMillis)
	}

	result := AllowResult{Allowed: count <= int64(limit), Count: count}
	if !result.Allowed {
		result.RetryAfter = window
		if ttl, ttlErr := r.redis.PTTL(ctx, redisKey).Result(); ttlErr == nil && ttl > 0 {
			result.RetryAfter = ttl
		}
	}
	return result, nil
}

// clientIPForRateLimit 返回 IP 维度限流使用的客户端地址。
// 与审计日志/会话绑定/API Key IP ACL 共用同一套安全客户端 IP 解析
// （SessionBindingContext 快照：兼容开关开启时信任反代转发头，关闭时走
// server.trusted_proxies 可信链）。避免默认反代部署下 Gin ClientIP 恒等于
// 代理地址、所有用户坍缩进同一个限流桶造成整体误拦截。
func clientIPForRateLimit(c *gin.Context) string {
	if resolved := ippkg.GetSecurityClientIP(c, false); resolved != "" {
		return resolved
	}
	return c.ClientIP()
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
		result, err := r.Allow(c.Request.Context(), key+":"+clientIPForRateLimit(c), limit, window)
		if err != nil {
			log.Printf("[RateLimit] redis error: key=%s mode=%s err=%v", r.prefix+key, failureModeLabel(failureMode), err)
			if failureMode == RateLimitFailClose {
				abortRateLimit(c, window)
				return
			}
			// Redis 错误时放行，避免影响正常服务
			c.Next()
			return
		}

		// 超过限制
		if !result.Allowed {
			abortRateLimit(c, result.RetryAfter)
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
				abortRateLimit(c, window)
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
				abortRateLimit(c, window)
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
			abortRateLimit(c, window)
			return
		}
		if reserved != 1 {
			abortRateLimit(c, window)
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

func abortRateLimit(c *gin.Context, retryAfter time.Duration) {
	if retryAfter > 0 {
		seconds := int64(retryAfter / time.Second)
		if retryAfter%time.Second > 0 {
			seconds++
		}
		c.Header("Retry-After", strconv.FormatInt(seconds, 10))
	}
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
