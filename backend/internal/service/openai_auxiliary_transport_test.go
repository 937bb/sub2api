package service

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

func TestCodexAuxiliaryEndpointsSendMeasuredTLSAndVerifyCertificate(t *testing.T) {
	t.Setenv("CODEX_CA_CERTIFICATE", "")
	t.Setenv("SSL_CERT_FILE", "")
	SetCodexCanonicalUserAgentResolver(nil)
	t.Cleanup(func() { SetCodexCanonicalUserAgentResolver(nil) })
	for _, endpoint := range []string{"pat", "models"} {
		t.Run(endpoint, func(t *testing.T) {
			hellos := make(chan []uint16, 4)
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Error("untrusted endpoint must not receive an application request")
			}))
			server.TLS = &tls.Config{GetConfigForClient: func(h *tls.ClientHelloInfo) (*tls.Config, error) {
				hellos <- append([]uint16(nil), h.CipherSuites...)
				return nil, nil
			}}
			server.StartTLS()
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var err error
			if endpoint == "pat" {
				old := openAICodexPATWhoamiURL
				openAICodexPATWhoamiURL = server.URL
				defer func() { openAICodexPATWhoamiURL = old }()
				_, err = (&OpenAIOAuthService{}).ValidateCodexPersonalAccessToken(ctx, "at-fixture-only", "")
			} else {
				headers := make(http.Header)
				headers.Set("User-Agent", CodexCanonicalUserAgent())
				headers.Set("Authorization", "Bearer fixture-only")
				_, err = (&OpenAIGatewayService{}).fetchOpenAIModelsUpstream(ctx, openAIModelsRequest{url: server.URL, headers: headers}, "")
			}
			require.Error(t, err)
			select {
			case suites := <-hellos:
				require.Equal(t, tlsfingerprint.CodexLocalHTTPProfile().CipherSuites, suites)
			case <-ctx.Done():
				t.Fatal("no ClientHello captured")
			}
		})
	}
}
