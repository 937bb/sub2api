package tlsfingerprint

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// CodexLocalHTTPProfile reproduces the default macOS CLI ClientHello measured
// without custom CA environment variables. Native HTTP did not offer ALPN.
func CodexLocalHTTPProfile() *Profile {
	return &Profile{
		Name: "codex-macos-native-http-20261103", MinVersion: 0x0303, MaxVersion: 0x0303,
		CipherSuites: []uint16{255, 49196, 49195, 49188, 49187, 49162, 49161, 49160, 49200, 49199, 49192, 49191, 49172, 49171, 49170, 157, 156, 61, 60, 53, 47, 10},
		Curves:       []uint16{23, 24, 25}, PointFormats: []uint16{0},
		SignatureAlgorithms: []uint16{1025, 513, 1281, 1537, 1027, 515, 1283, 1539},
		Extensions:          []uint16{0, 10, 11, 13, 5, 18, 23},
	}
}

// CodexLocalWSProfile is the bundled CLI's Rustls WebSocket ClientHello.
// Extension order varies on each connection; WS does not advertise ALPN.
func CodexLocalWSProfile() *Profile {
	return &Profile{
		Name: "codex-rustls-ws-20261103", MinVersion: 0x0303, MaxVersion: 0x0304, ShuffleExtensions: true,
		CipherSuites: []uint16{4866, 4865, 4867, 49196, 49195, 52393, 49200, 49199, 52392, 255},
		Curves:       []uint16{4588, 29, 23, 24}, PointFormats: []uint16{0},
		SignatureAlgorithms: []uint16{1283, 1027, 1539, 2055, 2054, 2053, 2052, 1537, 1281, 1025, 2308, 2309, 2310},
		SupportedVersions:   []uint16{772, 771}, KeyShareGroups: []uint16{4588, 29}, PSKModes: []uint16{1},
		Extensions: []uint16{0, 5, 10, 11, 13, 23, 35, 43, 45, 51},
	}
}

// CodexCustomCAHTTPProfile matches the CLI's Rustls HTTP backend selected by
// either custom-CA environment variable. The server negotiates H2 or HTTP/1.
func CodexCustomCAHTTPProfile() *Profile {
	p := CodexLocalWSProfile()
	p.Name = "codex-rustls-http-custom-ca-20261103"
	p.ALPNProtocols = []string{"h2", "http/1.1"}
	p.Extensions = append(p.Extensions, 16)
	return p
}

// CodexHTTPProfile selects the independently measured native or custom-CA path.
func CodexHTTPProfile() *Profile {
	if strings.TrimSpace(os.Getenv("CODEX_CA_CERTIFICATE")) != "" || strings.TrimSpace(os.Getenv("SSL_CERT_FILE")) != "" {
		return CodexCustomCAHTTPProfile()
	}
	return CodexLocalHTTPProfile()
}

// CacheKey includes every field that can change the emitted ClientHello.
func (p *Profile) CacheKey() string {
	encoded, _ := json.Marshal(p)
	return fmt.Sprintf("%x", sha256.Sum256(encoded))
}

// ConfigureTLS configures HTTPS dialing only. Callers must route plaintext
// HTTP through a separate transport so explicit proxies cannot be bypassed.
func ConfigureTLS(t *http.Transport, p *Profile, proxyURL *url.URL) error {
	t.Proxy = nil
	t.ForceAttemptHTTP2 = false
	switch {
	case proxyURL == nil:
		t.DialTLSContext = NewDialer(p, nil).DialTLSContext
	case proxyURL.Scheme == "http" || proxyURL.Scheme == "https":
		t.DialTLSContext = NewHTTPProxyDialer(p, proxyURL).DialTLSContext
	case proxyURL.Scheme == "socks5" || proxyURL.Scheme == "socks5h":
		t.DialTLSContext = NewSOCKS5ProxyDialer(p, proxyURL).DialTLSContext
	default:
		return fmt.Errorf("unsupported fingerprint proxy scheme %q", proxyURL.Scheme)
	}
	return nil
}
