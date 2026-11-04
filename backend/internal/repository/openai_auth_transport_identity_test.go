package repository

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAIAuthTransportIsolatesCredentialsOnWire(t *testing.T) {
	sharedReqClients = sync.Map{}
	addresses := make(chan string, 8)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		addresses <- r.RemoteAddr
		if r.Header.Get("Version") != "" || r.Header.Get("X-Codex-Installation-Id") != "" {
			t.Error("auth request acquired inference-only headers")
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Form.Get("refresh_token") == "" && r.Form.Get("code") == "" {
			t.Error("missing fixture credential")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"fixture-access","refresh_token":"fixture-refresh","token_type":"Bearer","expires_in":3600}`))
	}))
	defer server.Close()
	ca := filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600))
	t.Setenv("CODEX_CA_CERTIFICATE", ca)
	t.Setenv("SSL_CERT_FILE", "")
	svc := &openaiOAuthService{tokenURL: server.URL}
	refresh := func(token string) string {
		_, err := svc.RefreshToken(context.Background(), token, "")
		require.NoError(t, err)
		return <-addresses
	}
	first := refresh("fixture-one")
	require.Equal(t, first, refresh("fixture-one"))
	require.NotEqual(t, first, refresh("fixture-two"))
	require.Equal(t, first, refresh("fixture-one"))
	_, err := svc.ExchangeCode(context.Background(), "fixture-code-one", "fixture-verifier", "http://localhost/callback", "", "")
	require.NoError(t, err)
	exchange := <-addresses
	require.NotEqual(t, first, exchange)
	_, err = svc.ExchangeCode(context.Background(), "fixture-code-two", "fixture-verifier", "http://localhost/callback", "", "")
	require.NoError(t, err)
	require.NotEqual(t, exchange, <-addresses)
}
