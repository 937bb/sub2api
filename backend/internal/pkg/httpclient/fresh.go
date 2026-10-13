package httpclient

import (
	"context"
	"io"
	"net/http"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/pkg/servertiming"
)

type freshConnectionKey struct{}

// NewFreshClient creates an uncached transport with the same proxy and TLS
// settings as GetClient. It also works when the cached client is instrumented.
func NewFreshClient(opts Options) (*http.Client, error) {
	transport, err := buildTransport(opts)
	if err != nil {
		return nil, err
	}
	transport.DisableKeepAlives = true
	var rt http.RoundTripper = FreshTransport{Base: transport}
	if opts.ValidateResolvedIP && !opts.AllowPrivateHosts {
		rt = newValidatedTransport(rt)
	}
	return &http.Client{Transport: servertiming.WrapRoundTripper(rt), Timeout: opts.Timeout}, nil
}

// WithFreshConnection requests a private transport for this attempt only.
func WithFreshConnection(ctx context.Context) context.Context {
	return context.WithValue(ctx, freshConnectionKey{}, true)
}

func RequiresFreshConnection(ctx context.Context) bool {
	return ctx != nil && ctx.Value(freshConnectionKey{}) == true
}

// RoundTripFresh preserves proxy, TLS and protocol settings without sharing
// idle HTTP/1 connections or active HTTP/2 connections with the normal pool.
// Other transport implementations must handle the context marker themselves.
func RoundTripFresh(base http.RoundTripper, req *http.Request) (*http.Response, error) {
	if base == nil {
		base = http.DefaultTransport
	}
	transport, ok := base.(*http.Transport)
	if !RequiresFreshConnection(req.Context()) || !ok {
		return base.RoundTrip(req)
	}
	fresh := transport.Clone()
	// Explicit x/net/http2 hooks close over the original HTTP/2 pool. Cloning
	// that map alone can still send the retry on the failed multiplexed socket.
	if fresh.TLSNextProto["h2"] != nil {
		fresh.TLSNextProto = nil
		fresh.ForceAttemptHTTP2 = true
	}
	fresh.DisableKeepAlives = true
	resp, err := fresh.RoundTrip(req)
	if err != nil {
		fresh.CloseIdleConnections()
		return resp, err
	}
	resp.Body = &freshConnectionBody{ReadCloser: resp.Body, cleanup: fresh.CloseIdleConnections}
	return resp, nil
}

type freshConnectionBody struct {
	io.ReadCloser
	cleanup func()
	once    sync.Once
	err     error
}

func (b *freshConnectionBody) Close() error {
	b.once.Do(func() {
		b.err = b.ReadCloser.Close()
		b.cleanup()
	})
	return b.err
}

type FreshTransport struct{ Base http.RoundTripper }

func (t FreshTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return RoundTripFresh(t.Base, req)
}
