//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/stretchr/testify/require"
)

func poolTestAccount() *Account {
	id := int64(1)
	return &Account{ID: 50, Platform: PlatformOpenAI, Type: AccountTypeOAuth, ProxyID: &id,
		Extra:           map[string]any{AccountProxyFallbackIDsKey: []int64{2}},
		Proxy:           &Proxy{ID: 1, Protocol: "http", Host: "primary.invalid", Port: 8080, Status: StatusActive},
		FallbackProxies: []*Proxy{{ID: 2, Protocol: "http", Host: "backup.invalid", Port: 8080, Status: StatusActive}}}
}

func TestAccountProxyPoolHTTPRealConnectionFailure(t *testing.T) {
	accountProxyRotationCounters.Delete(int64(50))
	// Closed local listener forces an actual dial refusal without an upstream request.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	closed := l.Addr().(*net.TCPAddr).Port
	require.NoError(t, l.Close())
	calls := 0
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		body, e := io.ReadAll(r.Body)
		require.NoError(t, e)
		require.Equal(t, `{"input":"hello"}`, string(body))
		_, _ = w.Write([]byte("ok"))
	}))
	defer backup.Close()
	a := poolTestAccount()
	a.Proxy.Host = "127.0.0.1"
	a.Proxy.Port = closed
	u, _ := url.Parse(backup.URL)
	port, _ := strconv.Atoi(u.Port())
	a.FallbackProxies[0].Host = u.Hostname()
	a.FallbackProxies[0].Port = port
	req, _ := http.NewRequest(http.MethodPost, "http://upstream.invalid/v1/responses", strings.NewReader(`{"input":"hello"}`))
	resp, err := doWithAccountProxyFallback(req, a.Proxy.URL(), a, func(r *http.Request, route string) (*http.Response, error) {
		p, e := url.Parse(route)
		require.NoError(t, e)
		tr := &http.Transport{Proxy: http.ProxyURL(p)}
		defer tr.CloseIdleConnections()
		return (&http.Client{Transport: tr, Timeout: time.Second}).Do(r)
	})
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)
	require.Equal(t, 1, calls)
}

func TestAccountProxyPoolHTTPRotation(t *testing.T) {
	a := poolTestAccount()
	a.ID = 9501
	accountProxyRotationCounters.Delete(a.ID)
	var got []string
	for i := 0; i < 6; i++ {
		// Simulate different snapshots of the same account on successive requests.
		copy := *a
		req, _ := http.NewRequest("POST", "https://example.com", strings.NewReader("test"))
		resp, err := doWithAccountProxyFallback(req, a.Proxy.URL(), &copy, func(_ *http.Request, route string) (*http.Response, error) {
			got = append(got, route)
			return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
		})
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
	}
	p, b := a.Proxy.URL(), a.FallbackProxies[0].URL()
	require.Equal(t, []string{p, b, p, b, p, b}, got)
	other := *a
	other.ID = 9502
	accountProxyRotationCounters.Delete(other.ID)
	routes, err := rotatingAccountProxyRoutes(&other, "")
	require.NoError(t, err)
	require.Equal(t, p, routes[0], "accounts have independent rotation")
	a.FallbackProxies[0].Status = "inactive"
	for i := 0; i < 3; i++ {
		routes, err = rotatingAccountProxyRoutes(a, "")
		require.NoError(t, err)
		require.Equal(t, []string{p}, routes)
	}
}

func TestAccountProxyPoolConcurrentRotation(t *testing.T) {
	a := poolTestAccount()
	a.ID = 9503
	accountProxyRotationCounters.Delete(a.ID)
	results := make(chan string, 100)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			routes, err := rotatingAccountProxyRoutes(a, "")
			if err == nil {
				results <- routes[0]
			}
		}()
	}
	wg.Wait()
	close(results)
	counts := map[string]int{}
	for r := range results {
		counts[r]++
	}
	require.Equal(t, map[string]int{a.Proxy.URL(): 50, a.FallbackProxies[0].URL(): 50}, counts)
}

