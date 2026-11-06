package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
	"github.com/stretchr/testify/require"
)

func assertKiroTransportContext(t *testing.T, req *http.Request) {
	t.Helper()
	require.Equal(t, HTTPUpstreamProfileKiro, HTTPUpstreamProfileFromContext(req.Context()))
	require.True(t, HTTPUpstreamRedirectsDisabled(req.Context()))
	require.True(t, req.Close)
}

func TestKiroFingerprintDiagnosticsMatchRequestsAndExcludeSecrets(t *testing.T) {
	a := fixedKiroAccount()
	a.Credentials["access_token"] = "private-access"
	a.Credentials["refresh_token"] = "private-refresh"
	a.Credentials["client_secret"] = "private-client"
	a.Credentials["profile_arn"] = "private-profile"
	s := &adminServiceImpl{accountRepo: &fingerprintViewRepo{accounts: map[int64]*Account{a.ID: a}}}
	result, err := s.GetAccountFingerprint(context.Background(), a.ID)
	require.NoError(t, err)
	require.NotNil(t, result.Kiro)
	require.Len(t, result.Kiro.Operations, 4)
	require.False(t, result.Kiro.TLSCaptureVerified)
	require.False(t, result.Kiro.WebSocketSupported)
	for _, op := range result.Kiro.Operations {
		req, err := http.NewRequest(op.Method, op.URL, nil)
		require.NoError(t, err)
		req = prepareKiroHTTPRequest(req, a.Credentials, "private-access", op.Operation)
		assertKiroTransportContext(t, req)
		for key, value := range op.Headers {
			if key == "Connection" {
				require.True(t, req.Close)
			} else {
				require.Equal(t, value, req.Header.Get(key))
			}
		}
	}
	raw, err := json.Marshal(result)
	require.NoError(t, err)
	for _, secret := range []string{"private-access", "private-refresh", "private-client", "private-profile", "test:secret", "192.0.2.17", "Authorization", "amz-sdk-invocation-id"} {
		require.NotContains(t, string(raw), secret)
	}
	require.Equal(t, strings.Repeat("a", 64), result.InstallationID)
	a.Credentials["auth_method"] = "api_key"
	require.Len(t, kiroAccountFingerprintSummary(a).Kiro.Operations, 3)
}

func TestKiroCatalogCacheInvalidatesEnvironmentEdits(t *testing.T) {
	u := &kiroCompatUpstream{t: t}
	s := &GatewayService{httpUpstream: u}
	a := fixedKiroAccount()
	_, err := s.kiroModels(context.Background(), a, "environment-cache-test")
	require.NoError(t, err)
	_, err = s.kiroModels(context.Background(), a, "environment-cache-test")
	require.NoError(t, err)
	require.Equal(t, 1, u.requests)
	a.Credentials["kiro_version"] = "0.11.108"
	_, err = s.kiroModels(context.Background(), a, "environment-cache-test")
	require.NoError(t, err)
	require.Equal(t, 2, u.requests)
}

func TestKiroCredentialUpdatePreservesEnvironmentAtServiceBoundary(t *testing.T) {
	a := fixedKiroAccount()
	require.NoError(t, normalizeKiroAccount(a))
	next := preserveKiroAccountEnvironment(a, map[string]any{"refresh_token": "new"})
	normalized, err := kiro.Normalize(next)
	require.NoError(t, err)
	require.Equal(t, kiro.AccountEnvironment(a.Credentials), kiro.AccountEnvironment(normalized))
	a.Platform = PlatformOpenAI
	require.Equal(t, map[string]any{"refresh_token": "new"}, preserveKiroAccountEnvironment(a, map[string]any{"refresh_token": "new"}))
}

type kiroEnvironmentAccountRepo struct {
	AccountRepository
	account *Account
	updates int
}

func (r *kiroEnvironmentAccountRepo) GetByID(context.Context, int64) (*Account, error) {
	return r.account, nil
}
func (r *kiroEnvironmentAccountRepo) Update(_ context.Context, a *Account) error {
	r.account = a
	r.updates++
	return nil
}

type kiroEnvironmentProxyRepo struct {
	ProxyRepository
	proxy *Proxy
}

func (r *kiroEnvironmentProxyRepo) GetByID(context.Context, int64) (*Proxy, error) {
	return r.proxy, nil
}
func (r *kiroEnvironmentProxyRepo) ListAccountSummariesByProxyID(context.Context, int64) ([]ProxyAccountSummary, error) {
	return nil, nil
}

