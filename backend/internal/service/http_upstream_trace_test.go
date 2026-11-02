package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func traceRequest(t *testing.T) (*http.Request, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zap.InfoLevel)
	ctx := logger.IntoContext(context.Background(), zap.New(core))
	ctx = context.WithValue(ctx, ctxkey.ClientRequestID, "client-test")
	ctx = context.WithValue(ctx, ctxkey.RequestID, "local-test")
	req, err := http.NewRequestWithContext(ctx, "POST", "https://example.com/private-path?api_key=secret", strings.NewReader("private prompt"))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer secret")
	return req, logs
}

func TestHTTPUpstreamTraceRetriesKeepDistinctAttemptsAndIDs(t *testing.T) {
	req, logs := traceRequest(t)
	for _, id := range []string{"rate-limited", "successful"} {
		finish := BeginHTTPUpstreamTrace(req, 64232)
		h := http.Header{}
		h.Set("X-Oneapi-Request-Id", id)
		resp := &http.Response{StatusCode: 200, Header: h, Body: io.NopCloser(strings.NewReader("response"))}
		if id == "rate-limited" {
			resp.StatusCode = 429
		}
		finish(resp, nil)
		// The header event exists before the body is consumed or closed.
		require.Equal(t, id, logs.FilterMessage("upstream_attempt_headers").All()[len(logs.FilterMessage("upstream_attempt_headers").All())-1].ContextMap()["upstream_request_id"])
		_, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
	}
	started := logs.FilterMessage("upstream_attempt_started").All()
	require.Len(t, started, 2)
	require.NotEqual(t, started[0].ContextMap()["attempt_id"], started[1].ContextMap()["attempt_id"])
	finished := logs.FilterMessage("upstream_attempt_finished").All()
	require.Len(t, finished, 2)
	for _, e := range logs.All() {
		require.Equal(t, "client:client-test", e.ContextMap()["usage_request_id"])
		require.Equal(t, int64(64232), e.ContextMap()["account_id"])
		for _, value := range e.ContextMap() {
			if s, ok := value.(string); ok {
				require.NotContains(t, s, "secret")
				require.NotContains(t, s, "private")
			}
		}
	}
	for _, e := range finished {
		require.Equal(t, "eof", e.ContextMap()["outcome"])
	}
}

func TestHTTPUpstreamTraceTransportFailureAndEarlyClose(t *testing.T) {
	req, logs := traceRequest(t)
	BeginHTTPUpstreamTrace(req, 1)(nil, errors.New("https://secret:password@example.com failed"))
	require.Equal(t, "transport_error", logs.FilterMessage("upstream_attempt_finished").All()[0].ContextMap()["outcome"])
	body := &traceTestBody{Reader: strings.NewReader("unread")}
	resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: body}
	BeginHTTPUpstreamTrace(req, 2)(resp, nil)
	require.NoError(t, resp.Body.Close())
	require.True(t, body.closed)
	require.Equal(t, "closed_early", logs.FilterMessage("upstream_attempt_finished").All()[1].ContextMap()["outcome"])
}

type traceTestBody struct {
	io.Reader
	closed bool
}

func (b *traceTestBody) Close() error { b.closed = true; return nil }

func TestHTTPUpstreamTraceBodyReadErrorPreservesError(t *testing.T) {
	req, logs := traceRequest(t)
	resp := &http.Response{StatusCode: 200, Body: &traceTestBody{Reader: failingTraceReader{}}}
	BeginHTTPUpstreamTrace(req, 1)(resp, nil)
	_, err := io.ReadAll(resp.Body)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	require.NoError(t, resp.Body.Close())
	entries := logs.FilterMessage("upstream_attempt_finished").All()
	require.Len(t, entries, 1)
	require.Equal(t, "body_read_error", entries[0].ContextMap()["outcome"])
}

type failingTraceReader struct{}

func (failingTraceReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
