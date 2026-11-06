package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

type kiroUsageUpstream struct {
	t     *testing.T
	calls int
	code  int
}

func (u *kiroUsageUpstream) Do(r *http.Request, p string, id int64, _ int) (*http.Response, error) {
	u.calls++
	assertKiroTransportContext(u.t, r)
	require.Equal(u.t, "http://test:secret@192.0.2.17:8000", p)
	require.EqualValues(u.t, 19, id)
	require.Equal(u.t, "/getUsageLimits", r.URL.Path)
	require.Equal(u.t, "AI_EDITOR", r.URL.Query().Get("origin"))
	require.Contains(u.t, r.Header.Get("User-Agent"), "api/codewhispererruntime#1.0.0")
	require.NotEmpty(u.t, r.Header.Get("amz-sdk-invocation-id"))
	return &http.Response{StatusCode: u.code, Body: io.NopCloser(strings.NewReader(`{"usageBreakdownList":[{"currentUsage":5,"usageLimit":50}]}`))}, nil
}
func (u *kiroUsageUpstream) DoWithTLS(r *http.Request, p string, id int64, n int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(r, p, id, n)
}
func TestKiroUsageRoutingCacheAndProxyIsolation(t *testing.T) {
	u := &kiroUsageUpstream{t: t, code: 200}
	s := &AccountUsageService{kiroGateway: &GatewayService{httpUpstream: u}}
	a := fixedKiroAccount()
	a.Credentials["expires_at"] = time.Now().Add(time.Hour).Format(time.RFC3339)
	a.Credentials["access_token"] = "test"
	info, err := s.GetUsageForAccount(context.Background(), a)
	require.NoError(t, err)
	require.Equal(t, 10.0, *info.KiroQuota.Utilization)
	_, err = s.GetUsageForAccount(context.Background(), a)
	require.NoError(t, err)
	require.Equal(t, 1, u.calls)
	_, err = s.GetUsageForAccount(context.Background(), a, true)
	require.NoError(t, err)
	require.Equal(t, 2, u.calls)
	a.Credentials["machine_id"] = strings.Repeat("b", 64)
	_, err = s.GetUsageForAccount(context.Background(), a)
	require.NoError(t, err)
	require.Equal(t, 3, u.calls)
	u.code = 403
	info, err = s.GetUsageForAccount(context.Background(), a, true)
	require.NoError(t, err)
	require.Contains(t, info.Error, "HTTP 403")
	require.Nil(t, info.KiroQuota)
	a.Proxy.Status = "inactive"
	_, err = s.GetUsageForAccount(context.Background(), a)
	require.Error(t, err)
	require.Equal(t, 4, u.calls)
}