func TestKiroCredentialUpdateLifecyclePreservesIdentity(t *testing.T) {
	for _, boundary := range []string{"admin", "account", "admin-idc", "account-idc"} {
		for _, nested := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/nested=%v", boundary, nested), func(t *testing.T) {
				a := fixedKiroAccount()
				if strings.HasSuffix(boundary, "-idc") {
					a.Credentials["auth_method"] = "idc"
					a.Credentials["client_id"] = "original-client"
					a.Credentials["client_secret"] = "original-secret"
				}
				a.Credentials["profile_arn"] = "original-profile"
				require.NoError(t, normalizeKiroAccount(a))
				before := kiro.EnvironmentKey(a.Credentials)
				repo := &kiroEnvironmentAccountRepo{account: a}
				incoming := map[string]any{"refreshToken": "replacement-token"}
				if nested {
					incoming = map[string]any{"credentials": incoming}
				}
				var updated *Account
				var err error
				if strings.HasPrefix(boundary, "admin") {
					svc := &adminServiceImpl{accountRepo: repo, proxyRepo: &kiroEnvironmentProxyRepo{proxy: a.Proxy}}
					updated, err = svc.UpdateAccount(context.Background(), a.ID, &UpdateAccountInput{Credentials: incoming})
				} else {
					svc := &AccountService{accountRepo: repo}
					updated, err = svc.Update(context.Background(), a.ID, UpdateAccountRequest{Credentials: &incoming})
				}
				require.NoError(t, err)
				require.Equal(t, 1, repo.updates)
				require.Equal(t, "replacement-token", updated.GetCredential("refresh_token"))
				require.Equal(t, before, kiro.EnvironmentKey(updated.Credentials))
				require.Equal(t, "original-profile", updated.GetCredential("profile_arn"))
			})
		}
	}
}

func TestKiroResponseCacheInvalidatesEnvironmentEdits(t *testing.T) {
	for _, protocol := range []string{"messages", "chat", "responses", "gemini"} {
		t.Run(protocol, func(t *testing.T) {
			up := &kiroCompatUpstream{t: t}
			cache := &kiroMemoryCache{values: map[string][]byte{}}
			groupID := int64(7)
			svc := &GatewayService{httpUpstream: up, cfg: &config.Config{}, cache: cache, groupRepo: &kiroGroupRepo{group: &Group{ID: groupID, KiroResponseCache: KiroCachePolicy{Enabled: true, TargetPercent: 100, TTLSeconds: 300, MaxEntries: 128}}}}
			a := fixedKiroAccount()
			a.Credentials["access_token"] = t.Name()
			a.Credentials["expires_at"] = time.Now().Add(time.Hour).Format(time.RFC3339)
			for i := 0; i < 3; i++ {
				if i == 2 {
					a.Credentials["kiro_version"] = "0.11.108"
				}
				raw := []byte(`{"model":"claude-haiku-4.5","max_tokens":1024,"messages":[{"role":"user","content":"hi"}]}`)
				if protocol == "responses" {
					raw = []byte(`{"model":"claude-haiku-4.5","input":"hi"}`)
				}
				if protocol == "gemini" {
					raw = []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
				}
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest("POST", "/test", bytes.NewReader(raw))
				c.Set("api_key", &APIKey{ID: 1, UserID: 2, GroupID: &groupID})
				var result *ForwardResult
				var err error
				switch protocol {
				case "messages":
					parsed, e := ParseGatewayRequest(NewRequestBodyRef(raw), PlatformAnthropic)
					require.NoError(t, e)
					result, err = svc.Forward(context.Background(), c, a, parsed)
				case "chat":
					result, err = svc.ForwardAsChatCompletions(context.Background(), c, a, raw, nil)
				case "responses":
					result, err = svc.ForwardAsResponses(context.Background(), c, a, raw, nil)
				case "gemini":
					result, err = svc.ForwardKiroGemini(context.Background(), c, a, "claude-haiku-4.5", "generateContent", false, raw)
				}
				require.NoError(t, err)
				if i == 1 {
					require.Equal(t, 11, result.Usage.CacheReadInputTokens)
				} else {
					require.Equal(t, 11, result.Usage.CacheCreationInputTokens)
					require.Zero(t, result.Usage.CacheReadInputTokens)
				}
			}
			require.Equal(t, 4, up.requests, "catalog and inference must both reload after an environment edit")
		})
	}
}
