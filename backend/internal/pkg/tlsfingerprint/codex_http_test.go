package tlsfingerprint

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

func TestCodexCustomCAHTTPNegotiation(t *testing.T) {
	for _, env := range []string{"CODEX_CA_CERTIFICATE", "SSL_CERT_FILE"} {
		for _, h2 := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/h2=%v", env, h2), func(t *testing.T) {
				t.Setenv("CODEX_CA_CERTIFICATE", "")
				t.Setenv("SSL_CERT_FILE", "")
				s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if (r.ProtoMajor == 2) != h2 {
						t.Errorf("negotiated %s", r.Proto)
					}
					if r.Header.Get("User-Agent") != "fixture-agent" {
						t.Errorf("UA = %q", r.UserAgent())
					}
					if r.Header.Get("Accept-Encoding") != "" {
						t.Error("unexpected compression header")
					}
					_, _ = io.WriteString(w, "ok")
				}))
				s.EnableHTTP2 = h2
				s.StartTLS()
				defer s.Close()
				path := filepath.Join(t.TempDir(), "ca.pem")
				if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: s.Certificate().Raw}), 0600); err != nil {
					t.Fatal(err)
				}
				t.Setenv(env, path)
				base := &http.Transport{DisableCompression: true}
				profile := CodexHTTPProfile()
				if profile.Name != CodexCustomCAHTTPProfile().Name {
					t.Fatal("custom CA not selected")
				}
				if err := ConfigureTLS(base, profile, nil); err != nil {
					t.Fatal(err)
				}
				transport := NewCodexHTTPTransport(base, profile)
				defer transport.(interface{ CloseIdleConnections() }).CloseIdleConnections()
				client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
				for i := 0; i < 2; i++ {
					req, _ := http.NewRequest("POST", s.URL, bytes.NewBufferString("{}"))
					req.Header.Set("User-Agent", "fixture-agent")
					resp, err := client.Do(req)
					if err != nil {
						t.Fatal(err)
					}
					body, err := io.ReadAll(resp.Body)
					_ = resp.Body.Close()
					if err != nil || string(body) != "ok" {
						t.Fatalf("body %q: %v", body, err)
					}
				}
			})
		}
	}
}

func TestCodexH2WireSettings(t *testing.T) {
	// Exercise the real serializer and ALPN adapter; no external service is used.
	s := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	s.EnableHTTP2 = true
	s.StartTLS()
	cert := s.TLS.Certificates[0]
	s.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan error, 1)
	headerNames := codexCustomCAHeaderNames(t)
	wantNames := append([]string{":method", ":scheme", ":authority", ":path"}, headerNames[:len(headerNames)-2]...)
	wantNames = append(wantNames, "content-length")
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		tc := newTestH2Server(conn, cert)
		preface := make([]byte, len(http2.ClientPreface))
		if _, err := io.ReadFull(tc, preface); err != nil {
			done <- err
			return
		}
		if string(preface) != http2.ClientPreface {
			done <- fmt.Errorf("bad preface")
			return
		}
		f := http2.NewFramer(tc, tc)
		_ = f.WriteSettings()
		var settings []http2.Setting
		var window uint32
		var stream uint32
		for stream == 0 {
			frame, err := f.ReadFrame()
			if err != nil {
				done <- err
				return
			}
			switch v := frame.(type) {
			case *http2.SettingsFrame:
				if !v.IsAck() {
					_ = v.ForeachSetting(func(s http2.Setting) error { settings = append(settings, s); return nil })
					_ = f.WriteSettingsAck()
				}
			case *http2.WindowUpdateFrame:
				if v.StreamID == 0 {
					window = v.Increment
				}
			case *http2.HeadersFrame:
				stream = v.StreamID
				var names []string
				dec := hpack.NewDecoder(4096, func(field hpack.HeaderField) { names = append(names, field.Name) })
				if _, err := dec.Write(v.HeaderBlockFragment()); err != nil {
					done <- err
					return
				}
				if !reflect.DeepEqual(names, wantNames) {
					done <- fmt.Errorf("H2 header order %v", names)
					return
				}
			}
		}
		want := []http2.Setting{{ID: 2, Val: 0}, {ID: 4, Val: 2097152}, {ID: 5, Val: 16384}, {ID: 6, Val: 16384}}
		if !reflect.DeepEqual(settings, want) || window != 5177345 {
			done <- fmt.Errorf("settings=%v window=%d", settings, window)
			return
		}
		var header bytes.Buffer
		enc := hpack.NewEncoder(&header)
		_ = enc.WriteField(hpack.HeaderField{Name: ":status", Value: "200"})
		err = f.WriteHeaders(http2.HeadersFrameParam{StreamID: stream, BlockFragment: header.Bytes(), EndHeaders: true, EndStream: true})
		done <- err
	}()
	base := &http.Transport{DisableCompression: true}
	base.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, err := (&net.Dialer{}).DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		return performTLSHandshakeWithConfig(ctx, conn, CodexCustomCAHTTPProfile(), addr, testTrustConfig(cert.Certificate[0]))
	}
	transport := NewCodexHTTPTransport(base, CodexCustomCAHTTPProfile())
	defer transport.(interface{ CloseIdleConnections() }).CloseIdleConnections()
	request, _ := http.NewRequest("POST", "https://"+listener.Addr().String(), bytes.NewBufferString("{}"))
	request.Header = codexCustomCAFixtureHeaders()
	resp, err := (&http.Client{Transport: transport, Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func newTestH2Server(conn net.Conn, cert tls.Certificate) *tls.Conn {
	return tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{cert}, NextProtos: []string{"h2"}})
}
func testTrustConfig(der []byte) *utls.Config {
	pool := x509.NewCertPool()
	cert, _ := x509.ParseCertificate(der)
	pool.AddCert(cert)
	return &utls.Config{RootCAs: pool}
}
