package httpclient

import (
	"encoding/pem"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/stretchr/testify/require"
)

func testCodexUA() string {
	version := openai.CodexDesktopEngineVersion
	return openai.CodexDefaultOriginator + "/" + version + openai.CodexLocalCLIPlatform + " (codex_exec; " + version + ")"
}

func TestCodexClientTLSReuseIdentityAndCloseIdle(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor != 2 {
			t.Errorf("expected HTTP/2, got %s", r.Proto)
		}
		if r.Header.Get("Accept-Encoding") != "" {
			t.Error("unexpected automatic compression header")
		}
		_, _ = io.WriteString(w, r.RemoteAddr)
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	ca := filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600))
	t.Setenv("CODEX_CA_CERTIFICATE", ca)
	t.Setenv("SSL_CERT_FILE", "")
	original := validateResolvedIP
	validateResolvedIP = func(string) error { return nil }
	t.Cleanup(func() { validateResolvedIP = original })
	opts := Options{Timeout: 5 * time.Second, ValidateResolvedIP: true}
	client, err := GetCodexClient(opts, testCodexUA())
	require.NoError(t, err)
	defer client.CloseIdleConnections()
	send := func(token, device string) string {
		r, err := http.NewRequest(http.MethodGet, server.URL, nil)
		require.NoError(t, err)
		r.Header.Set("User-Agent", testCodexUA())
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("X-Codex-Installation-Id", device)
		response, err := client.Do(r)
		require.NoError(t, err)
		body, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
		return string(body)
	}
	first := send("one", "")
	require.Equal(t, first, send("one", ""))
	require.NotEqual(t, first, send("two", ""))
	device := send("one", "device")
	require.Equal(t, device, send("refreshed", "device"))
	client.CloseIdleConnections()
	require.NotEqual(t, device, send("refreshed", "device"), "close must pass through metrics, validation and identity wrappers")
	again, err := GetCodexClient(opts, testCodexUA())
	require.NoError(t, err)
	require.Same(t, client, again)
}

func TestCodexClientPlaintextRetainsExplicitProxy(t *testing.T) {
	t.Setenv("CODEX_CA_CERTIFICATE", "")
	t.Setenv("SSL_CERT_FILE", "")
	received := make(chan string, 1)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.URL.String()
		if r.Header.Get("Accept-Encoding") != "" {
			t.Error("unexpected automatic compression header")
		}
		_, _ = io.WriteString(w, "proxied")
	}))
	defer proxy.Close()
	client, err := GetCodexClient(Options{ProxyURL: proxy.URL, Timeout: 5 * time.Second}, testCodexUA())
	require.NoError(t, err)
	defer client.CloseIdleConnections()
	r, err := http.NewRequest(http.MethodGet, "http://fixture.invalid/models", nil)
	require.NoError(t, err)
	r.Header.Set("User-Agent", testCodexUA())
	response, err := client.Do(r)
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	require.Equal(t, "http://fixture.invalid/models", <-received)
}

func TestCodexClientPreservesValidationAndProfileSelection(t *testing.T) {
	t.Setenv("CODEX_CA_CERTIFICATE", "")
	t.Setenv("SSL_CERT_FILE", "")
	opts := Options{Timeout: 7 * time.Second, ValidateResolvedIP: true}
	native, err := GetCodexClient(opts, testCodexUA())
	require.NoError(t, err)
	defer native.CloseIdleConnections()
	t.Setenv("CODEX_CA_CERTIFICATE", "fixture-only-not-read-before-dial.pem")
	custom, err := GetCodexClient(opts, testCodexUA())
	require.NoError(t, err)
	defer custom.CloseIdleConnections()
	require.NotSame(t, native, custom)
	standard, err := GetClient(opts)
	require.NoError(t, err)
	fallback, err := GetCodexClient(opts, "other-client")
	require.NoError(t, err)
	require.Same(t, standard, fallback)
	require.NotSame(t, standard, native)
	denied := errors.New("fixture host rejected")
	original := validateResolvedIP
	validateResolvedIP = func(string) error { return denied }
	t.Cleanup(func() { validateResolvedIP = original })
	r, err := http.NewRequest(http.MethodGet, "https://blocked.invalid/models", nil)
	require.NoError(t, err)
	_, err = native.Do(r)
	require.ErrorIs(t, err, denied)
	_, err = GetCodexClient(Options{InsecureSkipVerify: true}, testCodexUA())
	require.Error(t, err)
	_, err = GetCodexClient(Options{ProxyURL: "ftp://fixture.invalid"}, testCodexUA())
	require.Error(t, err)
}

func TestCodexClientHTTPSRetainsConnectProxy(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "through-tunnel") }))
	defer upstream.Close()
	ca := filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: upstream.Certificate().Raw}), 0600))
	t.Setenv("CODEX_CA_CERTIFICATE", ca)
	t.Setenv("SSL_CERT_FILE", "")
	connected := make(chan string, 1)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || r.Host != strings.TrimPrefix(upstream.URL, "https://") {
			http.Error(w, "unexpected route", http.StatusBadRequest)
			return
		}
		remote, err := net.Dial("tcp", r.Host)
		if err != nil {
			t.Error(err)
			http.Error(w, "fixture connection failed", http.StatusBadGateway)
			return
		}
		defer func() { _ = remote.Close() }()
		peer, buffer, err := http.NewResponseController(w).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = peer.Close() }()
		if _, err = buffer.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
			t.Error(err)
			return
		}
		if err = buffer.Flush(); err != nil {
			t.Error(err)
			return
		}
		connected <- r.Host
		done := make(chan struct{})
		go func() { _, _ = io.Copy(remote, buffer); _ = remote.Close(); close(done) }()
		_, _ = io.Copy(peer, remote)
		_ = peer.Close()
		<-done
	}))
	defer proxy.Close()
	client, err := GetCodexClient(Options{ProxyURL: proxy.URL, Timeout: 5 * time.Second}, testCodexUA())
	require.NoError(t, err)
	defer client.CloseIdleConnections()
	r, err := http.NewRequest(http.MethodGet, upstream.URL, nil)
	require.NoError(t, err)
	r.Header.Set("User-Agent", testCodexUA())
	response, err := client.Do(r)
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, "through-tunnel", string(body))
	select {
	case host := <-connected:
		require.Equal(t, strings.TrimPrefix(upstream.URL, "https://"), host)
	default:
		t.Fatal("proxy was bypassed")
	}
}
