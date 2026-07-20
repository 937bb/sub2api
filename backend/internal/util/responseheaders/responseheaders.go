package responseheaders

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// SetSSEStreamingHeaders sets the standard SSE response headers on a Gin context.
// It always sets Content-Type and Cache-Control. For Connection and X-Accel-Buffering,
// it applies protocol-aware logic to avoid sending inappropriate headers:
//
//   - Connection: keep-alive is only set for HTTP/1.x requests. HTTP/2 (RFC 9113 §8.2.2)
//     forbids connection-specific headers; real api.anthropic.com never sends it over h2.
//   - X-Accel-Buffering: no is only set when the request arrived through a reverse proxy
//     (detected via X-Forwarded-For or X-Real-Ip headers). This is an nginx-specific
//     directive that the real Anthropic API never sends. Operators not using a reverse
//     proxy won't leak it; nginx users behind a proxy get it automatically, or can
//     configure "proxy_buffering off" server-side.
func SetSSEStreamingHeaders(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")

	// Connection: keep-alive is an HTTP/1.1 hop-by-hop header.
	// It MUST NOT appear in HTTP/2 responses (RFC 9113 §8.2.2).
	if c.Request.ProtoMajor < 2 {
		c.Header("Connection", "keep-alive")
	}

	// X-Accel-Buffering is an nginx-specific directive. Only set it when
	// the request clearly came through a reverse proxy, so deployments
	// without nginx don't leak this non-standard header.
	if IsBehindReverseProxy(c.Request) {
		c.Header("X-Accel-Buffering", "no")
	}
}

// SetSSEStreamingHeadersRaw is like SetSSEStreamingHeaders but operates on a
// raw http.Header and *http.Request. Use this when a gin.Context is not directly
// available (e.g. inside keepalive/heartbeat goroutines that only hold a
// gin.ResponseWriter).
func SetSSEStreamingHeadersRaw(header http.Header, req *http.Request) {
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	if req.ProtoMajor < 2 {
		header.Set("Connection", "keep-alive")
	}
	if IsBehindReverseProxy(req) {
		header.Set("X-Accel-Buffering", "no")
	}
}

// IsBehindReverseProxy returns true when the request appears to have arrived
// through a reverse proxy, detected by the presence of X-Forwarded-For or
// X-Real-Ip headers.
func IsBehindReverseProxy(req *http.Request) bool {
	return req.Header.Get("X-Forwarded-For") != "" || req.Header.Get("X-Real-Ip") != ""
}

// defaultAllowed 定义允许透传的响应头白名单
// 注意：以下头部由 Go HTTP 包自动处理，不应手动设置：
//   - content-length: 由 ResponseWriter 根据实际写入数据自动设置
//   - transfer-encoding: 由 HTTP 库根据需要自动添加/移除
//   - connection: 由 HTTP 库管理连接复用
var defaultAllowed = map[string]struct{}{
	"content-type":                   {},
	"content-encoding":               {},
	"content-language":               {},
	"cache-control":                  {},
	"etag":                           {},
	"last-modified":                  {},
	"expires":                        {},
	"vary":                           {},
	"date":                           {},
	"x-request-id":                   {},
	"x-ratelimit-limit-requests":     {},
	"x-ratelimit-limit-tokens":       {},
	"x-ratelimit-remaining-requests": {},
	"x-ratelimit-remaining-tokens":   {},
	"x-ratelimit-reset-requests":     {},
	"x-ratelimit-reset-tokens":       {},
	"retry-after":                    {},
	"location":                       {},
	"www-authenticate":               {},
}

// hopByHopHeaders 是跳过的 hop-by-hop 头部，这些头部由 HTTP 库自动处理
var hopByHopHeaders = map[string]struct{}{
	"content-length":    {},
	"transfer-encoding": {},
	"connection":        {},
}

type CompiledHeaderFilter struct {
	allowed     map[string]struct{}
	forceRemove map[string]struct{}
}

var defaultCompiledHeaderFilter = CompileHeaderFilter(config.ResponseHeaderConfig{})

func CompileHeaderFilter(cfg config.ResponseHeaderConfig) *CompiledHeaderFilter {
	allowed := make(map[string]struct{}, len(defaultAllowed)+len(cfg.AdditionalAllowed))
	for key := range defaultAllowed {
		allowed[key] = struct{}{}
	}
	// 关闭时只使用默认白名单，additional/force_remove 不生效
	if cfg.Enabled {
		for _, key := range cfg.AdditionalAllowed {
			normalized := strings.ToLower(strings.TrimSpace(key))
			if normalized == "" {
				continue
			}
			allowed[normalized] = struct{}{}
		}
	}

	forceRemove := map[string]struct{}{}
	if cfg.Enabled {
		forceRemove = make(map[string]struct{}, len(cfg.ForceRemove))
		for _, key := range cfg.ForceRemove {
			normalized := strings.ToLower(strings.TrimSpace(key))
			if normalized == "" {
				continue
			}
			forceRemove[normalized] = struct{}{}
		}
	}

	return &CompiledHeaderFilter{
		allowed:     allowed,
		forceRemove: forceRemove,
	}
}

func FilterHeaders(src http.Header, filter *CompiledHeaderFilter) http.Header {
	if filter == nil {
		filter = defaultCompiledHeaderFilter
	}

	filtered := make(http.Header, len(src))
	for key, values := range src {
		lower := strings.ToLower(key)
		if _, blocked := filter.forceRemove[lower]; blocked {
			continue
		}
		if _, ok := filter.allowed[lower]; !ok {
			continue
		}
		// 跳过 hop-by-hop 头部，这些由 HTTP 库自动处理
		if _, isHopByHop := hopByHopHeaders[lower]; isHopByHop {
			continue
		}
		for _, value := range values {
			filtered.Add(key, value)
		}
	}
	return filtered
}

func WriteFilteredHeaders(dst http.Header, src http.Header, filter *CompiledHeaderFilter) {
	filtered := FilterHeaders(src, filter)
	for key, values := range filtered {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}
