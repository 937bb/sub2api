package tlsfingerprint

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCodexNativeHTTPWireOrderAndReuse(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	captures := make(chan []string, 2)
	done := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		for range 2 {
			line, err := reader.ReadString('\n')
			if err != nil {
				done <- err
				return
			}
			if line != "POST /v1/responses HTTP/1.1\r\n" {
				done <- fmt.Errorf("request line %q", line)
				return
			}
			headers := []string{}
			for {
				line, err = reader.ReadString('\n')
				if err != nil {
					done <- err
					return
				}
				if line == "\r\n" {
					break
				}
				name, _, _ := strings.Cut(line, ":")
				headers = append(headers, name)
			}
			if _, err = io.CopyN(io.Discard, reader, 2); err != nil {
				done <- err
				return
			}
			captures <- headers
			if _, err = io.WriteString(conn, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nOK"); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	transport := NewOrderedHTTPTransport(&http.Transport{DisableCompression: true})
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	headers := http.Header{"User-Agent": {"fixture-cli"}, "Accept": {"text/event-stream"}, "Authorization": {"Bearer test-only"}, "Content-Type": {"application/json"}, "Originator": {"codex_exec"}, "Session-Id": {"session"}, "Thread-Id": {"thread"}, "X-Client-Request-Id": {"request"}, "X-Codex-Beta-Features": {"remote_compaction_v2"}, "X-Codex-Turn-Metadata": {"{}"}, "X-Codex-Window-Id": {"window"}}
	original := headers.Clone()
	want := []string{"Host", "User-Agent", "Content-Length", "Accept", "Authorization", "Content-Type", "Originator", "Session-Id", "Thread-Id", "X-Client-Request-Id", "X-Codex-Beta-Features", "X-Codex-Turn-Metadata", "X-Codex-Window-Id"}
	for range 2 {
		r, _ := http.NewRequestWithContext(ctx, "POST", "http://"+listener.Addr().String()+"/v1/responses", strings.NewReader("{}"))
		r.Header = headers
		response, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		_, err = io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		got := <-captures
		if !reflect.DeepEqual(want, got) {
			t.Fatalf("header order: got %v want %v", got, want)
		}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(headers, original) {
		t.Fatal("mutated original request headers")
	}
}
