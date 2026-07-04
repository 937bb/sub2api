package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAccountTestService_OpenAIImageOAuthHandlesOutputItemDoneFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/1/test", nil)

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type": []string{"text/event-stream"},
			},
			Body: io.NopCloser(strings.NewReader(
				"data: {\"type\":\"response.output_item.done\",\"item\":{\"id\":\"ig_123\",\"type\":\"image_generation_call\",\"result\":\"aGVsbG8=\",\"revised_prompt\":\"draw a cat\",\"output_format\":\"png\"}}\n\n" +
					"data: {\"type\":\"response.completed\",\"response\":{\"created_at\":1710000006,\"tool_usage\":{\"image_gen\":{\"images\":1}},\"output\":[]}}\n\n" +
					"data: [DONE]\n\n",
			)),
		},
	}
	svc := &AccountTestService{httpUpstream: upstream}
	account := &Account{
		ID:       53,
		Name:     "openai-oauth",
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":               "token-123",
			"chatgpt_account_id":         "chatgpt-acc",
			"chatgpt_account_is_fedramp": true,
		},
	}

	err := svc.testOpenAIImageOAuth(c, context.Background(), account, "gpt-image-2", "draw a cat")
	require.NoError(t, err)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, HTTPUpstreamProfileOpenAI, HTTPUpstreamProfileFromContext(upstream.lastReq.Context()))
	require.Equal(t, codexOfficialOriginator, upstream.lastReq.Header.Get("originator"))
	require.Equal(t, codexCLIVersion, upstream.lastReq.Header.Get("Version"))
	require.Equal(t, "chatgpt-acc", upstream.lastReq.Header.Get("chatgpt-account-id"))
	require.Equal(t, "true", upstream.lastReq.Header.Get("x-openai-fedramp"))
	require.NotEmpty(t, upstream.lastReq.Header.Get(openAICodexSessionIDHeader))
	require.NotEmpty(t, upstream.lastReq.Header.Get(openAICodexThreadIDHeader))
	require.NotEmpty(t, upstream.lastReq.Header.Get(openAICodexClientRequestIDHeader))
	require.NotEmpty(t, upstream.lastReq.Header.Get(openAICodexInstallationIDHeader))
	require.NotEmpty(t, upstream.lastReq.Header.Get(openAICodexWindowIDHeader))
	promptCacheKey := gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String()
	require.Equal(t, upstream.lastReq.Header.Get(openAICodexThreadIDHeader), promptCacheKey)
	require.NotEqual(t, "probe_openai_image", promptCacheKey)
	require.Contains(t, promptCacheKey, "-")
	require.Equal(t, upstream.lastReq.Header.Get(openAICodexInstallationIDHeader), gjson.GetBytes(upstream.lastBody, "client_metadata.x-codex-installation-id").String())
	// Codex HTTP 不在 client_metadata 中放 x-codex-window-id，仅放在 HTTP header。
	require.False(t, gjson.GetBytes(upstream.lastBody, "client_metadata.x-codex-window-id").Exists())
	require.Contains(t, rec.Body.String(), "Calling Codex /responses image tool")
	require.Contains(t, rec.Body.String(), "data:image/png;base64,aGVsbG8=")
	require.Contains(t, rec.Body.String(), "\"success\":true")
}

func TestAccountTestService_OpenAIImageOAuthPersistsSnapshotFromHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/1/test", nil)

	repo := &snapshotUpdateAccountRepo{updateExtraCalls: make(chan map[string]any, 2)}
	headers := http.Header{}
	headers.Set("Content-Type", "text/event-stream")
	headers.Set(openAICodexPrimaryUsedPercentHeader, "77")
	headers.Set(openAICodexPrimaryResetSecondsHeader, "604800")
	headers.Set(openAICodexPrimaryWindowMinutesHeader, "10080")
	headers.Set(openAICodexSecondUsedPercentHeader, "55")
	headers.Set(openAICodexSecondResetSecondsHeader, "18000")
	headers.Set(openAICodexSecondWindowMinutesHeader, "300")
	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     headers,
			Body: io.NopCloser(strings.NewReader(
				"data: {\"type\":\"response.output_item.done\",\"item\":{\"id\":\"ig_123\",\"type\":\"image_generation_call\",\"result\":\"aGVsbG8=\",\"output_format\":\"png\"}}\n\n" +
					"data: {\"type\":\"response.completed\",\"response\":{\"output\":[]}}\n\n" +
					"data: [DONE]\n\n",
			)),
		},
	}
	svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream}
	account := &Account{
		ID:       531,
		Name:     "openai-oauth",
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "token-123",
		},
	}

	err := svc.testOpenAIImageOAuth(c, context.Background(), account, "gpt-image-2", "draw a cat")
	require.NoError(t, err)
	require.Contains(t, rec.Body.String(), "\"success\":true")

	var snapshotUpdates map[string]any
	deadline := time.After(time.Second)
	for snapshotUpdates == nil {
		select {
		case updates := <-repo.updateExtraCalls:
			if _, ok := updates["codex_usage_updated_at"]; ok {
				snapshotUpdates = updates
			}
		case <-deadline:
			t.Fatal("expected image OAuth probe to persist Codex usage snapshot")
		}
	}
	require.NotNil(t, snapshotUpdates)
	require.Equal(t, 77.0, snapshotUpdates["codex_7d_used_percent"])
	require.Equal(t, 55.0, snapshotUpdates["codex_5h_used_percent"])
	require.Equal(t, 77.0, account.Extra["codex_7d_used_percent"])
}

