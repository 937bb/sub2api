package service

import (
	"net/http"
	"net/url"
	"sort"
	"strings"

	req "github.com/imroc/req/v3"
)

// codexWSHTTPTransport preserves the measured CLI upgrade header order/casing.
// req's HTTP/1 transport retains writable 101 response bodies, cancellation,
// proxy support and connection cleanup; only serialization is specialized here.
type codexWSHTTPTransport struct{ transport *req.Transport }

func newCodexWSHTTPTransport(base *http.Transport, proxyURL *url.URL) *codexWSHTTPTransport {
	t := req.NewTransport().EnableForceHTTP1()
	t.DialContext = base.DialContext
	t.DialTLSContext = base.DialTLSContext
	t.TLSClientConfig = base.TLSClientConfig
	t.TLSHandshakeTimeout = base.TLSHandshakeTimeout
	t.DisableCompression = true
	t.DisableKeepAlives = base.DisableKeepAlives
	t.MaxIdleConns = base.MaxIdleConns
	t.MaxIdleConnsPerHost = base.MaxIdleConnsPerHost
	t.MaxConnsPerHost = base.MaxConnsPerHost
	t.IdleConnTimeout = base.IdleConnTimeout
	t.ExpectContinueTimeout = base.ExpectContinueTimeout
	t.ResponseHeaderTimeout = base.ResponseHeaderTimeout
	t.MaxResponseHeaderBytes = base.MaxResponseHeaderBytes
	// HTTPS proxy tunneling is already performed by the measured TLS dialer.
	// Plain HTTP must still use the explicitly configured proxy.
	t.Proxy = func(r *http.Request) (*url.URL, error) {
		if r.URL.Scheme == "http" {
			return proxyURL, nil
		}
		return nil, nil
	}
	return &codexWSHTTPTransport{transport: t}
}

func (t *codexWSHTTPTransport) CloseIdleConnections() { t.transport.CloseIdleConnections() }

func (t *codexWSHTTPTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	clone := r.Clone(r.Context())
	clone.Header = make(http.Header, len(r.Header)+2)
	order := []string{"Host", "Connection", "Upgrade", "Sec-WebSocket-Version", "Sec-WebSocket-Key", "authorization", "user-agent", "originator", "openai-beta", "x-codex-turn-metadata", "x-codex-beta-features", "x-client-request-id", "session-id", "thread-id", "x-codex-window-id"}
	known := make(map[string]bool, len(order))
	for _, key := range order {
		known[strings.ToLower(key)] = true
	}
	keys := make([]string, 0, len(r.Header))
	for key := range r.Header {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	extra := []string{}
	for _, key := range keys {
		lower := strings.ToLower(key)
		if lower == req.HeaderOderKey || lower == req.PseudoHeaderOderKey {
			continue
		}
		name := lower
		switch lower {
		case "host", "content-length", "transfer-encoding", "trailer":
			name = http.CanonicalHeaderKey(key)
		case "connection":
			name = "Connection"
		case "upgrade":
			name = "Upgrade"
		case "sec-websocket-version":
			name = "Sec-WebSocket-Version"
		case "sec-websocket-key":
			name = "Sec-WebSocket-Key"
		}
		clone.Header[name] = append(clone.Header[name], r.Header[key]...)
		if !known[lower] && lower != "sec-websocket-extensions" {
			extra = append(extra, name)
			known[lower] = true
		}
	}
	sort.Strings(extra)
	order = append(order, extra...)
	order = append(order, "sec-websocket-extensions")
	// Suppress req's automatic title-case User-Agent; the lowercase field above
	// carries the original value at the measured position in the upgrade.
	clone.Header["User-Agent"] = []string{""}
	clone.Header[req.HeaderOderKey] = order
	return t.transport.RoundTrip(clone)
}