func TestAccountProxyPoolWebSocketRotationAndPinnedContinuation(t *testing.T) {
	for _, capacity := range []int{1, 2} {
		t.Run(strconv.Itoa(capacity), func(t *testing.T) {
			a := poolTestAccount()
			a.ID = int64(9600 + capacity)
			accountProxyRotationCounters.Delete(a.ID)
			cfg := &config.Config{}
			cfg.Gateway.OpenAIWS.MaxConnsPerAccount = capacity
			cfg.Gateway.OpenAIWS.MaxIdlePerAccount = capacity
			pool := newOpenAIWSConnPool(cfg)
			defer pool.Close()
			dialer := &rotationTestDialer{}
			pool.setClientDialerForTest(dialer)
			req := openAIWSAcquireRequest{Account: a, WSURL: "wss://example.com/v1/responses"}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			var ids []string
			for i := 0; i < 4; i++ {
				lease, err := pool.Acquire(ctx, req)
				require.NoError(t, err)
				want := a.Proxy.URL()
				if i%2 == 1 {
					want = a.FallbackProxies[0].URL()
				}
				require.Equal(t, want, lease.conn.handshakeCompatibility.selectedProxy)
				ids = append(ids, lease.ConnID())
				lease.Release()
			}
			if capacity == 2 {
				require.Equal(t, ids[0], ids[2])
				require.Equal(t, ids[1], ids[3])
				require.Equal(t, []string{a.Proxy.URL(), a.FallbackProxies[0].URL()}, dialer.routes)
			} else {
				require.Equal(t, []string{a.Proxy.URL(), a.FallbackProxies[0].URL(), a.Proxy.URL(), a.FallbackProxies[0].URL()}, dialer.routes)
			}
			req.ForcePreferredConn = true
			req.PreferredConnID = ids[3]
			lease, err := pool.Acquire(ctx, req)
			require.NoError(t, err)
			require.Equal(t, ids[3], lease.ConnID())
			lease.Release()
			// Pinned continuation must not consume another rotation slot.
			req.ForcePreferredConn = false
			req.PreferredConnID = ""
			lease, err = pool.Acquire(ctx, req)
			require.NoError(t, err)
			require.Equal(t, a.Proxy.URL(), lease.conn.handshakeCompatibility.selectedProxy)
			lease.Release()
		})
	}
}

type rotationTestDialer struct {
	routes    []string
	failFirst bool
}

func (d *rotationTestDialer) Dial(_ context.Context, _ string, _ http.Header, route string) (openAIWSClientConn, int, http.Header, error) {
	d.routes = append(d.routes, route)
	if d.failFirst && len(d.routes) == 1 {
		return nil, 0, nil, &net.OpError{Op: "dial", Err: errors.New("refused")}
	}
	return &openAIWSFakeConn{}, 0, nil, nil
}

func TestAccountProxyPoolWebSocketReusesActualFailoverRoute(t *testing.T) {
	a := poolTestAccount()
	a.ID = 9701
	accountProxyRotationCounters.Delete(a.ID)
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 2
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 2
	pool := newOpenAIWSConnPool(cfg)
	defer pool.Close()
	dialer := &rotationTestDialer{failFirst: true}
	pool.setClientDialerForTest(dialer)
	req := openAIWSAcquireRequest{Account: a, WSURL: "wss://example.com/v1/responses"}
	lease, err := pool.Acquire(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, a.FallbackProxies[0].URL(), lease.conn.handshakeCompatibility.selectedProxy)
	id := lease.ConnID()
	lease.Release()
	lease, err = pool.Acquire(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, id, lease.ConnID())
	require.Equal(t, []string{a.Proxy.URL(), a.FallbackProxies[0].URL()}, dialer.routes)
	lease.Release()
}

func TestAccountProxyPoolHTTPNeverReplaysUpstreamResponses(t *testing.T) {
	for _, code := range []int{200, 401, 403, 429, 502, 503} {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			req, _ := http.NewRequest("POST", "https://example.com", strings.NewReader("data"))
			calls := 0
			resp, err := doWithAccountProxyFallback(req, "", poolTestAccount(), func(*http.Request, string) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader("body"))}, nil
			})
			require.NoError(t, err)
			require.Equal(t, code, resp.StatusCode)
			require.Equal(t, 1, calls)
			_ = resp.Body.Close()
		})
	}
	for _, connected := range []bool{false, true} {
		req, _ := http.NewRequest("POST", "https://example.com", strings.NewReader("data"))
		calls := 0
		_, err := doWithAccountProxyFallback(req, "", poolTestAccount(), func(r *http.Request, _ string) (*http.Response, error) {
			calls++
			if connected {
				httptrace.ContextClientTrace(r.Context()).GotConn(httptrace.GotConnInfo{})
			}
			return nil, &net.OpError{Op: "read", Err: io.ErrUnexpectedEOF}
		})
		require.Error(t, err)
		require.Equal(t, 1, calls)
	}
}

