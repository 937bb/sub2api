package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

func TestOAuthRetryLogCorrelationAndResponseReceived(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	downstream := context.WithValue(context.Background(), ctxkey.RequestID, "gateway-request-id")
	ctx := withOAuthRetryLogContext(context.Background(), downstream, 42)
	req, err := http.NewRequestWithContext(ctx, "POST", "https://upstream/responses", strings.NewReader("private-payload"))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer private-token")
	req.Header.Set("X-Request-ID", "different-upstream-id")
	calls := 0
	resp, err, exhausted := retryOAuthHTTP(ctx, req, OAuthRetrySettings{Enabled: true, MaxRetries: 1, StatusCodes: []int{502}}, func(r *http.Request) (*http.Response, error) {
		calls++
		body, readErr := io.ReadAll(r.Body)
		require.NoError(t, readErr)
		require.Equal(t, "private-payload", string(body))
		status := 502
		if calls == 2 {
			status = 200
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("untouched response"))}, nil
	})
	require.NoError(t, err)
	require.False(t, exhausted)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "untouched response", string(body))
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	require.Len(t, lines, 2)
	var first, second map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &first))
	require.NoError(t, json.Unmarshal([]byte(lines[1]), &second))
	require.Equal(t, "retry", first["event"])
	require.Equal(t, "response_received", second["event"])
	require.Equal(t, "gateway-request-id", first["request_id"])
	require.Equal(t, float64(42), first["account_id"])
	require.NotEmpty(t, first["retry_id"])
	require.Equal(t, first["retry_id"], second["retry_id"])
	require.NotContains(t, output.String(), "private-payload")
	require.NotContains(t, output.String(), "private-token")
}
