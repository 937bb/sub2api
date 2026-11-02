package repository

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestHTTPUpstreamTracePublicEntrypoints(t *testing.T) {
	for _, withTLS := range []bool{false, true} {
		t.Run(fmt.Sprintf("tls=%v", withTLS), func(t *testing.T) {
			core, events := observer.New(zap.InfoLevel)
			ctx := logger.IntoContext(context.Background(), zap.New(core))
			ctx = context.WithValue(ctx, ctxkey.ClientRequestID, "correlated-request")
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Oneapi-Request-Id", "supplier-request")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = io.WriteString(w, "retry later")
			}))
			defer srv.Close()
			upstream, send := lifecycleClient(t, srv, withTLS)
			req, err := http.NewRequestWithContext(ctx, "POST", srv.URL, nil)
			require.NoError(t, err)
			resp, err := send(req)
			require.NoError(t, err)
			require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
			headers := events.FilterMessage("upstream_attempt_headers").All()
			require.Len(t, headers, 1)
			require.Equal(t, "supplier-request", headers[0].ContextMap()["upstream_request_id"])
			require.Equal(t, "client:correlated-request", headers[0].ContextMap()["usage_request_id"])
			require.Empty(t, events.FilterMessage("upstream_attempt_finished").All())
			require.NoError(t, resp.Body.Close())
			require.Len(t, events.FilterMessage("upstream_attempt_finished").All(), 1)
			requireNoUpstreamInFlight(t, upstream)
		})
	}
}
