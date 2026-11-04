package repository

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyurl"
	"github.com/Wei-Shaw/sub2api/internal/pkg/servertiming"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"

	"github.com/imroc/req/v3"
)

// reqClientOptions 定义 req 客户端的构建参数
type reqClientOptions struct {
	ProxyURL    string        // 代理 URL（支持 http/https/socks5）
	Timeout     time.Duration // 请求超时时间
	Impersonate bool          // 是否模拟浏览器指纹（当前为 Firefox，Chrome 伪装会被 chatgpt.com 的 Cloudflare 质询）
	ForceHTTP2  bool          // 是否强制使用 HTTP/2
}

// sharedReqClients 存储按配置参数缓存的 req 客户端实例
//
// 性能优化说明：
// 原实现在每次 OAuth 刷新时都创建新的 req.Client：
// 1. claude_oauth_service.go: 每次刷新创建新客户端
// 2. openai_oauth_service.go: 每次刷新创建新客户端
// 3. gemini_oauth_client.go: 每次刷新创建新客户端
//
// 新实现使用 sync.Map 缓存客户端：
// 1. 相同配置（代理+超时+模拟设置）复用同一客户端
// 2. 复用底层连接池，减少 TLS 握手开销
// 3. LoadOrStore 保证并发安全，避免重复创建
var sharedReqClients sync.Map

// getSharedReqClient 获取共享的 req 客户端实例
// 性能优化：相同配置复用同一客户端，避免重复创建
func getSharedReqClient(opts reqClientOptions) (*req.Client, error) {
	key := buildReqClientKey(opts)
	if cached, ok := sharedReqClients.Load(key); ok {
		if c, ok := cached.(*req.Client); ok {
			return c, nil
		}
	}

	client := req.C().SetTimeout(opts.Timeout)
	if opts.ForceHTTP2 {
		client = client.EnableForceHTTP2()
	}
	if opts.Impersonate {
		// chatgpt.com 的 Cloudflare 会对 req 内置的 Chrome 伪装（UA 固定为 Chrome/120，
		// 与 sec-ch-ua 等 Client Hints 一起已明显过时）直接返回 403 cf-mitigated=challenge，
		// 导致 accounts/check、subscriptions、隐私设置等 backend-api 调用全部失败，
		// 订阅到期时间因此长期不更新（见 issue #4825）。Firefox 伪装的 UA/头部组合
		// 在同一出口 IP 下稳定通过，故改用 Firefox 指纹。
		client = client.ImpersonateFirefox()
	}
	trimmed, parsedProxy, err := proxyurl.Parse(opts.ProxyURL)
	if err != nil {
		return nil, err
	}
	if trimmed != "" {
		client.SetProxyURL(trimmed)
	}
	// Auth and quota requests carrying the measured CLI identity use the same
	// HTTPS profile as inference. Browser-oriented privacy calls keep their transport.
	localTransport := &http.Transport{DisableCompression: true, MaxIdleConns: 32, MaxIdleConnsPerHost: 8}
	if err := tlsfingerprint.ConfigureTLS(localTransport, tlsfingerprint.CodexHTTPProfile(), parsedProxy); err != nil {
		return nil, err
	}
	orderedLocalTransport := tlsfingerprint.NewCodexHTTPTransport(localTransport, tlsfingerprint.CodexHTTPProfile())
	client.GetTransport().WrapRoundTripFunc(func(rt http.RoundTripper) req.HttpRoundTripFunc {
		return func(r *http.Request) (*http.Response, error) {
			if r.URL.Scheme == "https" && openai.IsLocalCLIUserAgent(r.UserAgent()) {
				return orderedLocalTransport.RoundTrip(localCLIReqWithoutBrowserDefaults(r))
			}
			return rt.RoundTrip(r)
		}
	})
	client = instrumentReqClient(client)

	actual, _ := sharedReqClients.LoadOrStore(key, client)
	if c, ok := actual.(*req.Client); ok {
		return c, nil
	}
	return client, nil
}

func instrumentReqClient(client *req.Client) *req.Client {
	if client == nil {
		return nil
	}
	client.GetTransport().WrapRoundTripFunc(func(rt http.RoundTripper) req.HttpRoundTripFunc {
		timed := servertiming.WrapRoundTripper(rt)
		return timed.RoundTrip
	})
	return client
}

func buildReqClientKey(opts reqClientOptions) string {
	return fmt.Sprintf("%s|%s|%t|%t",
		strings.TrimSpace(opts.ProxyURL),
		opts.Timeout.String(),
		opts.Impersonate,
		opts.ForceHTTP2,
	)
}

// CreatePrivacyReqClient creates an HTTP client for OpenAI privacy settings API
// This is exported for use by OpenAIPrivacyService
// Uses Chrome TLS fingerprint impersonation to bypass Cloudflare checks
func CreatePrivacyReqClient(proxyURL string) (*req.Client, error) {
	return getSharedReqClient(reqClientOptions{
		ProxyURL:    proxyURL,
		Timeout:     30 * time.Second,
		Impersonate: true, // Enable browser TLS fingerprint impersonation (Firefox, see getSharedReqClient)
	})
}

// req browser impersonation installs common navigation headers. Never carry
// those defaults into a request explicitly using the local CLI identity.
func localCLIReqWithoutBrowserDefaults(r *http.Request) *http.Request {
	cloned := r.Clone(r.Context())
	for key := range cloned.Header {
		lower := strings.ToLower(key)
		if strings.HasPrefix(lower, "sec-fetch-") || strings.HasPrefix(lower, "sec-ch-ua") || lower == "upgrade-insecure-requests" || key == req.HeaderOderKey || key == req.PseudoHeaderOderKey {
			delete(cloned.Header, key)
		}
	}
	return cloned
}
