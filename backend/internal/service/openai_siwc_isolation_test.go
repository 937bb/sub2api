package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
	"github.com/imroc/req/v3"
	"github.com/stretchr/testify/require"
)

func newSIWCIsolationAccount() *Account {
	return &Account{ID: 901, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive,
		Credentials: map[string]any{"auth_mode": "siwc", "access_token": "siwc-test-token", "chatgpt_account_id": "must-not-use"},
		Extra:       map[string]any{OpenAIAutoResetCreditEnabledExtraKey: true}}
}

func TestSIWCBackgroundIsolation(t *testing.T) {
	ctx := context.Background()
	account := newSIWCIsolationAccount()
	factory := func(string) (*req.Client, error) {
		t.Error("SIWC must not create a ChatGPT privacy client")
		return nil, errors.New("unexpected privacy client")
	}
	admin := &adminServiceImpl{privacyClientFactory: factory}
	require.Empty(t, admin.EnsureOpenAIPrivacy(ctx, account))
	require.Empty(t, admin.ForceOpenAIPrivacy(ctx, account))
	refresh := &TokenRefreshService{privacyClientFactory: factory}
	refresh.ensureOpenAIPrivacy(ctx, account)

	usageService := &AccountUsageService{}
	usage, err := usageService.getUsageForAccount(ctx, account, true)
	require.NoError(t, err)
	require.Equal(t, "passive", usage.Source)
	require.Equal(t, "siwc_quota_unsupported", usage.ErrorCode)
	require.Nil(t, usage.FiveHour)
	require.Nil(t, usage.SevenDay)
	require.False(t, shouldRefreshOpenAICodexSnapshot(account, nil, time.Now()))
	_, err = usageService.probeOpenAICodexSnapshot(ctx, account)
	require.ErrorContains(t, err, "SIWC")

	require.False(t, ResolveOpenAIAutoResetCreditConfig(account).Enabled)
	quota := &OpenAIQuotaService{accountRepo: newStubCredRepo(account), privacyClientFactory: factory}
	_, _, _, _, err = quota.prepareUpstreamCall(ctx, account.ID)
	require.ErrorContains(t, err, "SIWC")

	require.Contains(t, openAICodexStateProbeUnsupportedReason(account, "gpt-6-astra", true), "SIWC")
	_, err = (&OpenAIGatewayService{}).fireOpenAICodexStateShot(ctx, account, "test", "gpt-6-astra", "", "", "")
	require.ErrorContains(t, err, "SIWC")
	guard := &AccountTokenGuardService{}
	require.Equal(t, "siwc_probe_unsupported", guard.probe(ctx, AccountTokenGuardConfig{}, account).Diagnostic.Code)
	state, detail := (&AccountTokenGuardV2Service{}).probeAccount(ctx, account)
	require.Equal(t, AccountTokenGuardV2ProbeTransient, state)
	require.Contains(t, detail, "SIWC")
}

func TestSIWCShadowAndManifestIsolation(t *testing.T) {
	ctx := context.Background()
	account := newSIWCIsolationAccount()
	shadow := &Account{ID: 902, Platform: PlatformOpenAI, Type: AccountTypeOAuth, ParentAccountID: &account.ID}
	repo := newStubCredRepo(account)
	_, err := resolveCredentialAccount(ctx, repo, shadow)
	require.Error(t, err)
	require.False(t, parentHealthyForShadow(shadow, func(int64) *Account { return account }))
	_, err = (&adminServiceImpl{accountRepo: repo}).CreateShadow(ctx, account.ID, ShadowOptions{})
	require.Error(t, err)
	_, err = (&OpenAIGatewayService{}).FetchCodexModelsManifest(ctx, account, "", "")
	require.ErrorContains(t, err, "SIWC")
	_, err = (&AccountTestService{}).buildOpenAIOAuthUpstreamModelsRequest(ctx, account)
	require.Error(t, err)
	capabilities := accountCodexToolCapabilities(account, "gpt-6-astra")
	require.Equal(t, "false", string(capabilities["use_responses_lite"]))
	require.Equal(t, "null", string(capabilities["tool_mode"]))

	ordinary := &Account{ID: account.ID, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive,
		Credentials: map[string]any{"access_token": "ordinary-token"}, Extra: account.Extra}
	_, err = resolveCredentialAccount(ctx, newStubCredRepo(ordinary), shadow)
	require.NoError(t, err)
	require.True(t, parentHealthyForShadow(shadow, func(int64) *Account { return ordinary }))
	require.True(t, ResolveOpenAIAutoResetCreditConfig(ordinary).Enabled)
}

type siwcCatalogRoundTripper func(*http.Request) (*http.Response, error)

func (f siwcCatalogRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type siwcCatalogRepo struct {
	AccountRepository
	models  []string
	changed bool
}

func (r *siwcCatalogRepo) UpdateOpenAISiwcModels(_ context.Context, _ int64, _, _ string, models []string) (bool, error) {
	if r.changed {
		return false, nil
	}
	r.models = append([]string(nil), models...)
	return true, nil
}

func TestSIWCPublicModelsUseDedicatedCatalog(t *testing.T) {
	account := newSIWCIsolationAccount()
	proxyID := int64(999091)
	account.ProxyID = &proxyID
	account.Proxy = &Proxy{ID: proxyID, Protocol: "http", Host: "127.0.0.1", Port: 9991}
	proxyURL := account.Proxy.URL()
	client, err := httpclient.GetClient(httpclient.Options{ProxyURL: proxyURL})
	require.NoError(t, err)
	originalTransport := client.Transport
	calls := 0
	client.Transport = siwcCatalogRoundTripper(func(r *http.Request) (*http.Response, error) {
		calls++
		require.Equal(t, "https://api.openai.com/v1/models", r.URL.String())
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "Bearer siwc-test-token", r.Header.Get("Authorization"))
		require.Empty(t, r.Header.Get("ChatGPT-Account-ID"))
		require.Empty(t, r.Header.Get("Originator"))
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header),
			Body: io.NopCloser(strings.NewReader(`{"models":[{"slug":"gpt-visible","visibility":"list"},{"slug":"gpt-hidden","visibility":"hidden"}]}`))}, nil
	})
	t.Cleanup(func() { client.Transport = originalTransport; httpclient.EvictProxyClients(proxyURL) })
	repo := &siwcCatalogRepo{}
	gateway := &OpenAIGatewayService{accountRepo: repo}
	response, err := gateway.FetchOpenAIModelsList(context.Background(), account)
	require.NoError(t, err)
	require.JSONEq(t, `{"object":"list","data":[{"id":"gpt-visible","object":"model","owned_by":"openai","created":0}]}`, string(response.Body))
	require.NotEmpty(t, response.ETag)
	require.Equal(t, []string{"gpt-visible"}, repo.models)
	require.True(t, account.IsModelSupported("gpt-visible"))
	require.False(t, account.IsModelSupported("gpt-hidden"))
	admin := &AccountTestService{openaiGatewayService: gateway}
	catalog, err := admin.SyncUpstreamModelCatalog(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, []string{"gpt-visible"}, catalog.Models)
	require.Empty(t, catalog.Metadata)
	require.Equal(t, 2, calls)
	repo.changed = true
	_, err = gateway.fetchSIWCModels(context.Background(), account)
	require.ErrorContains(t, err, "identity changed")
	require.Equal(t, 3, calls)

	account.Proxy = nil
	_, err = gateway.fetchSIWCModels(context.Background(), account)
	require.ErrorContains(t, err, "proxy")
	require.Equal(t, 3, calls)
}
