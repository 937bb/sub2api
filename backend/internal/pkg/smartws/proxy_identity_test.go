package smartws

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

// The IPv6 egress proxy uses distinct Basic credentials at a shared endpoint.
// Changing the assigned lease must therefore change WS reuse identity too.
func TestProxyCredentialsIsolateEgressAndFailuresNeverDialDirect(t *testing.T) {
	target, _, generated, _ := successServer(t)
	var first, second atomic.Int64
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			w.WriteHeader(405)
			return
		}
		auth := &http.Request{Header: http.Header{"Authorization": r.Header.Values("Proxy-Authorization")}}
		user, _, ok := auth.BasicAuth()
		if !ok {
			w.WriteHeader(407)
			return
		}
		switch user {
		case "lease-a":
			first.Add(1)
		case "lease-b":
			second.Add(1)
		default:
			w.WriteHeader(502)
			return
		}
		upstream, err := net.DialTimeout("tcp", r.Host, time.Second)
		if err != nil {
			w.WriteHeader(502)
			return
		}
		client, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			upstream.Close()
			return
		}
		defer client.Close()
		defer upstream.Close()
		_, _ = io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n")
		go func() { _, _ = io.Copy(upstream, client); upstream.Close() }()
		_, _ = io.Copy(client, upstream)
	}))
	t.Cleanup(proxy.Close)
	p := testPool(t, 8)
	proxyURL, _ := url.Parse(proxy.URL)
	for _, user := range []string{"lease-a", "lease-b", "lease-a"} {
		proxyURL.User = url.UserPassword(user, "secret")
		consume(t, p, request(t, target.URL, validBody, "same-session"), proxyURL.String(), "same-tenant")
	}
	if first.Load() != 1 || second.Load() != 1 || generated.Load() != 3 {
		t.Fatalf("egress reuse crossed credentials: a=%d b=%d generated=%d", first.Load(), second.Load(), generated.Load())
	}
	proxyURL.User = url.UserPassword("broken-lease", "secret")
	r := request(t, target.URL, validBody, "same-session")
	resp, handled, err := p.Try(r, proxyURL.String(), "same-tenant")
	if resp != nil || handled || err != nil || generated.Load() != 3 {
		t.Fatalf("proxy failure must return untouched HTTP fallback without direct dispatch: handled=%v err=%v generated=%d", handled, err, generated.Load())
	}
	// Core's fallback receives this original body and the same proxy URL.
	body, _ := io.ReadAll(r.Body)
	if string(body) != validBody {
		t.Fatal("fallback body was consumed")
	}
}
