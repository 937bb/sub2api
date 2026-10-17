package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/imroc/req/v3"
	"github.com/stretchr/testify/require"
)

const siwcWebQuotaFixture = `{"email":"owner@example.com","rate_limit":{"primary_window":{"used_percent":98,"limit_window_seconds":18000,"reset_after_seconds":8580},"secondary_window":{"used_percent":15,"limit_window_seconds":604800,"reset_after_seconds":590000}}}`

func siwcWebQuotaAccount() *Account {
	a := newSIWCIsolationAccount()
	a.Credentials["email"] = "owner@example.com"
	a.Credentials[siwcQuotaAccessTokenKey] = "web-quota-token"
	return a
}

func TestSIWCWebQuotaReadOnlyAndIdentity(t *testing.T) {
	for _, tc := range []struct {
		name, body, wantCode string
		status               int
	}{
		{"valid", siwcWebQuotaFixture, "", 200},
		{"wrong account", `{"email":"other@example.com","rate_limit":{}}`, "siwc_quota_account_mismatch", 200},
		{"missing identity", `{"rate_limit":{}}`, "siwc_quota_account_mismatch", 200},
		{"empty data", `{}`, "siwc_quota_invalid_response", 200},
		{"null windows", `{"email":"owner@example.com","rate_limit":{"primary_window":null,"secondary_window":null}}`, "siwc_quota_invalid_response", 200},
		{"missing percentage", `{"email":"owner@example.com","rate_limit":{"primary_window":{"limit_window_seconds":18000}}}`, "siwc_quota_invalid_response", 200},
		{"zero length window", `{"email":"owner@example.com","rate_limit":{"primary_window":{"used_percent":0,"limit_window_seconds":0}}}`, "siwc_quota_invalid_response", 200},
		{"expired token", `{"error":"echo web-quota-token"}`, "siwc_quota_web_token_invalid", 401},
		{"forbidden", `denied`, "siwc_quota_web_forbidden", 403},
		{"limited", `limited`, "siwc_quota_query_limited", 429},
		{"html", `<html>web-quota-token</html>`, "siwc_quota_invalid_response", 200},
		{"server error", `web-quota-token`, "siwc_quota_unavailable", 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			account := siwcWebQuotaAccount()
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				require.Equal(t, http.MethodGet, r.Method)
				require.Equal(t, "/backend-api/wham/usage", r.URL.Path)
				require.Equal(t, "Bearer web-quota-token", r.Header.Get("Authorization"))
				require.Empty(t, r.Header.Get("ChatGPT-Account-ID"), "must not reuse SIWC identity headers")
				require.Empty(t, r.Header.Get("Cookie"))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			svc := &OpenAIQuotaService{privacyClientFactory: newQuotaRedirectingFactory(srv)}
			usage, err := svc.querySIWCWebUsage(context.Background(), account)
			require.Equal(t, 1, calls)
			if tc.wantCode != "" {
				require.Equal(t, tc.wantCode, infraerrors.Reason(err))
				require.NotContains(t, err.Error(), "web-quota-token")
				require.Nil(t, usage)
				return
			}
			require.NoError(t, err)
			require.Equal(t, 98.0, usage.FiveHour.Utilization)
			require.Equal(t, 15.0, usage.SevenDay.Utilization)
			require.NotNil(t, usage.FiveHour.ResetsAt)
			require.Equal(t, "siwc-test-token", account.GetOpenAIAccessToken())
			require.True(t, account.Schedulable == newSIWCIsolationAccount().Schedulable)
		})
	}
}

