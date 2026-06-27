package service

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAccountTestService_TestAccountConnection_OpenAICompactOAuthSuccessPersistsSupport(t *testing.T) {
	gin.SetMode(gin.TestMode)

	updateCalls := make(chan map[string]any, 2)
	account := Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
	}
	repo := &snapshotUpdateAccountRepo{
		stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{account}},
		updateExtraCalls:      updateCalls,
	}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid-probe"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"cmp_probe","status":"completed"}`)),
	}}
	svc := &AccountTestService{
		accountRepo:  repo,
		httpUpstream: upstream,
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/1/test", bytes.NewReader(nil))

	err := svc.TestAccountConnection(c, account.ID, "gpt-5.4", "", AccountTestModeCompact)
	require.NoError(t, err)

	require.Equal(t, chatgptCodexAPIURL+"/compact", upstream.lastReq.URL.String())
	require.Equal(t, "chatgpt.com", upstream.lastReq.Host)
	require.Equal(t, "application/json", upstream.lastReq.Header.Get("Accept"))
	require.Equal(t, codexCLIVersion, upstream.lastReq.Header.Get("Version"))
	require.NotEmpty(t, upstream.lastReq.Header.Get("Session_Id"))
	require.Equal(t, HTTPUpstreamProfileOpenAI, HTTPUpstreamProfileFromContext(upstream.lastReq.Context()))
	require.NotEmpty(t, upstream.lastReq.Header.Get(openAICodexSessionIDHeader))
	require.NotEmpty(t, upstream.lastReq.Header.Get(openAICodexThreadIDHeader))
	require.NotEmpty(t, upstream.lastReq.Header.Get(openAICodexClientRequestIDHeader))
	require.NotEmpty(t, upstream.lastReq.Header.Get(openAICodexInstallationIDHeader))
	require.NotEmpty(t, upstream.lastReq.Header.Get(openAICodexWindowIDHeader))
	require.Equal(t, codexCLIUserAgent, upstream.lastReq.Header.Get("User-Agent"))
	require.Equal(t, "chatgpt-acc", upstream.lastReq.Header.Get("chatgpt-account-id"))
	require.Equal(t, "gpt-5.4", gjson.GetBytes(upstream.lastBody, "model").String())
	promptCacheKey := gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String()
	legacyCompactSeed := compactProbeSessionID(account.ID)
	legacySessionID := isolateOpenAICodexOAuthSessionID(0, legacyCompactSeed, "session")
	legacyThreadID := isolateOpenAICodexOAuthSessionID(0, legacyCompactSeed, "thread")
	require.Equal(t, upstream.lastReq.Header.Get(openAICodexThreadIDHeader), promptCacheKey)
	require.NotEqual(t, legacyCompactSeed, promptCacheKey)
	require.NotEqual(t, legacySessionID, upstream.lastReq.Header.Get(openAICodexSessionIDHeader))
	require.NotEqual(t, legacyThreadID, upstream.lastReq.Header.Get(openAICodexThreadIDHeader))
	require.Contains(t, promptCacheKey, "-")
	require.False(t, gjson.GetBytes(upstream.lastBody, "client_metadata").Exists())

	var compactUpdates map[string]any
	for i := 0; i < 2; i++ {
		updates := <-updateCalls
		if _, ok := updates["openai_compact_supported"]; ok {
			compactUpdates = updates
		}
	}
	require.NotNil(t, compactUpdates)
	require.Equal(t, true, compactUpdates["openai_compact_supported"])
	require.Equal(t, http.StatusOK, compactUpdates["openai_compact_last_status"])
	require.Contains(t, rec.Body.String(), `"type":"test_complete"`)
}

func TestAccountTestService_TestAccountConnection_OpenAICompactOAuthPersistsStateOnlyAfterRepositorySuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	updateCalls := make(chan map[string]any, 2)
	account := Account{
		ID:          12,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
	}
	repo := &snapshotUpdateAccountRepo{
		stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{account}},
		updateExtraCalls:      updateCalls,
		updateExtraErrFor: func(updates map[string]any) error {
			if _, ok := updates["openai_compact_supported"]; ok {
				return errors.New("compact persist failed")
			}
			return nil
		},
	}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":                        []string{"application/json"},
			openAICodexPrimaryUsedPercentHeader:   []string{"64"},
			openAICodexPrimaryResetSecondsHeader:  []string{"604800"},
			openAICodexPrimaryWindowMinutesHeader: []string{"10080"},
			openAICodexSecondUsedPercentHeader:    []string{"32"},
			openAICodexSecondResetSecondsHeader:   []string{"18000"},
			openAICodexSecondWindowMinutesHeader:  []string{"300"},
		},
		Body: io.NopCloser(strings.NewReader(`{"id":"cmp_probe","status":"completed"}`)),
	}}
	svc := &AccountTestService{
		accountRepo:  repo,
		httpUpstream: upstream,
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/12/test", bytes.NewReader(nil))

	err := svc.testOpenAICompactConnection(c, &repo.accounts[0], "gpt-5.4")
	require.Error(t, err)
	require.Contains(t, rec.Body.String(), "Failed to persist compact probe state")
	require.Contains(t, repo.accounts[0].Extra, OpenAICodexFingerprintExtraKey)
	require.NotContains(t, repo.accounts[0].Extra, "openai_compact_supported")
	require.NotContains(t, repo.accounts[0].Extra, "codex_usage_updated_at")
}