func TestAccountTestService_OpenAIImageOAuthPersistsSnapshotOnlyAfterRepositorySuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/1/test", nil)

	repo := &snapshotUpdateAccountRepo{
		updateExtraCalls: make(chan map[string]any, 2),
		updateExtraErrFor: func(updates map[string]any) error {
			if _, ok := updates["codex_usage_updated_at"]; ok {
				return errors.New("snapshot persist failed")
			}
			return nil
		},
	}
	headers := http.Header{}
	headers.Set("Content-Type", "text/event-stream")
	headers.Set(openAICodexPrimaryUsedPercentHeader, "77")
	headers.Set(openAICodexPrimaryResetSecondsHeader, "604800")
	headers.Set(openAICodexPrimaryWindowMinutesHeader, "10080")
	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     headers,
			Body: io.NopCloser(strings.NewReader(
				"data: {\"type\":\"response.output_item.done\",\"item\":{\"id\":\"ig_123\",\"type\":\"image_generation_call\",\"result\":\"aGVsbG8=\",\"output_format\":\"png\"}}\n\n" +
					"data: [DONE]\n\n",
			)),
		},
	}
	svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream}
	account := &Account{
		ID:       532,
		Name:     "openai-oauth",
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "token-123",
		},
	}

	err := svc.testOpenAIImageOAuth(c, context.Background(), account, "gpt-image-2", "draw a cat")
	require.Error(t, err)
	require.Contains(t, rec.Body.String(), "Failed to persist Codex image probe snapshot")
	require.Contains(t, account.Extra, OpenAICodexFingerprintExtraKey)
	require.NotContains(t, account.Extra, "codex_usage_updated_at")
	require.NotContains(t, account.Extra, "codex_7d_used_percent")
}

func TestAccountTestService_OpenAIImageOAuthErrorSanitizesUpstreamBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/1/test", nil)

	installationID := "550e8400-e29b-41d4-a716-446655440000"
	threadID := "018fed75-1b7e-7000-8000-000000000123"
	accessToken := "image-secret-access-token"
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusUnauthorized,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"image denied x-codex-installation-id=` + installationID + ` thread-id=` + threadID + ` Authorization=Bearer ` + accessToken + `"}}`)),
	}}
	svc := &AccountTestService{httpUpstream: upstream}
	account := &Account{
		ID:       533,
		Name:     "openai-oauth",
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": accessToken,
		},
	}

	err := svc.testOpenAIImageOAuth(c, context.Background(), account, "gpt-image-2", "draw a cat")
	require.Error(t, err)
	output := rec.Body.String()
	require.Contains(t, output, "API returned 401")
	for _, leaked := range []string{installationID, threadID, accessToken, "image-secret"} {
		require.NotContains(t, output, leaked)
	}
	require.Contains(t, output, "x-codex-installation-id=[redacted]")
	require.Contains(t, output, "thread-id=[redacted]")
	require.Contains(t, output, "Authorization=[redacted]")
}

func TestAccountTestService_OpenAIImageAPIKeyUsesConfiguredV1BaseURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/1/test", nil)

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type": []string{"application/json"},
			},
			Body: io.NopCloser(strings.NewReader(`{"data":[{"b64_json":"aGVsbG8=","revised_prompt":"draw a cat"}]}`)),
		},
	}
	svc := &AccountTestService{
		httpUpstream: upstream,
		cfg:          &config.Config{},
	}
	account := &Account{
		ID:       54,
		Name:     "openai-apikey",
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "test-api-key",
			"base_url": "https://image-upstream.example/v1",
		},
	}

	err := svc.testOpenAIImageAPIKey(c, context.Background(), account, "gpt-image-2", "draw a cat")
	require.NoError(t, err)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, HTTPUpstreamProfileOpenAI, HTTPUpstreamProfileFromContext(upstream.lastReq.Context()))
	require.Equal(t, "https://image-upstream.example/v1/images/generations", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer test-api-key", upstream.lastReq.Header.Get("Authorization"))
	require.Empty(t, upstream.lastReq.Header.Get("x-openai-fedramp"))
	require.Contains(t, rec.Body.String(), "data:image/png;base64,aGVsbG8=")
	require.Contains(t, rec.Body.String(), "\"success\":true")
}
