package repository

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestKiroTransportIsolationAndWireProtocol(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.ConnectionPoolIsolation = config.ConnectionPoolIsolationProxy
	cfg.Gateway.ResponseHeaderTimeout = 60
	s := NewHTTPUpstream(cfg).(*httpUpstreamService)
	first, err := s.getClientEntry("", 11, 1, service.HTTPUpstreamProfileKiro, false, false)
	require.NoError(t, err)
	second, err := s.getClientEntry("", 22, 1, service.HTTPUpstreamProfileKiro, false, false)
	require.NoError(t, err)
	require.NotSame(t, first, second)
	again, err := s.getClientEntry("", 11, 1, service.HTTPUpstreamProfileKiro, false, false)
	require.NoError(t, err)
	require.Same(t, first, again)
	ordinary, err := s.getClientEntry("", 11, 1, service.HTTPUpstreamProfileDefault, false, false)
	require.NoError(t, err)
	require.NotSame(t, first, ordinary)
	for _, entry := range []*upstreamClientEntry{first, second, ordinary} {
		t.Cleanup(entry.client.CloseIdleConnections)
	}
	transport := first.client.Transport.(*http.Transport)
	require.Zero(t, transport.ResponseHeaderTimeout)
	require.False(t, transport.ForceAttemptHTTP2)
	observed := make(chan *http.Request, 1)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { observed <- r; w.WriteHeader(200) }))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	transport.TLSClientConfig = &tls.Config{RootCAs: roots}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(service.WithHTTPUpstreamProfile(ctx, service.HTTPUpstreamProfileKiro), http.MethodGet, server.URL, nil)
	require.NoError(t, err)
	req.Close = true
	response, err := s.Do(req, "", 11, 1)
	require.NoError(t, err)
	_, err = io.Copy(io.Discard, response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	r := <-observed
	require.Equal(t, "HTTP/1.1", r.Proto)
	require.True(t, r.Close)
	require.NotEqual(t, "h2", r.TLS.NegotiatedProtocol)
}
