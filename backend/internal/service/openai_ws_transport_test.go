package service

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

type codexWireListener struct {
	net.Listener
	headers chan string
}

func (l *codexWireListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &codexWireConn{Conn: c, headers: l.headers}, nil
}

type codexWireConn struct {
	net.Conn
	buf     bytes.Buffer
	done    bool
	headers chan string
	mu      sync.Mutex
}

func (c *codexWireConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.done {
		c.buf.Write(p[:n])
		if end := strings.Index(c.buf.String(), "\r\n\r\n"); end >= 0 {
			c.headers <- c.buf.String()[:end]
			c.done = true
			c.buf.Reset()
		}
	}
	return n, err
}

func TestCodexWSWireMatchesMeasuredUpgrade(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	serverDone := make(chan error, 1)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeover})
		if err != nil {
			serverDone <- err
			return
		}
		defer conn.CloseNow()
		for range 2 {
			typ, p, err := conn.Read(ctx)
			if err != nil {
				serverDone <- err
				return
			}
			if err = conn.Write(ctx, typ, p); err != nil {
				serverDone <- err
				return
			}
		}
		serverDone <- nil
	}))
	captured := make(chan string, 1)
	server.Listener = &codexWireListener{Listener: server.Listener, headers: captured}
	server.Start()
	defer server.Close()
	headers := http.Header{}
	headers.Set("User-Agent", codexCLIUserAgent)
	for _, name := range []string{"Authorization", "Originator", "OpenAI-Beta", "X-Codex-Turn-Metadata", "X-Codex-Beta-Features", "X-Client-Request-Id", "Session-Id", "Thread-Id", "X-Codex-Window-Id"} {
		headers.Set(name, "test-only")
	}
	original := headers.Clone()
	dialer := newDefaultOpenAIWSClientDialer().(*coderOpenAIWSClientDialer)
	conn, _, _, err := dialer.Dial(ctx, strings.Replace(server.URL, "http://", "ws://", 1)+"/v1/responses", headers, "")
	require.NoError(t, err)
	defer conn.(openAIWSForceCloser).CloseNow()
	require.Equal(t, original, headers, "serialization must not mutate the pool identity")
	var wire string
	select {
	case wire = <-captured:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	lines := strings.Split(wire, "\r\n")
	require.Equal(t, "GET /v1/responses HTTP/1.1", lines[0])
	names := []string{}
	for _, line := range lines[1:] {
		name, _, ok := strings.Cut(line, ":")
		require.True(t, ok)
		names = append(names, name)
	}
	// Names/order from the 2026-11-03 local bundled CLI loopback capture.
	require.Equal(t, []string{"Host", "Connection", "Upgrade", "Sec-WebSocket-Version", "Sec-WebSocket-Key", "authorization", "user-agent", "originator", "openai-beta", "x-codex-turn-metadata", "x-codex-beta-features", "x-client-request-id", "session-id", "thread-id", "x-codex-window-id", "sec-websocket-extensions"}, names)
	require.Equal(t, "sec-websocket-extensions: permessage-deflate; client_max_window_bits", lines[len(lines)-1])
	for range 2 {
		require.NoError(t, conn.WriteJSON(ctx, map[string]string{"text": strings.Repeat("compressed-echo", 100)}))
		response, err := conn.ReadMessage(ctx)
		require.NoError(t, err)
		require.Contains(t, string(response), strings.Repeat("compressed-echo", 100))
	}
	require.NoError(t, <-serverDone)
}

func TestCodexWSPlaintextStillUsesExplicitProxy(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	seen := make(chan string, 1)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { seen <- r.URL.String(); w.WriteHeader(502) }))
	defer proxy.Close()
	headers := http.Header{"User-Agent": {codexCLIUserAgent}}
	_, status, _, err := newDefaultOpenAIWSClientDialer().Dial(ctx, "ws://must-not-resolve.invalid/v1/responses", headers, proxy.URL)
	require.Error(t, err)
	require.Equal(t, 502, status)
	select {
	case target := <-seen:
		require.Equal(t, "http://must-not-resolve.invalid/v1/responses", target)
	case <-ctx.Done():
		t.Fatal("proxy bypassed")
	}
}
