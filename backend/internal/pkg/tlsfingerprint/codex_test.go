package tlsfingerprint

import (
	"context"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"fmt"
	coderws "github.com/coder/websocket"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	utls "github.com/refraction-networking/utls"
)

// Capture serialized bytes, not just the preset passed to uTLS.
func captureCodexHello(t *testing.T, profile *Profile) ([]uint16, []uint16, map[uint16][]byte) {
	t.Helper()
	client, server := net.Pipe()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := performTLSHandshake(ctx, client, profile, "localhost:443"); done <- err }()
	header := make([]byte, 5)
	if _, err := io.ReadFull(server, header); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, binary.BigEndian.Uint16(header[3:]))
	if _, err := io.ReadFull(server, data); err != nil {
		t.Fatal(err)
	}
	_ = server.Close()
	<-done
	if header[0] != 22 || data[0] != 1 {
		t.Fatal("expected ClientHello")
	}
	pos := 4 + 2 + 32
	pos += 1 + int(data[pos])
	n := int(binary.BigEndian.Uint16(data[pos:]))
	pos += 2
	ciphers := decodeU16(data[pos : pos+n])
	pos += n
	pos += 1 + int(data[pos])
	n = int(binary.BigEndian.Uint16(data[pos:]))
	pos += 2
	end := pos + n
	var ids []uint16
	extensions := map[uint16][]byte{}
	for pos < end {
		id := binary.BigEndian.Uint16(data[pos:])
		size := int(binary.BigEndian.Uint16(data[pos+2:]))
		pos += 4
		ids = append(ids, id)
		extensions[id] = data[pos : pos+size]
		pos += size
	}
	return ciphers, ids, extensions
}

func decodeU16(data []byte) []uint16 {
	values := make([]uint16, 0, len(data)/2)
	for len(data) >= 2 {
		values = append(values, binary.BigEndian.Uint16(data))
		data = data[2:]
	}
	return values
}

func TestCodexClientHelloMatchesLocalCapture(t *testing.T) {
	for _, kind := range []string{"http", "ws", "custom_ca"} {
		t.Run(kind, func(t *testing.T) {
			raw, err := os.ReadFile("testdata/codex_" + kind + "_20261103.json")
			if err != nil {
				t.Fatal(err)
			}
			var want struct {
				CipherSuites []uint16                   `json:"cipher_suites"`
				Extensions   []uint16                   `json:"extensions"`
				Details      map[string]json.RawMessage `json:"details"`
			}
			if err := json.Unmarshal(raw, &want); err != nil {
				t.Fatal(err)
			}
			profile := CodexLocalHTTPProfile()
			if kind != "http" {
				profile = CodexLocalWSProfile()
				if kind == "custom_ca" {
					profile = CodexCustomCAHTTPProfile()
				}
			}
			seenOrders := map[string]bool{}
			for i := 0; i < 4; i++ {
				ciphers, ids, extensions := captureCodexHello(t, profile)
				seenOrders[fmt.Sprint(ids)] = true
				if !reflect.DeepEqual(ciphers, want.CipherSuites) {
					t.Fatalf("cipher suites: got %v want %v", ciphers, want.CipherSuites)
				}
				expectedIDs := slices.Clone(want.Extensions)
				if kind != "http" {
					slices.Sort(ids)
					slices.Sort(expectedIDs)
				}
				if !reflect.DeepEqual(ids, expectedIDs) {
					t.Fatalf("extensions: got %v want %v", ids, expectedIDs)
				}
				for _, id := range []uint16{10, 13} {
					var expected []uint16
					if err := json.Unmarshal(want.Details[fmt.Sprint(id)], &expected); err != nil {
						t.Fatal(err)
					}
					if got := decodeU16(extensions[id][2:]); !reflect.DeepEqual(got, expected) {
						t.Fatalf("extension %d: %v want %v", id, got, expected)
					}
				}
				if _, exists := extensions[16]; exists && kind != "custom_ca" {
					t.Fatal("native HTTP/WS must not advertise ALPN")
				}
				if kind != "http" {
					if got := decodeU16(extensions[43][1:]); !reflect.DeepEqual(got, []uint16{772, 771}) {
						t.Fatalf("versions: %v", got)
					}
					shares := extensions[51][2:]
					var groups, sizes []uint16
					for len(shares) > 0 {
						groups = append(groups, binary.BigEndian.Uint16(shares))
						n := binary.BigEndian.Uint16(shares[2:])
						sizes = append(sizes, n)
						shares = shares[4+int(n):]
					}
					if !reflect.DeepEqual(groups, []uint16{4588, 29}) || !reflect.DeepEqual(sizes, []uint16{1216, 32}) {
						t.Fatalf("shares: %v %v", groups, sizes)
					}
				}
			}
			if kind == "ws" && len(seenOrders) == 1 {
				t.Fatal("WS extension order was not randomized")
			}
		})
	}
}

