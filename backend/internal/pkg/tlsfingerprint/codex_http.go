package tlsfingerprint

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"

	req "github.com/imroc/req/v3"
	"github.com/imroc/req/v3/http2"
	utls "github.com/refraction-networking/utls"
)

// NewCodexHTTPTransport keeps native HTTP/1 and custom-CA Rustls negotiation
// separate. The supplied dialer retains direct, CONNECT and SOCKS routing.
func NewCodexHTTPTransport(base *http.Transport, profile *Profile) http.RoundTripper {
	if profile.Name != CodexCustomCAHTTPProfile().Name {
		return NewOrderedHTTPTransport(base)
	}
	t := NewOrderedHTTPTransport(base).Transport.DisableForceHttpVersion()
	t.DisableCompression = true
	t.LowercaseHTTP1Headers = true
	var settings []http2.Setting
	for _, setting := range CodexHTTP2Settings() {
		settings = append(settings, http2.Setting{ID: http2.SettingID(setting[0]), Val: setting[1]})
	}
	t.SetHTTP2SettingsFrame(settings...)
	t.SetHTTP2ConnectionFlow(CodexHTTP2ConnectionWindowUpdate)
	t.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if base.DialTLSContext == nil {
			return nil, fmt.Errorf("Codex custom-CA transport requires a TLS dialer")
		}
		conn, err := base.DialTLSContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		if c, ok := conn.(*utls.UConn); ok {
			return &codexTLSConn{c}, nil
		}
		return conn, nil
	}
	return &codexRustlsHTTPTransport{t}
}

// Adapt uTLS's state to req's standard TLS interface so ALPN actually selects H2.
type codexTLSConn struct{ *utls.UConn }

func (c *codexTLSConn) ConnectionState() tls.ConnectionState {
	s := c.UConn.ConnectionState()
	return tls.ConnectionState{Version: s.Version, HandshakeComplete: s.HandshakeComplete,
		DidResume: s.DidResume, CipherSuite: s.CipherSuite, NegotiatedProtocol: s.NegotiatedProtocol,
		NegotiatedProtocolIsMutual: s.NegotiatedProtocolIsMutual, ServerName: s.ServerName,
		PeerCertificates: s.PeerCertificates, VerifiedChains: s.VerifiedChains,
		SignedCertificateTimestamps: s.SignedCertificateTimestamps, OCSPResponse: s.OCSPResponse,
		TLSUnique: s.TLSUnique}
}

type codexRustlsHTTPTransport struct{ transport *req.Transport }

func (t *codexRustlsHTTPTransport) CloseIdleConnections() { t.transport.CloseIdleConnections() }
func (t *codexRustlsHTTPTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	clone := r.Clone(r.Context())
	clone.Header = make(http.Header, len(r.Header)+3)
	order := CodexRustlsHTTPHeaderOrder()
	known := make(map[string]bool, len(order))
	for _, key := range order {
		known[key] = true
	}
	var keys, extra []string
	for key := range r.Header {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		lower := strings.ToLower(key)
		if lower == req.HeaderOderKey || lower == req.PseudoHeaderOderKey {
			continue
		}
		// The serializer owns framing and host fields.
		if lower == "host" || lower == "content-length" {
			continue
		}
		clone.Header[lower] = append(clone.Header[lower], r.Header[key]...)
		if !known[lower] {
			extra = append(extra, lower)
			known[lower] = true
		}
	}
	sort.Strings(extra)
	order = append(order, extra...)
	order = append(order, "host", "content-length", "transfer-encoding", "trailer", "connection", "accept-encoding")
	clone.Header["User-Agent"] = []string{""}
	clone.Header[req.HeaderOderKey] = order
	clone.Header[req.PseudoHeaderOderKey] = CodexHTTP2PseudoHeaderOrder()
	return t.transport.RoundTrip(clone)
}

// These functions are shared by the serializer and administrator diagnostics.
const CodexHTTP2ConnectionWindowUpdate = 5177345

func CodexHTTP2Settings() [][2]uint32 {
	return [][2]uint32{{2, 0}, {4, 2097152}, {5, 16384}, {6, 16384}}
}
func CodexHTTP2PseudoHeaderOrder() []string {
	return []string{":method", ":scheme", ":authority", ":path"}
}
func CodexRustlsHTTPHeaderOrder() []string {
	return []string{"x-codex-beta-features", "x-codex-window-id", "x-codex-turn-metadata", "x-client-request-id", "session-id", "thread-id", "accept", "content-type", "authorization", "originator", "user-agent"}
}
