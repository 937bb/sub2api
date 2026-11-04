package httpclient

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyurl"
	"github.com/Wei-Shaw/sub2api/internal/pkg/servertiming"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

// GetCodexClient applies the measured transport to clients using the local CLI
// identity. Other identities retain the standard client, including its proxy
// and validation policy. Request headers and endpoint semantics are unchanged.
func GetCodexClient(opts Options, userAgent string) (*http.Client, error) {
	if !openai.IsLocalCLIUserAgent(userAgent) {
		return GetClient(opts)
	}
	profile := tlsfingerprint.CodexHTTPProfile()
	key := "codex:" + profile.Name + ":" + buildClientKey(opts)
	if cached, ok := sharedClients.Load(key); ok {
		if client, ok := cached.(*http.Client); ok {
			return client, nil
		}
	}
	plain, err := buildTransport(opts)
	if err != nil {
		return nil, err
	}
	_, proxy, err := proxyurl.Parse(opts.ProxyURL)
	if err != nil {
		return nil, err
	}
	plain.DisableCompression = true
	secure := plain.Clone()
	if err := tlsfingerprint.ConfigureTLS(secure, profile, proxy); err != nil {
		return nil, err
	}
	// ConfigureTLS owns CONNECT/SOCKS for HTTPS. Keep the original transport for
	// plaintext HTTP, including redirects, so an explicit proxy is never skipped.
	var rt http.RoundTripper = &codexSchemeTransport{
		plain:  tlsfingerprint.NewCodexHTTPTransport(plain, profile),
		secure: tlsfingerprint.NewCodexHTTPTransport(secure, profile),
	}
	if opts.ValidateResolvedIP && !opts.AllowPrivateHosts {
		rt = newValidatedTransport(rt)
	}
	client := &http.Client{Transport: servertiming.WrapRoundTripper(rt), Timeout: opts.Timeout}
	actual, loaded := sharedClients.LoadOrStore(key, client)
	if cached, ok := actual.(*http.Client); ok {
		if loaded {
			client.CloseIdleConnections()
		}
		return cached, nil
	}
	return client, nil
}

type codexSchemeTransport struct {
	plain  http.RoundTripper
	secure http.RoundTripper
}

func (t *codexSchemeTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Scheme == "https" {
		return t.secure.RoundTrip(r)
	}
	return t.plain.RoundTrip(r)
}

func (t *codexSchemeTransport) CloseIdleConnections() {
	for _, rt := range []http.RoundTripper{t.plain, t.secure} {
		if closer, ok := rt.(interface{ CloseIdleConnections() }); ok {
			closer.CloseIdleConnections()
		}
	}
}
