package repository

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

func TestLocalCLIActualHTTPSPathsSelectMeasuredTLS(t *testing.T) {
	for _, path := range []string{"inference", "auth", "quota"} {
		t.Run(path, func(t *testing.T) {
			hellos := make(chan []uint16, 4)
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
			server.TLS = &tls.Config{GetConfigForClient: func(h *tls.ClientHelloInfo) (*tls.Config, error) {
				hellos <- append([]uint16(nil), h.CipherSuites...)
				return nil, nil
			}}
			server.StartTLS()
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			ua := "codex_exec/0.166.0-alpha.5.1" + openai.CodexLocalCLIPlatform + " (codex_exec; 0.166.0-alpha.5.1)"
			if path == "inference" {
				request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
				require.NoError(t, err)
				request.Header.Set("User-Agent", ua)
				_, err = NewHTTPUpstream(nil).Do(request, "", 9871, 1)
				require.Error(t, err, "untrusted local certificate must be rejected")
			} else {
				client, err := getSharedReqClient(reqClientOptions{Timeout: 5 * time.Second, Impersonate: path == "quota"})
				require.NoError(t, err)
				_, err = client.R().SetContext(ctx).SetHeader("User-Agent", ua).Get(server.URL)
				require.Error(t, err, "untrusted local certificate must be rejected")
			}
			select {
			case hello := <-hellos:
				require.Equal(t, tlsfingerprint.CodexLocalHTTPProfile().CipherSuites, hello)
			case <-ctx.Done():
				t.Fatal("measured TLS profile was not sent")
			}
		})
	}
}
