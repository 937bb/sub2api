//go:build unit

package service

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestAccountTestService_AnthropicAPIKeyAuthScheme(t *testing.T) {
	tests := []struct {
		name       string
		scheme     any
		wantAPIKey string
		wantAuth   string
	}{
		{
			name:     "bearer",
			scheme:   AnthropicAPIKeyAuthSchemeAuthorizationBearer,
			wantAuth: "Bearer anthropic-test-key",
		},
		{
			name:       "default",
			wantAPIKey: "anthropic-test-key",
		},
		{
			name:       "invalid scheme defaults to x-api-key",
			scheme:     "bearer",
			wantAPIKey: "anthropic-test-key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extra := map[string]any{}
			if tt.scheme != nil {
				extra[anthropicAPIKeyAuthSchemeExtraKey] = tt.scheme
			}
			account := &Account{
				ID:          1,
				Platform:    PlatformAnthropic,
				Type:        AccountTypeAPIKey,
				Concurrency: 1,
				Credentials: map[string]any{"api_key": "anthropic-test-key"},
				Extra:       extra,
			}
			upstream := &queuedHTTPUpstream{responses: []*http.Response{{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("data: {\"type\":\"message_stop\"}\n\n")),
			}}}
			svc := &AccountTestService{httpUpstream: upstream, cfg: &config.Config{}}
			c, _ := newTestContext()

			err := svc.testClaudeAccountConnection(c, account, "claude-test-model")
			require.NoError(t, err)
			require.Len(t, upstream.requests, 1)

			req := upstream.requests[0]
			require.Equal(t, tt.wantAPIKey, getHeaderRaw(req.Header, "x-api-key"))
			require.Equal(t, tt.wantAuth, getHeaderRaw(req.Header, "authorization"))
			require.NotEqual(t, "", req.Header.Get("anthropic-beta"))
		})
	}
}
