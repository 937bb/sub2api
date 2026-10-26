package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

func TestCodexMacOSFingerprintUsesDesktopIdentityAndDropsVersionHeader(t *testing.T) {
	SetCodexMacOSFingerprintConfig(true, "26.6.2", "26.1013.40934")
	SetCodexCanonicalUserAgentResolver(func() string {
		return "codex-tui/0.200.1" + codexCLIUserAgentSuffix
	})
	t.Cleanup(func() {
		SetCodexMacOSFingerprintConfig(false, "", "")
		SetCodexCanonicalUserAgentResolver(nil)
	})

	wantUA := "Codex Desktop/0.200.1 (Mac OS 26.6.2; arm64) dumb (codex_desktop; 26.1013.40934)"
	require.Equal(t, wantUA, CodexCanonicalUserAgent())
	require.Equal(t, "0.200.1", CodexCanonicalClientVersion())

	header := http.Header{
		"Originator": {openai.CodexDefaultOriginator},
		"User-Agent": {"codex-tui/0.100.0 (Linux)"},
		"Version":    {"0.100.0"},
	}
	enforceCodexIdentityHeadersWithUA(header, "codex-tui/0.199.0 (Linux)")
	require.Equal(t, codexDesktopOriginator, header.Get("originator"))
	require.Equal(t, wantUA, header.Get("user-agent"))
	require.Empty(t, header.Get("version"))
}

func TestCodexMacOSFingerprintDoesNotChangeAPIKeyProbeIdentity(t *testing.T) {
	SetCodexMacOSFingerprintConfig(true, "26.6.2", "26.1013.40934")
	t.Cleanup(func() { SetCodexMacOSFingerprintConfig(false, "", "") })

	header := make(http.Header)
	applyOpenAICodexProbeHeaders(header)
	require.Equal(t, openai.CodexDefaultOriginator, header.Get("originator"))
	require.True(t, strings.HasPrefix(header.Get("user-agent"), openai.CodexDefaultOriginator+"/"))
	require.NotEmpty(t, header.Get("version"))
}

type macOSFingerprintUpstream struct {
	doCalls        int
	doWithTLSCalls int
	profile        *tlsfingerprint.Profile
}

func (u *macOSFingerprintUpstream) response() *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("ok")),
	}
}

func (u *macOSFingerprintUpstream) Do(*http.Request, string, int64, int) (*http.Response, error) {
	u.doCalls++
	return u.response(), nil
}

func (u *macOSFingerprintUpstream) DoWithTLS(_ *http.Request, _ string, _ int64, _ int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	u.doWithTLSCalls++
	u.profile = profile
	return u.response(), nil
}

func TestOpenAIMacOSFingerprintTransportIsOAuthOnly(t *testing.T) {
	SetCodexMacOSFingerprintConfig(true, "26.6.2", "26.1013.40934")
	t.Cleanup(func() { SetCodexMacOSFingerprintConfig(false, "", "") })

	upstream := &macOSFingerprintUpstream{}
	service := &OpenAIGatewayService{httpUpstream: upstream}
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://chatgpt.com/backend-api/codex/responses", nil)
	require.NoError(t, err)

	response, err := service.doOpenAIUpstream(request, "", &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth})
	require.NoError(t, err)
	_ = response.Body.Close()
	require.Equal(t, 1, upstream.doWithTLSCalls)
	require.Equal(t, "Codex Desktop macOS arm64", upstream.profile.Name)

	response, err = service.doOpenAIUpstream(request, "", &Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey})
	require.NoError(t, err)
	_ = response.Body.Close()
	require.Equal(t, 1, upstream.doCalls)
	require.Equal(t, 1, upstream.doWithTLSCalls)

	response, err = service.doOpenAIUpstream(request, "", &Account{ID: 3, Platform: PlatformAnthropic, Type: AccountTypeOAuth})
	require.NoError(t, err)
	_ = response.Body.Close()
	require.Equal(t, 2, upstream.doCalls)
	require.Equal(t, 1, upstream.doWithTLSCalls)
}

func TestCodexMacOSFingerprintKeepsOAuthOnHTTPSSE(t *testing.T) {
	SetCodexMacOSFingerprintConfig(true, "26.6.2", "26.1013.40934")
	t.Cleanup(func() { SetCodexMacOSFingerprintConfig(false, "", "") })

	decision := NewOpenAIWSProtocolResolver(nil).Resolve(&Account{
		ID:       1,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
	})
	require.Equal(t, OpenAIUpstreamTransportHTTPSSE, decision.Transport)
	require.Equal(t, "oauth_macos_fingerprint_http_only", decision.Reason)
}
