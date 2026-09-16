package service

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

type protectionTransportRecorder struct {
	normalCalls int
	tlsCalls    int
	profile     *tlsfingerprint.Profile
	concurrency int
}

func (r *protectionTransportRecorder) response() *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok"))}
}

func (r *protectionTransportRecorder) Do(_ *http.Request, _ string, _ int64, concurrency int) (*http.Response, error) {
	r.normalCalls++
	r.concurrency = concurrency
	return r.response(), nil
}

func (r *protectionTransportRecorder) DoWithTLS(_ *http.Request, _ string, _ int64, concurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	r.tlsCalls++
	r.profile = profile
	r.concurrency = concurrency
	return r.response(), nil
}

func TestProtectedOpenAIHTTPTransportUsesConfiguredStrategy(t *testing.T) {
	for _, tc := range []struct {
		name        string
		account     *Account
		tlsEnabled  bool
		wantNormal  int
		wantTLS     int
		wantProfile string
	}{
		{name: "mode1 v3 keeps standard transport", account: mode1IntegrityTestAccount(), tlsEnabled: true, wantNormal: 1},
		{name: "legacy uses explicit nodejs24 profile", account: legacyProtectionTransportAccount(), tlsEnabled: true, wantTLS: 1, wantProfile: "Node.js 24 compatibility"},
		{name: "global kill switch keeps legacy on standard transport", account: legacyProtectionTransportAccount(), wantNormal: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := &protectionTransportRecorder{}
			cfg := &config.Config{}
			cfg.Gateway.TLSFingerprint.Enabled = tc.tlsEnabled
			gateway := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
			req, err := http.NewRequest(http.MethodPost, "https://chatgpt.com/backend-api/codex/responses", strings.NewReader(`{}`))
			require.NoError(t, err)

			resp, err := gateway.doOpenAIUpstream(req, "", tc.account)

			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			require.Equal(t, tc.wantNormal, upstream.normalCalls)
			require.Equal(t, tc.wantTLS, upstream.tlsCalls)
			require.Equal(t, 100, upstream.concurrency)
			if tc.wantProfile != "" {
				require.NotNil(t, upstream.profile)
				require.Equal(t, tc.wantProfile, upstream.profile.Name)
			}
		})
	}
}

func legacyProtectionTransportAccount() *Account {
	account := mode1IntegrityTestAccount()
	account.Extra[codexFingerprintModeExtraKey] = "session"
	account.Extra["enable_tls_fingerprint"] = true
	account.Extra["tls_fingerprint_builtin"] = "nodejs24"
	account.Extra[AntiDegradeMarkerExtraKey] = map[string]any{
		"enabled": true, "mode": string(AntiDegradeModeLegacy), "max_concurrency": 100,
	}
	return account
}
