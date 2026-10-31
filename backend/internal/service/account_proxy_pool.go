package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptrace"
	"sync/atomic"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// AccountProxyFallbackIDsKey is additive: proxy_id remains the primary proxy.
const AccountProxyFallbackIDsKey = "proxy_fallback_ids"
const MaxAccountFallbackProxies = 7

// AccountProxyFallbackIDs accepts both freshly constructed and JSON-decoded extras.
func AccountProxyFallbackIDs(extra map[string]any) []int64 {
	var ids []int64
	if raw, ok := extra[AccountProxyFallbackIDsKey]; ok {
		b, err := json.Marshal(raw)
		if err != nil || json.Unmarshal(b, &ids) != nil {
			return nil
		}
	}
	return ids
}

func (s *adminServiceImpl) configureAccountFallbackProxies(ctx context.Context, account *Account, ids *[]int64) error {
	if ids == nil {
		return nil
	}
	bad := func(message string) error { return infraerrors.BadRequest("INVALID_ACCOUNT_PROXY_POOL", message) }
	if len(*ids) > MaxAccountFallbackProxies {
		return bad("at most 7 fallback proxies are allowed")
	}
	if len(*ids) > 0 && (account.Platform != PlatformOpenAI || account.IsCredentialShadow()) {
		return bad("fallback proxies are supported on primary OpenAI accounts")
	}
	if len(*ids) > 0 && (account.ProxyID == nil || *account.ProxyID <= 0) {
		return bad("select a primary proxy before adding fallback proxies")
	}
	seen := map[int64]bool{}
	if account.ProxyID != nil {
		seen[*account.ProxyID] = true
	}
	for _, id := range *ids {
		if id <= 0 || seen[id] {
			return bad("proxy IDs must be positive and unique, including the primary proxy")
		}
		seen[id] = true
		if s.proxyRepo == nil {
			return bad("proxy repository is unavailable")
		}
		p, err := s.proxyRepo.GetByID(ctx, id)
		if err != nil || p == nil || !p.IsActive() || p.IsExpired(time.Now()) {
			return bad("a fallback proxy is missing, disabled or expired")
		}
	}
	if account.Extra == nil {
		account.Extra = map[string]any{}
	}
	// Empty arrays explicitly clear the pool; omission preserves legacy behavior.
	delete(account.Extra, AccountProxyFallbackIDsKey)
	if len(*ids) > 0 {
		account.Extra[AccountProxyFallbackIDsKey] = append([]int64(nil), (*ids)...)
	}
	return nil
}

// accountProxyRoutes never falls back to a direct connection for a configured pool.
// Missing/deleted/inactive proxies are omitted; no usable route is a hard error.
func accountProxyRoutes(account *Account, legacyURL string) ([]string, error) {
	if account == nil || account.Platform != PlatformOpenAI || len(AccountProxyFallbackIDs(account.Extra)) == 0 {
		return []string{legacyURL}, nil
	}
	urls := []string{}
	seen := map[int64]bool{}
	add := func(p *Proxy) {
		if p == nil || seen[p.ID] || !p.IsActive() || p.IsExpired(time.Now()) {
			return
		}
		seen[p.ID] = true
		urls = append(urls, p.URL())
	}
	add(account.Proxy)
	for _, p := range account.FallbackProxies {
		add(p)
	}
	if len(urls) == 0 {
		return nil, errors.New("no available proxy in account proxy pool")
	}
	return urls, nil
}

// Only failures before the upstream request can have been written are retryable.
// An upstream HTTP error (including 429) and an interrupted stream are not retried.
func proxyConnectFailure(err error, gotConnection bool) bool {
	if err == nil || gotConnection || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var op *net.OpError
	return errors.As(err, &op) && (op.Op == "dial" || op.Op == "proxyconnect" || op.Op == "socks connect")
}

func doWithAccountProxyFallback(request *http.Request, legacyURL string, account *Account, send func(*http.Request, string) (*http.Response, error)) (*http.Response, error) {
	routes, err := accountProxyRoutes(account, legacyURL)
	if err != nil {
		return nil, err
	}
	if len(routes) == 1 {
		return send(request, routes[0])
	}
	var response *http.Response
	for i, route := range routes {
		if request.Context().Err() != nil {
			return nil, request.Context().Err()
		}
		attempt := request.Clone(request.Context())
		if i > 0 && request.Body != nil && request.Body != http.NoBody {
			if request.GetBody == nil {
				return response, err
			}
			attempt.Body, err = request.GetBody()
			if err != nil {
				return nil, err
			}
		}
		var connected atomic.Bool
		trace := &httptrace.ClientTrace{GotConn: func(httptrace.GotConnInfo) { connected.Store(true) }}
		attempt = attempt.WithContext(httptrace.WithClientTrace(attempt.Context(), trace))
		response, err = send(attempt, route)
		if response != nil {
			logAccountFallbackRoute(account, route, "http")
		}
		if response != nil || !proxyConnectFailure(err, connected.Load()) {
			return response, err
		}
	}
	return response, err
}

func dialWithAccountProxyFallback(ctx context.Context, dialer openAIWSClientDialer, target string, headers http.Header, legacyURL string, account *Account) (openAIWSClientConn, int, http.Header, error) {
	routes, err := accountProxyRoutes(account, legacyURL)
	if err != nil {
		return nil, 0, nil, err
	}
	var conn openAIWSClientConn
	var status int
	var responseHeaders http.Header
	for _, route := range routes {
		if ctx.Err() != nil {
			return nil, 0, nil, ctx.Err()
		}
		conn, status, responseHeaders, err = dialer.Dial(ctx, target, headers, route)
		if err == nil {
			logAccountFallbackRoute(account, route, "websocket")
		}
		if err == nil || status != 0 || !proxyConnectFailure(err, false) {
			return conn, status, responseHeaders, err
		}
		if conn != nil {
			_ = conn.Close()
		}
	}
	return nil, status, responseHeaders, err
}

func logAccountFallbackRoute(account *Account, route, transport string) {
	if account == nil || (account.Proxy != nil && account.Proxy.URL() == route) {
		return
	}
	for _, p := range account.FallbackProxies {
		if p != nil && p.URL() == route {
			slog.Info("account_proxy_fallback", "account_id", account.ID, "proxy_id", p.ID, "transport", transport)
			return
		}
	}
}

func refreshWithAccountProxyFallback(ctx context.Context, account *Account, legacyURL string, refresh func(context.Context, string) (*OpenAITokenInfo, error)) (*OpenAITokenInfo, error) {
	routes, err := accountProxyRoutes(account, legacyURL)
	if err != nil {
		return nil, err
	}
	var info *OpenAITokenInfo
	for _, route := range routes {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var connected atomic.Bool
		attempt := httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{GotConn: func(httptrace.GotConnInfo) { connected.Store(true) }})
		info, err = refresh(attempt, route)
		if err == nil {
			logAccountFallbackRoute(account, route, "oauth_refresh")
			return info, nil
		}
		if !proxyConnectFailure(err, connected.Load()) {
			return nil, err
		}
	}
	return info, err
}
