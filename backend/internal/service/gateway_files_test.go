package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGatewayService_ForwardClaudeFiles_UsesOAuthHeadersAndPath(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/files?after_id=file_1", strings.NewReader("multipart-body"))
	c.Request.Header.Set("Authorization", "Bearer inbound-token")
	c.Request.Header.Set("X-Api-Key", "inbound-api-key")
	c.Request.Header.Set("Cookie", "secret=1")
	c.Request.Header.Set("Content-Type", "multipart/form-data; boundary=test")
	c.Request.Header.Set("Anthropic-Beta", "wrong-beta")
	c.Request.Header.Set("User-Agent", "bad-client/1.0")
	c.Request.Header.Set("X-App", "bad-app")
	c.Request.Header.Set("X-Stainless-Lang", "python")
	c.Request.Header.Set("X-Stainless-OS", "Windows")
	c.Request.Header.Set("X-Client-Request-Id", "00000000-0000-4000-8000-000000000123")

	upstream := &anthropicHTTPUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusCreated,
			Header: http.Header{
				"Content-Type": []string{"application/json"},
				"x-request-id": []string{"rid-files"},
				"Set-Cookie":   []string{"secret=upstream"},
			},
			Body: io.NopCloser(strings.NewReader(`{"id":"file_123"}`)),
		},
	}
	cfg := &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{Enabled: false},
		},
	}
	svc := &GatewayService{
		cfg:                  cfg,
		responseHeaderFilter: compileResponseHeaderFilter(cfg),
		httpUpstream:         upstream,
	}
	account := &Account{
		ID:          301,
		Name:        "claude-files-oauth",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeSetupToken,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "upstream-oauth-token",
		},
		Status:      StatusActive,
		Schedulable: true,
	}

	err := svc.ForwardClaudeFiles(context.Background(), c, account)
	require.NoError(t, err)

	require.Equal(t, "https://api.anthropic.com/v1/files?after_id=file_1", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer upstream-oauth-token", getHeaderRaw(upstream.lastReq.Header, "authorization"))
	require.Empty(t, getHeaderRaw(upstream.lastReq.Header, "x-api-key"))
	require.Empty(t, getHeaderRaw(upstream.lastReq.Header, "cookie"))
	require.Equal(t, "multipart/form-data; boundary=test", getHeaderRaw(upstream.lastReq.Header, "content-type"))
	require.Equal(t, "2023-06-01", getHeaderRaw(upstream.lastReq.Header, "anthropic-version"))
	require.Equal(t, claudeFilesBetaHeader, getHeaderRaw(upstream.lastReq.Header, "anthropic-beta"))
	require.Equal(t, claude.DefaultHeaders["User-Agent"], getHeaderRaw(upstream.lastReq.Header, "user-agent"))
	require.Equal(t, claude.DefaultHeaders["X-App"], getHeaderRaw(upstream.lastReq.Header, "x-app"))
	require.Equal(t, claude.DefaultHeaders["X-Stainless-Lang"], getHeaderRaw(upstream.lastReq.Header, "x-stainless-lang"))
	require.Equal(t, claude.DefaultHeaders["X-Stainless-OS"], getHeaderRaw(upstream.lastReq.Header, "x-stainless-os"))
	require.Equal(t, "00000000-0000-4000-8000-000000000123", getHeaderRaw(upstream.lastReq.Header, "x-client-request-id"))
	require.Equal(t, []byte("multipart-body"), upstream.lastBody)

	require.Equal(t, http.StatusCreated, rec.Code)
	require.JSONEq(t, `{"id":"file_123"}`, rec.Body.String())
	require.Equal(t, "rid-files", rec.Header().Get("x-request-id"))
	require.Empty(t, rec.Header().Get("Set-Cookie"))
}

func TestBuildClaudeFilesURL_CustomBaseAvoidsDoubleV1(t *testing.T) {
	svc := &GatewayService{}
	account := &Account{
		ID:       302,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"custom_base_url_enabled": true,
			"custom_base_url":         "https://relay.example.com/v1",
		},
	}

	got, err := svc.buildClaudeFilesURL(account, "/v1/files/file_123/content", "download=1")
	require.NoError(t, err)
	require.Equal(t, "https://relay.example.com/v1/files/file_123/content?download=1", got)
}

func TestBuildClaudeFilesURL_CustomBaseEnabledRequiresURL(t *testing.T) {
	svc := &GatewayService{}
	account := &Account{
		ID:       303,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"custom_base_url_enabled": true,
		},
	}

	_, err := svc.buildClaudeFilesURL(account, "/v1/files", "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "custom_base_url is enabled")
}

func TestForwardClaudeFilesRejectsAPIKeyAccount(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/files", nil)

	svc := &GatewayService{httpUpstream: &anthropicHTTPUpstreamRecorder{}}
	err := svc.ForwardClaudeFiles(context.Background(), c, &Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey})
	require.Error(t, err)
	require.Contains(t, err.Error(), "OAuth")
}
