package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

func fixedKiroAccount() *Account {
	id := int64(17)
	return &Account{ID: 19, Platform: PlatformKiro, Type: AccountTypeOAuth, ProxyID: &id,
		Proxy:       &Proxy{ID: id, Host: "192.0.2.17", Port: 8000, Protocol: "http", Username: "test", Password: "secret", Status: StatusActive, FallbackMode: FallbackModeNone},
		Credentials: map[string]any{"refresh_token": "test-refresh", "machine_id": strings.Repeat("a", 64)},
	}
}

func TestKiroProxyFailsClosed(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Account)
	}{
		{"missing binding", func(a *Account) { a.ProxyID = nil }},
		{"missing relation", func(a *Account) { a.Proxy = nil }},
		{"stale relation", func(a *Account) { a.Proxy.ID = 18 }},
		{"disabled", func(a *Account) { a.Proxy.Status = "inactive" }},
		{"expired", func(a *Account) { past := time.Now().Add(-time.Second); a.Proxy.ExpiresAt = &past }},
		{"direct fallback", func(a *Account) { a.Proxy.FallbackMode = FallbackModeDirect }},
		{"backup fallback", func(a *Account) { id := int64(20); a.Proxy.BackupProxyID = &id }},
		{"rotation", func(a *Account) { a.Extra = map[string]any{AccountProxyFallbackIDsKey: []int64{20}} }},
		{"previously switched", func(a *Account) { id := int64(20); a.ProxyFallbackOriginID = &id }},
		{"empty host", func(a *Account) { a.Proxy.Host = "" }},
		{"invalid port", func(a *Account) { a.Proxy.Port = 0 }},
		{"invalid scheme", func(a *Account) { a.Proxy.Protocol = "file" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := fixedKiroAccount()
			tt.edit(a)
			url, err := kiroProxy(a)
			require.Error(t, err)
			require.Empty(t, url)
		})
	}
	_, err := kiroProxy(nil)
	require.Error(t, err)
	url, err := kiroProxy(fixedKiroAccount())
	require.NoError(t, err)
	require.Equal(t, "http://test:secret@192.0.2.17:8000", url)
}

type kiroProxyUpstream struct {
	calls   int
	request *http.Request
	proxy   string
	id      int64
}

func (u *kiroProxyUpstream) Do(req *http.Request, p string, id int64, _ int) (*http.Response, error) {
	u.calls++
	u.request = req
	u.proxy = p
	u.id = id
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"accessToken":"new-access","refreshToken":"new-refresh","expiresIn":3600}`))}, nil
}
func (u *kiroProxyUpstream) DoWithTLS(r *http.Request, p string, id int64, c int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(r, p, id, c)
}
func TestKiroRefreshAlwaysUsesBoundProxy(t *testing.T) {
	upstream := &kiroProxyUpstream{}
	executor := &kiroRefreshExecutor{upstream: upstream}
	a := fixedKiroAccount()
	credentials, err := executor.Refresh(context.Background(), a)
	require.NoError(t, err)
	require.Equal(t, "new-access", credentials["access_token"])
	require.Equal(t, kiro.EnvironmentKey(a.Credentials), kiro.EnvironmentKey(credentials))
	assertKiroTransportContext(t, upstream.request)
	require.Equal(t, a.ID, upstream.id)
	require.Equal(t, "http://test:secret@192.0.2.17:8000", upstream.proxy)
	a.Proxy = nil
	_, err = executor.Refresh(context.Background(), a)
	require.Error(t, err)
	require.Equal(t, 1, upstream.calls)
}
func TestNormalizeKiroRequiresBoundProxy(t *testing.T) {
	a := fixedKiroAccount()
	require.NoError(t, normalizeKiroAccount(a))
	a.ProxyID = nil
	require.Error(t, normalizeKiroAccount(a))
}
