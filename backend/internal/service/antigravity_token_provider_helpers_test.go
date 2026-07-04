package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type antigravityTokenProviderAccountRepoStub struct {
	AccountRepository
	account                  *Account
	updateCredentialsCalls   int
	updateCredentialsPayload []map[string]any
	updateCalls              int
}

func (r *antigravityTokenProviderAccountRepoStub) GetByID(_ context.Context, id int64) (*Account, error) {
	if r.account == nil || r.account.ID != id {
		return nil, ErrAccountNotFound
	}
	return r.account, nil
}

func (r *antigravityTokenProviderAccountRepoStub) UpdateCredentials(_ context.Context, id int64, credentials map[string]any) error {
	r.updateCredentialsCalls++
	r.updateCredentialsPayload = append(r.updateCredentialsPayload, cloneCredentials(credentials))
	if r.account != nil && r.account.ID == id {
		r.account.Credentials = cloneCredentials(credentials)
	}
	return nil
}

func (r *antigravityTokenProviderAccountRepoStub) Update(_ context.Context, account *Account) error {
	r.updateCalls++
	r.account = account
	return nil
}

type antigravityRefreshHTTPStub struct {
	requestPaths        []string
	loadCodeAssistCalls int
	onboardUserCalls    int
	projectID           string
}

func (s *antigravityRefreshHTTPStub) RoundTrip(req *http.Request) (*http.Response, error) {
	s.requestPaths = append(s.requestPaths, req.URL.Path)
	switch req.URL.Path {
	case "/token":
		return newAntigravityRefreshHTTPResponse(http.StatusOK, `{"access_token":"refreshed-token","refresh_token":"refreshed-refresh","expires_in":3600,"token_type":"Bearer"}`), nil
	case "/v1internal:loadCodeAssist":
		s.loadCodeAssistCalls++
		projectID := s.projectID
		if projectID == "" {
			projectID = "backfilled-project"
		}
		return newAntigravityRefreshHTTPResponse(http.StatusOK, `{"cloudaicompanionProject":"`+projectID+`"}`), nil
	case "/v1internal:onboardUser":
		s.onboardUserCalls++
		return newAntigravityRefreshHTTPResponse(http.StatusOK, `{"done":true,"response":{"cloudaicompanionProject":"onboarded-project"}}`), nil
	default:
		return newAntigravityRefreshHTTPResponse(http.StatusNotFound, `{}`), nil
	}
}

func newAntigravityRefreshHTTPResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func withAntigravityRefreshHTTPStub(t *testing.T, projectID string) *antigravityRefreshHTTPStub {
	t.Helper()
	stub := &antigravityRefreshHTTPStub{projectID: projectID}
	oldTransport := http.DefaultTransport
	http.DefaultTransport = stub
	t.Cleanup(func() {
		http.DefaultTransport = oldTransport
	})
	return stub
}
