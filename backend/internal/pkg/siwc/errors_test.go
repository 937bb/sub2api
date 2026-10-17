package siwc

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRefreshErrorCodesAreTypedAndDoNotExposeDescription(t *testing.T) {
	for _, code := range []string{"invalid_grant", "invalid_refresh_token", "token_expired", "refresh_token_expired", "refresh_token_invalidated", "refresh_token_reused", "invalid_client", "invalid_scope"} {
		t.Run(code, func(t *testing.T) {
			client := NewClient(&http.Client{Transport: errorRoundTripper(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader(fmt.Sprintf(`{"error":%q,"error_description":"secret credential"}`, code)))}, nil
			})})
			err := client.json(context.Background(), TokenURL, url.Values{"grant_type": {"refresh_token"}}, "", &struct{}{})
			var typed *AuthorizationError
			require.ErrorAs(t, err, &typed)
			require.Equal(t, code, typed.Code)
			require.NotContains(t, err.Error(), "secret")
			require.Equal(t, code != "invalid_client" && code != "invalid_scope", IsTerminalRefreshError(fmt.Errorf("refresh: %w", err)))
		})
	}
	require.False(t, IsTerminalRefreshError(fmt.Errorf("network returned invalid_grant in unrelated text")))
}

type errorRoundTripper func(*http.Request) (*http.Response, error)

func (f errorRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
