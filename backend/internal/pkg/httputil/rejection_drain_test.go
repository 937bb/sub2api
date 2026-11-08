package httputil

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEarlyRejectionPreservesResponseDuringLargeUpload(t *testing.T) {
	const size = 16 << 20
	for _, partial := range []bool{false, true} {
		t.Run(fmt.Sprint(partial), func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if partial {
					_, _ = io.CopyN(io.Discard, r.Body, 17)
				}
				w.WriteHeader(http.StatusForbidden)
				_, _ = io.WriteString(w, "rejected")
			})
			srv := httptest.NewServer(WithBodyReadTimeout(http.MaxBytesHandler(WithRejectedBodyDrain(handler, size+1), size+1), time.Second))
			defer srv.Close()
			conn, err := net.Dial("tcp", srv.Listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = conn.Close() }()
			_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
			_, err = fmt.Fprintf(conn, "POST / HTTP/1.1\r\nHost: test\r\nContent-Length: %d\r\nConnection: close\r\n\r\n", size)
			if err != nil {
				t.Fatal(err)
			}
			chunk := strings.Repeat("x", 64<<10)
			// Match a proxy that finishes writing before consuming the response.
			for sent := 0; sent < size; sent += len(chunk) {
				if _, err := io.WriteString(conn, chunk); err != nil {
					t.Fatalf("early rejection reset the upload: %v", err)
				}
			}
			resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = resp.Body.Close() }()
			body, err := io.ReadAll(resp.Body)
			if err != nil || resp.StatusCode != http.StatusForbidden || string(body) != "rejected" {
				t.Fatalf("lost original rejection: status=%d body=%q err=%v", resp.StatusCode, body, err)
			}
		})
	}
}

func TestEarlyRejectionDrainIsBoundedAndSkipsSuccessfulOrHTTP2Responses(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		status, proto, wantRead int
	}{
		{"bounded rejection", 403, 1, 1024},
		{"successful SSE", 200, 1, 0},
		{"websocket upgrade", 101, 1, 0},
		{"HTTP2 rejection", 403, 2, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := strings.NewReader(strings.Repeat("x", 4096))
			req := httptest.NewRequest(http.MethodPost, "/", io.NopCloser(reader))
			req.ContentLength = -1
			req.ProtoMajor = tc.proto
			WithRejectedBodyDrain(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				if tc.status == 200 {
					w.(http.Flusher).Flush()
				}
			}), 1024).ServeHTTP(httptest.NewRecorder(), req)
			if got := 4096 - reader.Len(); got != tc.wantRead {
				t.Fatalf("read %d bytes, want %d", got, tc.wantRead)
			}
		})
	}
}

func TestEarlyRejectionDrainRetainsUploadTimeout(t *testing.T) {
	done := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		close(done)
	})
	srv := httptest.NewServer(WithBodyReadTimeout(WithRejectedBodyDrain(handler, 2<<20), 50*time.Millisecond))
	defer srv.Close()
	conn, err := net.Dial("tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	_, err = io.WriteString(conn, "POST / HTTP/1.1\r\nHost: test\r\nContent-Length: 1048576\r\n\r\nx")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("rejection drain retained a stalled upload")
	}
}
