package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"

	"github.com/gin-gonic/gin"
)

const claudeFilesBetaHeader = "files-api-2025-04-14,oauth-2025-04-20"

// ForwardClaudeFiles transparently proxies Claude Code file API requests.
func (s *GatewayService) ForwardClaudeFiles(ctx context.Context, c *gin.Context, account *Account) error {
	if s == nil || s.httpUpstream == nil {
		return fmt.Errorf("gateway service is not configured")
	}
	if c == nil || c.Request == nil {
		return fmt.Errorf("request is missing")
	}
	if account == nil || !account.IsAnthropicOAuthOrSetupToken() {
		return fmt.Errorf("Claude Files API requires an Anthropic OAuth account")
	}

	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return fmt.Errorf("failed to get OAuth token: %w", err)
	}
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("OAuth token is empty")
	}

	targetURL, err := s.buildClaudeFilesURL(account, c.Request.URL.EscapedPath(), c.Request.URL.RawQuery)
	if err != nil {
		return err
	}

	var body io.Reader
	if c.Request.Body != nil {
		body = c.Request.Body
	}
	upstreamReq, err := http.NewRequestWithContext(ctx, c.Request.Method, targetURL, body)
	if err != nil {
		return err
	}
	copyClaudeFilesHeaders(upstreamReq.Header, c.Request.Header)
	applyClaudeOAuthHeaderDefaults(upstreamReq)
	s.applyGatewayFingerprintForPassthrough(ctx, upstreamReq)
	ensureClaudeClientRequestIDFromClient(upstreamReq, c.Request.Header)
	setHeaderRaw(upstreamReq.Header, "Authorization", "Bearer "+token)
	setHeaderRaw(upstreamReq.Header, "Anthropic-Version", "2023-06-01")
	setHeaderRaw(upstreamReq.Header, "Anthropic-Beta", claudeFilesBetaHeader)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.DoWithTLS(upstreamReq, proxyURL, account.ID, account.Concurrency, s.resolveTLSProfile(account))
	if err != nil {
		return err
	}
	if resp == nil || resp.Body == nil {
		return fmt.Errorf("upstream request failed: empty response")
	}
	defer func() { _ = resp.Body.Close() }()

	if s.rateLimitService != nil {
		s.rateLimitService.UpdateSessionWindow(ctx, account, resp.Header)
	}
	writeAnthropicPassthroughResponseHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/json"
	}
	c.Status(resp.StatusCode)
	c.Header("Content-Type", contentType)
	_, err = io.Copy(c.Writer, resp.Body)
	return err
}

func (s *GatewayService) buildClaudeFilesURL(account *Account, path, rawQuery string) (string, error) {
	baseURL := "https://api.anthropic.com"
	if account != nil && account.IsCustomBaseURLEnabled() {
		customURL := strings.TrimSpace(account.GetCustomBaseURL())
		if customURL == "" {
			return "", fmt.Errorf("custom_base_url is enabled but not configured for account %d", account.ID)
		}
		baseURL = customURL
	}
	validatedURL, err := s.validateClaudeFilesBaseURL(baseURL)
	if err != nil {
		return "", err
	}
	if path == "" {
		path = "/v1/files"
	}
	target := joinClaudeFilesPath(validatedURL, path)
	if rawQuery != "" {
		target += "?" + rawQuery
	}
	return target, nil
}

func (s *GatewayService) validateClaudeFilesBaseURL(raw string) (string, error) {
	if s != nil && s.cfg != nil {
		return s.validateUpstreamBaseURL(raw)
	}
	normalized, err := urlvalidator.ValidateURLFormat(raw, false)
	if err != nil {
		return "", fmt.Errorf("invalid base_url: %w", err)
	}
	return normalized, nil
}

func (s *GatewayService) resolveTLSProfile(account *Account) *tlsfingerprint.Profile {
	if s == nil || s.tlsFPProfileService == nil {
		return nil
	}
	return s.tlsFPProfileService.ResolveTLSProfile(account)
}

func joinClaudeFilesPath(baseURL, path string) string {
	base := strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(base, "/v1") && (path == "/v1" || strings.HasPrefix(path, "/v1/")) {
		path = strings.TrimPrefix(path, "/v1")
		if path == "" {
			path = "/"
		}
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}

func copyClaudeFilesHeaders(dst http.Header, src http.Header) {
	for key, values := range src {
		if isClaudeFilesBlockedHeader(key) {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func isClaudeFilesBlockedHeader(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "authorization",
		"x-api-key",
		"x-goog-api-key",
		"cookie",
		"user-agent",
		"x-app",
		"x-client-request-id",
		"x-claude-code-session-id",
		"x-stainless-arch",
		"x-stainless-helper-method",
		"x-stainless-lang",
		"x-stainless-os",
		"x-stainless-package-version",
		"x-stainless-retry-count",
		"x-stainless-runtime",
		"x-stainless-runtime-version",
		"x-stainless-timeout",
		"host",
		"content-length",
		"connection",
		"keep-alive",
		"proxy-authenticate",
		"proxy-authorization",
		"te",
		"trailer",
		"transfer-encoding",
		"upgrade":
		return true
	default:
		return false
	}
}
