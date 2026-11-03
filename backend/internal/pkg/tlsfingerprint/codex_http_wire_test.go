package tlsfingerprint

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	req "github.com/imroc/req/v3"
	reqh2 "github.com/imroc/req/v3/http2"
)

func codexCustomCAHeaderNames(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile("testdata/codex_custom_ca_http1_headers_20261103.json")
	if err != nil {
		t.Fatal(err)
	}
	var capture struct {
		Headers []string `json:"headers"`
	}
	if err := json.Unmarshal(data, &capture); err != nil {
		t.Fatal(err)
	}
	return capture.Headers
}

func codexCustomCAFixtureHeaders() http.Header {
	return http.Header{
		"User-Agent": {"fixture-cli"}, "Accept": {"text/event-stream"},
		"Authorization": {"Bearer test-only"}, "Content-Type": {"application/json"},
		"Originator": {"codex_exec"}, "Session-Id": {"session"}, "Thread-Id": {"thread"},
		"X-Client-Request-Id": {"request"}, "X-Codex-Beta-Features": {"remote_compaction_v2"},
		"X-Codex-Turn-Metadata": {"{}"}, "X-Codex-Window-Id": {"window"},
	}
}

func TestCodexCustomCAHTTP1WireAndReuse(t *testing.T) {
	s := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	cert := s.TLS.Certificates[0]
	s.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	wantNames := codexCustomCAHeaderNames(t)
	done := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		tc := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{cert}, NextProtos: []string{"http/1.1"}})
		reader := bufio.NewReader(tc)
		for range 2 {
			var raw strings.Builder
			var names []string
			for i := 0; ; i++ {
				line, err := reader.ReadString('\n')
				if err != nil {
					done <- err
					return
				}
				raw.WriteString(line)
				if line == "\r\n" {
					break
				}
				if i > 0 {
					name, _, _ := strings.Cut(line, ":")
					names = append(names, name)
				}
			}
			if !reflect.DeepEqual(names, wantNames) {
				done <- fmt.Errorf("HTTP/1 fields %v, want %v", names, wantNames)
				return
			}
			r, err := http.ReadRequest(bufio.NewReader(io.MultiReader(strings.NewReader(raw.String()), reader)))
			if err != nil {
				done <- err
				return
			}
			body, err := io.ReadAll(r.Body)
			_ = r.Body.Close()
			if err != nil || string(body) != "{}" || r.Host != "fixture.test" || r.ContentLength != 2 || r.Header.Get("Authorization") != "Bearer test-only" {
				done <- fmt.Errorf("request framing/values changed: host=%s length=%d body=%q error=%v", r.Host, r.ContentLength, body, err)
				return
			}
			if _, err := io.WriteString(tc, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nOK"); err != nil {
				done <- err
				return
			}
		}
		done <- nil
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
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	headers := codexCustomCAFixtureHeaders()
	headers["Host"] = []string{"ignored.invalid"}
	headers["Content-Length"] = []string{"999"}
	original := headers.Clone()
	for range 2 {
		r, _ := http.NewRequest("POST", "https://"+listener.Addr().String()+"/v1/responses", strings.NewReader("{}"))
		r.Header, r.Host = headers, "fixture.test"
		resp, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil || string(body) != "OK" {
			t.Fatalf("response %q: %v", body, err)
		}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(headers, original) {
		t.Fatal("mutated caller headers")
	}
}

func TestCustomH2SmallReceiveWindowProgress(t *testing.T) {
	// The old serializer advertised 1 KiB but accounted for 4 MiB. After the
	// first 1 KiB the server stalled because the client withheld WINDOW_UPDATE.
	want := bytes.Repeat([]byte("data: fixture\n\n"), 8192)
	s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor != 2 {
			t.Error("expected H2")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write(want)
	}))
	s.EnableHTTP2 = true
	s.StartTLS()
	defer s.Close()
	tx := req.NewTransport().EnableForceHTTP2()
	tx.TLSClientConfig = s.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	tx.SetHTTP2SettingsFrame(reqh2.Setting{ID: reqh2.SettingEnablePush, Val: 0}, reqh2.Setting{ID: reqh2.SettingInitialWindowSize, Val: 1024})
	defer tx.CloseIdleConnections()
	client := &http.Client{Transport: tx, Timeout: 5 * time.Second}
	for range 2 {
		resp, err := client.Get(s.URL)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil || !bytes.Equal(body, want) {
			t.Fatalf("response stalled/corrupted after %d bytes: %v", len(body), err)
		}
	}
}