func TestSIWCWebQuotaIsolationAndRedirects(t *testing.T) {
	account := siwcWebQuotaAccount()
	for _, tc := range []struct {
		name string
		edit func(*Account)
		code string
	}{
		{"no web token", func(a *Account) { delete(a.Credentials, siwcQuotaAccessTokenKey) }, "siwc_quota_web_token_required"},
		{"no email", func(a *Account) { delete(a.Credentials, "email") }, "siwc_quota_identity_missing"},
		{"unresolved proxy", func(a *Account) { id := int64(77); a.ProxyID = &id }, "siwc_quota_proxy_unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := siwcWebQuotaAccount()
			tc.edit(a)
			svc := &OpenAIQuotaService{privacyClientFactory: func(string) (*req.Client, error) {
				t.Fatal("must not request upstream with missing credentials or proxy")
				return nil, nil
			}}
			_, err := svc.querySIWCWebUsage(context.Background(), a)
			require.Equal(t, tc.code, infraerrors.Reason(err))
		})
	}
	redirected := 0
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected++ }))
	defer target.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	svc := &OpenAIQuotaService{privacyClientFactory: newQuotaRedirectingFactory(srv)}
	_, err := svc.querySIWCWebUsage(context.Background(), account)
	require.Error(t, err)
	require.Zero(t, redirected)
}

func TestSIWCWebQuotaCachingAndCredentialRotation(t *testing.T) {
	account := siwcWebQuotaAccount()
	calls, status := 0, 200
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(siwcWebQuotaFixture))
	}))
	defer srv.Close()
	svc := &AccountUsageService{cache: NewUsageCache(), openAIQuotaService: &OpenAIQuotaService{privacyClientFactory: newQuotaRedirectingFactory(srv)}}
	ctx := context.Background()
	usage := svc.getSIWCUsage(ctx, account, false)
	require.Empty(t, usage.ErrorCode)
	usage.FiveHour.Utilization = 0
	usage = svc.getSIWCUsage(ctx, account, false)
	require.Equal(t, 98.0, usage.FiveHour.Utilization, "callers must not mutate the cached snapshot")
	require.Equal(t, 1, calls)
	_ = svc.getSIWCUsage(ctx, account, true)
	require.Equal(t, 2, calls)
	status = 401
	usage = svc.getSIWCUsage(ctx, account, true)
	require.Equal(t, "siwc_quota_web_token_invalid", usage.ErrorCode)
	require.Nil(t, usage.FiveHour)
	_ = svc.getSIWCUsage(ctx, account, true)
	require.Equal(t, 3, calls, "negative cache also throttles manual retries")
	account.Credentials[siwcQuotaAccessTokenKey] = "replacement-web-token"
	status = 200
	usage = svc.getSIWCUsage(ctx, account, false)
	require.Empty(t, usage.ErrorCode)
	require.Equal(t, 4, calls)
	delete(account.Credentials, siwcQuotaAccessTokenKey)
	usage = svc.getSIWCUsage(ctx, account, false)
	require.Equal(t, "siwc_quota_web_token_required", usage.ErrorCode)
	require.Nil(t, usage.FiveHour)
	require.Equal(t, 4, calls)
}

func TestSIWCWebQuotaWindowSemantics(t *testing.T) {
	now := time.Now()
	used5h, used7d := 98.0, 15.0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"email": "OWNER@example.com", "rate_limit": map[string]any{
			"primary_window":   siwcWebQuotaWindow{UsedPercent: &used7d, LimitWindowSeconds: 604800, ResetAt: now.Add(time.Hour).Unix()},
			"secondary_window": siwcWebQuotaWindow{UsedPercent: &used5h, LimitWindowSeconds: 18000, ResetAt: now.Add(-time.Minute).Unix()},
		}})
	}))
	defer srv.Close()
	svc := &OpenAIQuotaService{privacyClientFactory: newQuotaRedirectingFactory(srv)}
	usage, err := svc.querySIWCWebUsage(context.Background(), siwcWebQuotaAccount())
	require.NoError(t, err)
	require.Nil(t, usage.FiveHour, "expired windows must not become 100% remaining")
	require.Equal(t, 15.0, usage.SevenDay.Utilization)
	reset := now.Add(-time.Second)
	usage.SevenDay.ResetsAt = &reset
	require.Nil(t, cloneSIWCQuotaUsage(usage).SevenDay)
}