func TestAccountTestService_TestAccountConnection_OpenAICompactOAuthErrorSanitizesUpstreamBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	installationID := "550e8400-e29b-41d4-a716-446655440000"
	threadID := "018fed75-1b7e-7000-8000-000000000123"
	accessToken := "compact-secret-access-token"
	account := Account{
		ID:          13,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       accessToken,
			"chatgpt_account_id": "chatgpt-acc",
		},
	}
	repo := &snapshotUpdateAccountRepo{}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusUnauthorized,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"compact denied x-codex-installation-id=` + installationID + ` thread-id=` + threadID + ` Authorization=Bearer ` + accessToken + `"}}`)),
	}}
	svc := &AccountTestService{
		accountRepo:  repo,
		httpUpstream: upstream,
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/13/test", bytes.NewReader(nil))

	err := svc.testOpenAICompactConnection(c, &account, "gpt-5.4")
	require.Error(t, err)
	output := rec.Body.String()
	require.Contains(t, output, "API returned 401")
	require.Contains(t, repo.setErrorMsg, "Authentication failed (401)")
	for _, leaked := range []string{installationID, threadID, accessToken, "compact-secret"} {
		require.NotContains(t, output, leaked)
		require.NotContains(t, repo.setErrorMsg, leaked)
	}
	require.Contains(t, output, "x-codex-installation-id=[redacted]")
	require.Contains(t, output, "thread-id=[redacted]")
	require.Contains(t, repo.setErrorMsg, "Authorization=[redacted]")
}

func TestAccountTestService_TestAccountConnection_OpenAICompactOAuth404MarksUnsupported(t *testing.T) {
	gin.SetMode(gin.TestMode)

	updateCalls := make(chan map[string]any, 2)
	account := Account{
		ID:          2,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
	}
	repo := &snapshotUpdateAccountRepo{
		stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{account}},
		updateExtraCalls:      updateCalls,
	}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusNotFound,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`404 page not found`)),
	}}
	svc := &AccountTestService{
		accountRepo:  repo,
		httpUpstream: upstream,
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/2/test", bytes.NewReader(nil))

	err := svc.TestAccountConnection(c, account.ID, "gpt-5.4", "", AccountTestModeCompact)
	require.Error(t, err)

	var compactUpdates map[string]any
	for i := 0; i < 2; i++ {
		updates := <-updateCalls
		if _, ok := updates["openai_compact_supported"]; ok {
			compactUpdates = updates
		}
	}
	require.NotNil(t, compactUpdates)
	require.Equal(t, false, compactUpdates["openai_compact_supported"])
	require.Equal(t, http.StatusNotFound, compactUpdates["openai_compact_last_status"])
	require.Contains(t, rec.Body.String(), `"type":"error"`)
}

func TestAccountTestService_TestAccountConnection_OpenAICompactAPIKeyUsesCompactPath(t *testing.T) {
	gin.SetMode(gin.TestMode)

	updateCalls := make(chan map[string]any, 1)
	account := Account{
		ID:          3,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":               "sk-test",
			"base_url":              "https://example.com/v1",
			"compact_model_mapping": map[string]any{"gpt-5.4": "gpt-5.4-openai-compact"},
		},
	}
	repo := &snapshotUpdateAccountRepo{
		stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{account}},
		updateExtraCalls:      updateCalls,
	}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"cmp_probe_apikey","status":"completed"}`)),
	}}
	svc := &AccountTestService{
		accountRepo:  repo,
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/3/test", bytes.NewReader(nil))

	err := svc.TestAccountConnection(c, account.ID, "gpt-5.4", "", AccountTestModeCompact)
	require.NoError(t, err)

	require.Equal(t, "https://example.com/v1/responses/compact", upstream.lastReq.URL.String())
	require.Equal(t, "gpt-5.4-openai-compact", gjson.GetBytes(upstream.lastBody, "model").String())
	updates := <-updateCalls
	require.Equal(t, true, updates["openai_compact_supported"])
}

func TestAccountTestService_TestAccountConnection_OpenAICompactAPIKeyDefaultBaseURLUsesV1Path(t *testing.T) {
	gin.SetMode(gin.TestMode)

	updateCalls := make(chan map[string]any, 1)
	account := Account{
		ID:          4,
		Name:        "openai-apikey-default",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key": "sk-test",
		},
	}
	repo := &snapshotUpdateAccountRepo{
		stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{account}},
		updateExtraCalls:      updateCalls,
	}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"cmp_probe_apikey_default","status":"completed"}`)),
	}}
	svc := &AccountTestService{
		accountRepo:  repo,
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/4/test", bytes.NewReader(nil))

	err := svc.TestAccountConnection(c, account.ID, "gpt-5.4", "", AccountTestModeCompact)
	require.NoError(t, err)
	require.Equal(t, "https://api.openai.com/v1/responses/compact", upstream.lastReq.URL.String())
	<-updateCalls
}