func TestAccountProxyPoolCompatibilityAndFailClosed(t *testing.T) {
	a := poolTestAccount()
	var cached Account
	b, err := json.Marshal(a)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(b, &cached))
	routes, err := accountProxyRoutes(&cached, "")
	require.NoError(t, err)
	require.Equal(t, []string{a.Proxy.URL(), a.FallbackProxies[0].URL()}, routes)
	a.Proxy.Status = "inactive"
	a.FallbackProxies = nil
	_, err = accountProxyRoutes(a, "")
	require.Error(t, err)
	a.Extra = nil
	_, err = accountProxyRoutes(a, "legacy")
	require.ErrorContains(t, err, "unavailable")
	a.ProxyID = nil
	routes, err = accountProxyRoutes(a, "legacy")
	require.NoError(t, err)
	require.Equal(t, []string{"legacy"}, routes)
	routes, err = accountProxyRoutes(&Account{}, "")
	require.NoError(t, err)
	require.Equal(t, []string{""}, routes)
}

type poolTestDialer struct {
	routes []string
	status int
}

func (d *poolTestDialer) Dial(_ context.Context, _ string, _ http.Header, route string) (openAIWSClientConn, int, http.Header, error) {
	d.routes = append(d.routes, route)
	if len(d.routes) == 1 {
		return nil, d.status, nil, &net.OpError{Op: "dial", Err: errors.New("refused")}
	}
	return nil, 0, http.Header{"X-Connected": {"yes"}}, nil
}

func TestAccountProxyPoolWebSocketFallbackBeforeHandshakeOnly(t *testing.T) {
	a := poolTestAccount()
	d := &poolTestDialer{}
	_, _, h, err := dialWithAccountProxyFallback(context.Background(), d, "wss://example.com", nil, "", a)
	require.NoError(t, err)
	require.Equal(t, "yes", h.Get("X-Connected"))
	require.Len(t, d.routes, 2)
	d = &poolTestDialer{status: 429}
	_, status, _, err := dialWithAccountProxyFallback(context.Background(), d, "wss://example.com", nil, "", a)
	require.Error(t, err)
	require.Equal(t, 429, status)
	require.Len(t, d.routes, 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	d = &poolTestDialer{}
	_, _, _, err = dialWithAccountProxyFallback(ctx, d, "wss://example.com", nil, "", a)
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, d.routes)
}

type poolTestProxyRepo struct {
	ProxyRepository
	proxies map[int64]*Proxy
}

func (r *poolTestProxyRepo) ListByIDs(_ context.Context, ids []int64) ([]Proxy, error) {
	out := []Proxy{}
	for _, id := range ids {
		if p := r.proxies[id]; p != nil {
			out = append(out, *p)
		}
	}
	return out, nil
}

type poolTestOAuthClient struct {
	OpenAIOAuthClient
	routes  []string
	failure error
}

func (c *poolTestOAuthClient) RefreshTokenWithClientID(_ context.Context, _, route, _ string) (*openai.TokenResponse, error) {
	c.routes = append(c.routes, route)
	if len(c.routes) == 1 {
		return nil, c.failure
	}
	return &openai.TokenResponse{AccessToken: "refreshed", ExpiresIn: 3600}, nil
}

func TestAccountProxyPoolOAuthRefresh(t *testing.T) {
	a := poolTestAccount()
	a.Credentials = map[string]any{"refresh_token": "test"}
	repo := &poolTestProxyRepo{proxies: map[int64]*Proxy{1: a.Proxy, 2: a.FallbackProxies[0]}}
	client := &poolTestOAuthClient{failure: &net.OpError{Op: "dial", Err: errors.New("refused")}}
	svc := NewOpenAIOAuthService(repo, client)
	info, err := svc.RefreshAccountToken(context.Background(), a)
	require.NoError(t, err)
	require.Equal(t, "refreshed", info.AccessToken)
	require.Len(t, client.routes, 2)
	client.routes = nil
	client.failure = errors.New("invalid_grant")
	_, err = svc.RefreshAccountToken(context.Background(), a)
	require.Error(t, err)
	require.Len(t, client.routes, 1)
}

