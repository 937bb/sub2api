package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type codexDiagnosticRepoStub struct {
	AccountRepository
	account *Account
}

func (r *codexDiagnosticRepoStub) GetByID(context.Context, int64) (*Account, error) {
	return r.account, nil
}

type codexDiagnosticUpstreamStub struct {
	HTTPUpstream
	requests []*http.Request
}

func (u *codexDiagnosticUpstreamStub) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	u.requests = append(u.requests, req)
	return &http.Response{StatusCode: http.StatusTooManyRequests, Header: make(http.Header),
		Body: io.NopCloser(strings.NewReader(`{"error":{"message":"rate limited"}}`))}, nil
}

func codexDiagnosticContext() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/91/test", nil)
	return c
}

func TestAccountDiagnosticUserAgentDoesNotMutateLiveCredentials(t *testing.T) {
	const candidate = "codex-tui/0.153.3 (Mac OS 26.5.1; arm64) iTerm.app/3.6.11 (codex-tui; 0.153.3)"
	account := &Account{ID: 91, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "test-token"}, Status: StatusActive}
	repo := &codexDiagnosticRepoStub{account: account}
	upstream := &codexDiagnosticUpstreamStub{}
	svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream}
	c := codexDiagnosticContext()
	err := svc.TestAccountConnectionReadOnly(c, account.ID, "gpt-5.5", "", "", AccountTestOptions{CodexUserAgent: candidate})
	require.Error(t, err)
	require.Len(t, upstream.requests, 1)
	require.Contains(t, upstream.requests[0].Header.Get("User-Agent"), "(Mac OS 26.5.1; arm64) iTerm.app/3.6.11")
	require.NotContains(t, account.Credentials, "user_agent")
	// Every repository write is unimplemented and would panic if attempted.
	require.Equal(t, StatusActive, account.Status)
	require.Nil(t, account.RateLimitedAt)
}

func TestAccountDiagnosticRejectsMalformedUserAgentBeforeUpstream(t *testing.T) {
	account := &Account{ID: 91, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	repo := &codexDiagnosticRepoStub{account: account}
	upstream := &codexDiagnosticUpstreamStub{}
	svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream}
	c := codexDiagnosticContext()
	err := svc.TestAccountConnectionReadOnly(c, account.ID, "gpt-5.5", "", "", AccountTestOptions{
		CodexUserAgent: "codex-tui/0.154.0\r\nX-Injected: yes",
	})
	require.Error(t, err)
	require.Empty(t, upstream.requests)
	require.Empty(t, account.Credentials)
}