func TestCodexVerifiedTLSRoundTrip(t *testing.T) {
	for _, profile := range []*Profile{CodexLocalHTTPProfile(), CodexLocalWSProfile()} {
		t.Run(profile.Name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "verified") }))
			defer server.Close()
			roots := x509.NewCertPool()
			roots.AddCert(server.Certificate())
			transport := &http.Transport{DisableCompression: true, DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				conn, err := (&net.Dialer{}).DialContext(ctx, network, addr)
				if err != nil {
					return nil, err
				}
				return performTLSHandshakeWithConfig(ctx, conn, profile, addr, &utls.Config{RootCAs: roots})
			}}
			defer transport.CloseIdleConnections()
			client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
			if profile.Name == CodexLocalHTTPProfile().Name {
				ordered := NewOrderedHTTPTransport(transport)
				defer ordered.CloseIdleConnections()
				client.Transport = ordered
			}
			response, err := client.Get(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if err != nil || string(data) != "verified" {
				t.Fatalf("response: %q %v", data, err)
			}
			// The normal constructor must reject the same untrusted leaf.
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, err = NewDialer(profile, nil).DialTLSContext(ctx, "tcp", server.Listener.Addr().String())
			if err == nil {
				t.Fatal("untrusted certificate accepted")
			}
		})
	}
}

func TestCodexProxyCONNECTCancellation(t *testing.T) {
	accepted := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.(http.Hijacker)
		conn, _, err := h.Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		close(accepted)
		_, _ = io.Copy(io.Discard, conn)
	}))
	defer server.Close()
	proxyURL, _ := url.Parse(server.URL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := NewHTTPProxyDialer(CodexLocalHTTPProfile(), proxyURL).DialTLSContext(ctx, "tcp", "localhost:443")
		done <- err
	}()
	select {
	case <-accepted:
	case <-time.After(5 * time.Second):
		t.Fatal("CONNECT not received")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected cancellation")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("CONNECT ignored cancellation")
	}
}

// Keep the default TLS certificate verifier exercised for the outer proxy hop.
func TestCodexHTTPSProxyDoesNotSendPlaintextCONNECT(t *testing.T) {
	observed := make(chan byte, 1)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		c, err := listener.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		var b [1]byte
		_, _ = io.ReadFull(c, b[:])
		observed <- b[0]
	}()
	proxyURL, _ := url.Parse("https://" + listener.Addr().String())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = NewHTTPProxyDialer(CodexLocalHTTPProfile(), proxyURL).DialTLSContext(ctx, "tcp", "localhost:443")
	if first := <-observed; first != 22 {
		t.Fatalf("proxy preface %d, want TLS handshake", first)
	}
}

func TestCodexWSTLSUpgradeAndCompressedFrames(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	gotHeader := make(chan http.Header, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader <- r.Header.Clone()
		conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeover})
		if err != nil {
			return
		}
		defer conn.CloseNow()
		typ, data, err := conn.Read(ctx)
		if err == nil {
			_ = conn.Write(ctx, typ, data)
		}
	}))
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	transport := &http.Transport{DisableCompression: true, DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, err := (&net.Dialer{}).DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		return performTLSHandshakeWithConfig(ctx, conn, CodexLocalWSProfile(), addr, &utls.Config{RootCAs: roots})
	}}
	defer transport.CloseIdleConnections()
	conn, _, err := coderws.Dial(ctx, "wss"+strings.TrimPrefix(server.URL, "https"), &coderws.DialOptions{
		HTTPClient: &http.Client{Transport: transport}, CompressionMode: coderws.CompressionContextTakeover,
		HTTPHeader: http.Header{"Originator": []string{"codex_exec"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	payload := []byte(strings.Repeat("compressed-echo", 500))
	if err := conn.Write(ctx, coderws.MessageText, payload); err != nil {
		t.Fatal(err)
	}
	_, got, err := conn.Read(ctx)
	if err != nil || string(got) != string(payload) {
		t.Fatalf("WS echo failed: %v", err)
	}
	headers := <-gotHeader
	if headers.Get("Originator") != "codex_exec" || headers.Get("Accept-Encoding") != "" {
		t.Fatalf("unexpected handshake headers: %v", headers)
	}
	// Document the remaining library negotiation boundary in a regression test.
	if headers.Get("Sec-WebSocket-Extensions") != "permessage-deflate" {
		t.Fatalf("review compression negotiation: %q", headers.Get("Sec-WebSocket-Extensions"))
	}
}
