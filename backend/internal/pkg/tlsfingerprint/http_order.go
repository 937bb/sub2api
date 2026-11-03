package tlsfingerprint

import (
	"net/http"
	"sort"
	"strings"

	req "github.com/imroc/req/v3"
)

// OrderedHTTPTransport retains the configured TLS dialer and pool policy while
// serializing native CLI HTTP/1 headers in their measured order.
type OrderedHTTPTransport struct{ Transport *req.Transport }

func NewOrderedHTTPTransport(base *http.Transport) *OrderedHTTPTransport {
	t := req.NewTransport().EnableForceHTTP1()
	t.Proxy = base.Proxy
	t.DialContext = base.DialContext
	t.DialTLSContext = base.DialTLSContext
	t.TLSClientConfig = base.TLSClientConfig
	t.TLSHandshakeTimeout = base.TLSHandshakeTimeout
	t.DisableCompression = base.DisableCompression
	t.DisableKeepAlives = base.DisableKeepAlives
	t.MaxIdleConns = base.MaxIdleConns
	t.MaxIdleConnsPerHost = base.MaxIdleConnsPerHost
	t.MaxConnsPerHost = base.MaxConnsPerHost
	t.IdleConnTimeout = base.IdleConnTimeout
	t.ResponseHeaderTimeout = base.ResponseHeaderTimeout
	t.ExpectContinueTimeout = base.ExpectContinueTimeout
	t.MaxResponseHeaderBytes = base.MaxResponseHeaderBytes
	t.ReadBufferSize = base.ReadBufferSize
	t.WriteBufferSize = base.WriteBufferSize
	return &OrderedHTTPTransport{Transport: t}
}

func (t *OrderedHTTPTransport) CloseIdleConnections() { t.Transport.CloseIdleConnections() }

func (t *OrderedHTTPTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	clone := r.Clone(r.Context())
	clone.Header = make(http.Header, len(r.Header)+1)
	keys := make([]string, 0, len(r.Header))
	for key := range r.Header {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if strings.EqualFold(key, req.HeaderOderKey) || strings.EqualFold(key, req.PseudoHeaderOderKey) {
			continue
		}
		canonical := http.CanonicalHeaderKey(key)
		clone.Header[canonical] = append(clone.Header[canonical], r.Header[key]...)
	}
	order := []string{"Host", "User-Agent", "Content-Length"}
	remaining := []string{}
	for key := range clone.Header {
		if key != "Host" && key != "User-Agent" && key != "Content-Length" {
			remaining = append(remaining, key)
		}
	}
	sort.Strings(remaining)
	order = append(order, remaining...)
	// Include fields synthesized by the transport so ordering remains total.
	for _, key := range []string{"Transfer-Encoding", "Trailer", "Connection", "Accept-Encoding"} {
		if _, exists := clone.Header[key]; !exists {
			order = append(order, key)
		}
	}
	clone.Header[req.HeaderOderKey] = order
	return t.Transport.RoundTrip(clone)
}