func TestAccountProxyPoolHydratesLegacySchedulerCache(t *testing.T) {
	fresh := poolTestAccount()
	legacy := *fresh
	legacy.FallbackProxies = nil
	cache := &snapshotHydrationCache{accounts: map[int64]*Account{fresh.ID: &legacy}}
	repo := &updateAccountCredsRepoStub{account: fresh}
	snapshot := NewSchedulerSnapshotService(cache, nil, repo, nil, nil)
	account, err := snapshot.GetAccount(context.Background(), fresh.ID)
	require.NoError(t, err)
	require.Len(t, account.FallbackProxies, 1)
	require.Nil(t, legacy.FallbackProxies, "do not mutate the shared legacy snapshot")
	// An explicitly empty list means the new repository has already hydrated
	// a pool whose members were deleted. It must not force repeated DB reads.
	legacy.FallbackProxies = []*Proxy{}
	snapshot = NewSchedulerSnapshotService(cache, nil, nil, nil, nil)
	account, err = snapshot.GetAccount(context.Background(), fresh.ID)
	require.NoError(t, err)
	require.Same(t, &legacy, account)
}

func (r *poolTestProxyRepo) GetByID(_ context.Context, id int64) (*Proxy, error) {
	return r.proxies[id], nil
}

func TestAccountProxyPoolValidationAndLegacyEdit(t *testing.T) {
	a := poolTestAccount()
	svc := &adminServiceImpl{proxyRepo: &poolTestProxyRepo{proxies: map[int64]*Proxy{2: a.FallbackProxies[0]}}}
	for _, ids := range [][]int64{{1}, {2, 2}, {-1}, {3}, {1, 2, 3, 4, 5, 6, 7, 8}} {
		require.Error(t, svc.configureAccountFallbackProxies(context.Background(), a, &ids))
	}
	ids := []int64{2}
	require.NoError(t, svc.configureAccountFallbackProxies(context.Background(), a, &ids))
	repo := &updateAccountCredsRepoStub{account: a}
	svc.accountRepo = repo
	updated, err := svc.UpdateAccount(context.Background(), a.ID, &UpdateAccountInput{Name: "renamed", Extra: map[string]any{}})
	require.NoError(t, err)
	require.Equal(t, []int64{2}, AccountProxyFallbackIDs(updated.Extra))
	zero := int64(0)
	updated, err = svc.UpdateAccount(context.Background(), a.ID, &UpdateAccountInput{ProxyID: &zero})
	require.NoError(t, err)
	require.Nil(t, updated.ProxyID)
	require.Empty(t, AccountProxyFallbackIDs(updated.Extra))
}

func TestAccountFixedProxyRebindingRejectsPinnedOldSocket(t *testing.T) {
	a := poolTestAccount()
	a.Extra = nil
	a.FallbackProxies = nil
	a.Proxy.Username = "fixed-account"
	a.Proxy.Password = "old-ipv6-lease"
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 2
	pool := newOpenAIWSConnPool(cfg)
	defer pool.Close()
	dialer := &rotationTestDialer{}
	pool.setClientDialerForTest(dialer)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req := openAIWSAcquireRequest{Account: a, WSURL: "wss://example.com/v1/responses"}
	lease, err := pool.Acquire(ctx, req)
	require.NoError(t, err)
	oldID := lease.ConnID()
	lease.Release()
	next := *a
	proxy := *a.Proxy
	proxy.Password = "new-ipv6-lease"
	require.NotEqual(t, a.Proxy.URL(), proxy.URL())
	next.Proxy = &proxy
	req.Account = &next
	req.ForcePreferredConn = true
	req.PreferredConnID = oldID
	_, err = pool.Acquire(ctx, req)
	require.ErrorIs(t, err, errOpenAIWSPreferredConnUnavailable)
	req.ForcePreferredConn = false
	req.PreferredConnID = ""
	lease, err = pool.Acquire(ctx, req)
	require.NoError(t, err)
	require.NotEqual(t, oldID, lease.ConnID())
	require.Equal(t, proxy.URL(), lease.conn.handshakeCompatibility.selectedProxy)
	lease.Release()
	next.Proxy = nil
	_, err = pool.Acquire(ctx, req)
	require.ErrorContains(t, err, "unavailable")
	require.Equal(t, []string{a.Proxy.URL(), proxy.URL()}, dialer.routes)
}
