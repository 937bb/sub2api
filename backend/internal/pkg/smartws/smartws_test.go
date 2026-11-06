package smartws

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func testPool(t *testing.T, capacity int) *Pool {
	t.Helper()
	p := newPool(poolConfig{capacity: capacity, perKey: 4, idle: time.Minute, lifetime: 20 * time.Minute,
		dialConcurrent: 128, dialBurst: 1024, dialRate: 1000, handshake: time.Second, allowed: func(u *url.URL) bool { return u != nil && u.Scheme == "http" && u.Hostname() == "127.0.0.1" },
		dialer: &websocket.Dialer{HandshakeTimeout: time.Second, NetDialContext: (&net.Dialer{Timeout: time.Second}).DialContext}})
	t.Cleanup(p.Close)
	return p
}
func request(t *testing.T, target, body, session string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, target, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Chatgpt-Account-Id", "test-account")
	if session != "" {
		req.Header.Set("Session_id", session)
	}
	return req
}

const validBody = `{"model":"test","stream":true,"input":"hello"}`

func emit(ws *websocket.Conn, kind, id, status string) error {
	m := map[string]any{"type": kind}
	if id != "" {
		r := map[string]any{"id": id}
		if status != "" {
			r["status"] = status
		}
		m["response"] = r
	}
	return ws.WriteJSON(m)
}
func successServer(t *testing.T) (*httptest.Server, *atomic.Int64, *atomic.Int64, chan []byte) {
	t.Helper()
	var connections, requests atomic.Int64
	messages := make(chan []byte, 128)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Proxy-Authorization") != "" {
			t.Error("proxy credentials reached target")
		}
		ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		connections.Add(1)
		defer ws.Close()
		for {
			_, raw, err := ws.ReadMessage()
			if err != nil {
				return
			}
			n := requests.Add(1)
			messages <- raw
			id := fmt.Sprintf("response-%d", n)
			if emit(ws, "response.created", id, "in_progress") != nil {
				return
			}
			if ws.WriteJSON(map[string]any{"type": "response.output_text.delta", "response_id": id, "delta": "hello"}) != nil {
				return
			}
			if ws.WriteJSON(map[string]any{"type": "response.completed", "response": map[string]any{"id": id, "status": "completed", "usage": map[string]int{"input_tokens": 12, "output_tokens": 3}}}) != nil {
				return
			}
		}
	}))
	t.Cleanup(s.Close)
	return s, &connections, &requests, messages
}
func consume(t *testing.T, p *Pool, r *http.Request, proxy, scope string) string {
	t.Helper()
	response, handled, err := p.Try(r, proxy, scope)
	if err != nil || !handled || response == nil {
		t.Fatalf("Try handled=%v err=%v", handled, err)
	}
	defer response.Body.Close()
	all, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.Header.Get("X-Sub2-Upstream-Transport") != "websocket" {
		t.Fatal("missing actual upstream marker")
	}
	return string(all)
}
func TestRealSocketConversionReuseAndOriginalBody(t *testing.T) {
	s, connections, requests, messages := successServer(t)
	p := testPool(t, 8)
	body := `{"model":"test","stream":true,"input":[{"type":"message","content":[{"type":"input_image","image_url":"data:image/png;base64,AA=="},{"type":"input_text","text":"x"}]}],"tools":[{"type":"function","name":"tool","parameters":{"type":"object"}}],"background":false}`
	for i := 0; i < 2; i++ {
		r := request(t, s.URL, body, "session")
		output := consume(t, p, r, "", "group1/user2/key3/session")
		if !strings.Contains(output, `"input_tokens":12`) || !strings.Contains(output, "event: response.completed") {
			t.Fatal(output)
		}
		original, err := io.ReadAll(r.Body)
		if err != nil || string(original) != body {
			t.Fatal("original request consumed")
		}
		var m map[string]json.RawMessage
		if json.Unmarshal(<-messages, &m) != nil {
			t.Fatal("wire JSON")
		}
		if string(m["type"]) != `"response.create"` || m["stream"] != nil || m["background"] != nil || !bytes.Contains(m["input"], []byte("input_image")) || m["tools"] == nil {
			t.Fatalf("wire fields %s", m)
		}
	}
	if connections.Load() != 1 || requests.Load() != 2 {
		t.Fatalf("reuse connections=%d requests=%d", connections.Load(), requests.Load())
	}
	if p.Stats()["active"] != 0 {
		t.Fatal(p.Stats())
	}
}
func TestEligibilityRejectsWithoutConsumingOriginal(t *testing.T) {
	s, _, requests, _ := successServer(t)
	p := testPool(t, 8)
	invalid := []string{
		`{"stream":false}`, `{"stream":"true"}`, `{"stream":true,"background":true}`,
		`{"stream":true,"previous_response_id":"prior-unknown"}`, `{"stream":true,"conversation":"x"}`,
		`{"stream":true,"type":"response.create"}`, `{"stream":true,"stream_id":"x"}`,
		`{"stream":true,"metadata":{"previous_response_id":"x"}}`,
		`{"stream":true,"metadata":{"conversation":"x"}}`,
		`{"stream":true,"stream":true}`, `{"stream":true,"input":{"x":1,"x":2}}`,
		`{"stream":true,"n":NaN}`, `[{"stream":true}]`, `{"stream":true} {}`,
	}
	for _, body := range invalid {
		r := request(t, s.URL, body, "s")
		resp, handled, err := p.Try(r, "", "scope")
		if handled || err != nil || resp != nil {
			t.Fatalf("accepted %s", body)
		}
		raw, _ := io.ReadAll(r.Body)
		if string(raw) != body {
			t.Fatal("consumed ineligible body")
		}
	}
	if requests.Load() != 0 {
		t.Fatal("unexpected generation")
	}
}
func TestTargetAndBodyReplayRequirements(t *testing.T) {
	p := NewPool()
	defer p.Close()
	for _, target := range []string{"http://chatgpt.com/backend-api/codex/responses", "https://evil.example/backend-api/codex/responses", "https://chatgpt.com/backend-api/codex/responses/compact", "https://u:p@chatgpt.com/backend-api/codex/responses", "https://chatgpt.com:444/backend-api/codex/responses"} {
		r := request(t, target, validBody, "s")
		_, h, e := p.Try(r, "", "scope")
		if h || e != nil {
			t.Fatal(target)
		}
	}
	r := request(t, "https://chatgpt.com/backend-api/codex/responses", validBody, "s")
	r.GetBody = nil
	_, h, e := p.Try(r, "", "scope")
	if h || e != nil {
		t.Fatal("non replayable body used")
	}
	r = request(t, "https://chatgpt.com/backend-api/codex/responses", validBody, "s")
	for _, proxy := range []string{"https://proxy:443", "file:///x", "socks4://proxy:1080"} {
		_, h, e = p.Try(r, proxy, "scope")
		if h || e != nil {
			t.Fatal(proxy)
		}
	}
}
func TestTrustedScopeAuthenticationAndSessionIsolation(t *testing.T) {
	s, connections, _, _ := successServer(t)
	p := testPool(t, 32)
	cases := []struct{ scope, session, token string }{
		{"group1/user1", "s", "a"}, {"group2/user1", "s", "a"}, {"group1/user2", "s", "a"},
		{"group1/user1", "s", "changed"}, {"group1/user1", "different", "a"},
		{"group1/user1", "", "a"}, {"group1/user1", "", "a"}, {"", "s", "a"}, {"", "s", "a"},
	}
	for _, c := range cases {
		r := request(t, s.URL, validBody, c.session)
		r.Header.Set("Authorization", "Bearer "+c.token)
		consume(t, p, r, "", c.scope)
	}
	if connections.Load() != int64(len(cases)) {
		t.Fatalf("identity reused %d", connections.Load())
	}
}
func TestHandshakeFallbackAndPreservedAuthRateErrors(t *testing.T) {
	for _, status := range []int{400, 404, 405, 426, 501, 401, 403, 429, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var attempts atomic.Int64
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = io.WriteString(w, `{"error":"unchanged"}`)
			}))
			t.Cleanup(s.Close)
			p := testPool(t, 8)
			unsupported := status == 400 || status == 404 || status == 405 || status == 426 || status == 501
			for i := 0; i < 2; i++ {
				resp, handled, err := p.Try(request(t, s.URL, validBody, "s"), "", "scope")
				if err != nil {
					t.Fatal(err)
				}
				if unsupported {
					if handled || resp != nil {
						t.Fatal("unsupported not fallback")
					}
				} else {
					if !handled || resp.StatusCode != status {
						t.Fatal("status lost")
					}
					raw, _ := io.ReadAll(resp.Body)
					_ = resp.Body.Close()
					if !bytes.Contains(raw, []byte("unchanged")) {
						t.Fatal(string(raw))
					}
				}
			}
			expected := int64(2)
			if unsupported {
				expected = 1
			}
			if attempts.Load() != expected {
				t.Fatal(attempts.Load())
			}
		})
	}
}
func TestPostDispatchFailureNeverReplayed(t *testing.T) {
	var received atomic.Int64
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()
		if _, _, err = ws.ReadMessage(); err == nil {
			received.Add(1)
		}
	}))
	t.Cleanup(s.Close)
	p := testPool(t, 8)
	resp, handled, err := p.Try(request(t, s.URL, validBody, "s"), "", "scope")
	if !handled {
		t.Fatal("dispatched request became fallback")
	}
	if err == nil {
		_, err = io.ReadAll(resp.Body)
		_ = resp.Body.Close()
	}
	var sent *SentError
	if !errors.As(err, &sent) {
		t.Fatalf("missing sent semantics %v", err)
	}
	if received.Load() != 1 {
		t.Fatalf("generation repeated %d", received.Load())
	}
}
func TestMidStreamCancellationClosesSocket(t *testing.T) {
	serverDone := make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()
		defer close(serverDone)
		if _, _, err = ws.ReadMessage(); err != nil {
			return
		}
		_ = emit(ws, "response.created", "r", "in_progress")
		_, _, _ = ws.ReadMessage()
	}))
	t.Cleanup(s.Close)
	p := testPool(t, 8)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resp, h, e := p.Try(request(t, s.URL, validBody, "s").WithContext(ctx), "", "scope")
	if e != nil || !h {
		t.Fatal(e)
	}
	buf := make([]byte, 4096)
	if _, e = resp.Body.Read(buf); e != nil {
		t.Fatal(e)
	}
	cancel()
	_, e = io.ReadAll(resp.Body)
	var sent *SentError
	if !errors.As(e, &sent) {
		t.Fatalf("cancel %v", e)
	}
	_ = resp.Body.Close()
	select {
	case <-serverDone:
	case <-time.After(2 * time.Second):
		t.Fatal("cancel leaked upstream socket")
	}
}
func TestTerminalFailurePreservedNotReused(t *testing.T) {
	var connections atomic.Int64
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, e := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if e != nil {
			return
		}
		connections.Add(1)
		defer ws.Close()
		if _, _, e = ws.ReadMessage(); e != nil {
			return
		}
		_ = emit(ws, "response.created", "r", "in_progress")
		_ = ws.WriteJSON(map[string]any{"type": "response.failed", "response": map[string]any{"id": "r", "status": "failed", "error": map[string]any{"code": "rate_limit_exceeded"}}})
		_, _, _ = ws.ReadMessage()
	}))
	t.Cleanup(s.Close)
	p := testPool(t, 8)
	for i := 0; i < 2; i++ {
		out := consume(t, p, request(t, s.URL, validBody, "s"), "", "scope")
		if !strings.Contains(out, "rate_limit_exceeded") {
			t.Fatal(out)
		}
	}
	if connections.Load() != 2 {
		t.Fatal("failed connection reused")
	}
}
func TestMalformedAndMismatchedEventsPoisonConnection(t *testing.T) {
	for _, event := range []string{
		`{"type":"response.output_text.delta","delta":"before-created"}`,
		`{"type":"response.created","response":{"id":"r","status":"in_progress"},"type":"response.completed"}`,
		`{"type":"bad\ninjection"}`,
		`{"type":"response.completed","response":{"id":"r","status":"in_progress"}}`,
	} {
		t.Run(event, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ws, e := (&websocket.Upgrader{}).Upgrade(w, r, nil)
				if e != nil {
					return
				}
				defer ws.Close()
				_, _, _ = ws.ReadMessage()
				_ = ws.WriteMessage(websocket.TextMessage, []byte(event))
			}))
			t.Cleanup(s.Close)
			p := testPool(t, 8)
			resp, h, e := p.Try(request(t, s.URL, validBody, "s"), "", "scope")
			if e != nil || !h {
				t.Fatal(e)
			}
			_, e = io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			var sent *SentError
			if !errors.As(e, &sent) {
				t.Fatal("malformed event accepted")
			}
		})
	}
}
func TestPoolCapacityDoesNotQueueOrDispatch(t *testing.T) {
	hold := make(chan struct{})
	var requests atomic.Int64
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, e := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if e != nil {
			return
		}
		defer ws.Close()
		_, _, e = ws.ReadMessage()
		if e != nil {
			return
		}
		requests.Add(1)
		_ = emit(ws, "response.created", "r", "in_progress")
		<-hold
	}))
	t.Cleanup(s.Close)
	defer close(hold)
	p := testPool(t, 1)
	first, h, e := p.Try(request(t, s.URL, validBody, "s"), "", "scope")
	if e != nil || !h {
		t.Fatal(e)
	}
	defer first.Body.Close()
	started := time.Now()
	_, h, e = p.Try(request(t, s.URL, validBody, "other"), "", "other")
	if h || e != nil || time.Since(started) > 250*time.Millisecond {
		t.Fatal("capacity queued or dispatched")
	}
	if p.Stats()["pool_full"] != uint64(1) {
		t.Fatal(p.Stats())
	}
}
func TestConcurrentBoundedPool(t *testing.T) {
	s, _, _, _ := successServer(t)
	p := testPool(t, 4)
	var workers sync.WaitGroup
	var success, fallback atomic.Int64
	for i := 0; i < 40; i++ {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			req, err := http.NewRequest("POST", s.URL, strings.NewReader(validBody))
			if err != nil {
				t.Error(err)
				return
			}
			req.Header.Set("Authorization", "Bearer a")
			req.Header.Set("Session_id", fmt.Sprint(i))
			resp, h, e := p.Try(req, "", fmt.Sprint(i))
			if e != nil {
				t.Error(e)
				return
			}
			if !h {
				fallback.Add(1)
				return
			}
			_, e = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if e != nil {
				t.Error(e)
				return
			}
			success.Add(1)
		}(i)
	}
	workers.Wait()
	if success.Load() == 0 || success.Load()+fallback.Load() != 40 || p.Stats()["active"] != 0 {
		t.Fatal(success.Load(), fallback.Load(), p.Stats())
	}
	if p.Stats()["connections"].(int) > 4 {
		t.Fatal("capacity exceeded")
	}
}
func TestHTTPConnectProxyHonored(t *testing.T) {
	target, _, _, _ := successServer(t)
	var connects atomic.Int64
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "CONNECT" {
			t.Error(r.Method)
			w.WriteHeader(405)
			return
		}
		upstream, err := net.DialTimeout("tcp", r.Host, time.Second)
		if err != nil {
			w.WriteHeader(502)
			return
		}
		client, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			_ = upstream.Close()
			return
		}
		connects.Add(1)
		_, _ = io.WriteString(client, "HTTP/1.1 200 Connection established\r\n\r\n")
		go func() { _, _ = io.Copy(upstream, client); _ = upstream.Close() }()
		_, _ = io.Copy(client, upstream)
		_ = client.Close()
	}))
	t.Cleanup(proxy.Close)
	p := testPool(t, 8)
	consume(t, p, request(t, target.URL, validBody, "s"), proxy.URL, "scope")
	if connects.Load() != 1 {
		t.Fatal("proxy bypassed")
	}
}
func TestSOCKS5ProxyHonored(t *testing.T) {
	target, _, _, _ := successServer(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	var connects atomic.Int64
	go func() {
		for {
			client, e := listener.Accept()
			if e != nil {
				return
			}
			go func() {
				defer client.Close()
				var e error
				head := make([]byte, 2)
				if _, e = io.ReadFull(client, head); e != nil {
					return
				}
				methods := make([]byte, int(head[1]))
				if _, e = io.ReadFull(client, methods); e != nil {
					return
				}
				_, _ = client.Write([]byte{5, 0})
				head = make([]byte, 4)
				if _, e = io.ReadFull(client, head); e != nil {
					return
				}
				var host string
				switch head[3] {
				case 1:
					b := make([]byte, 4)
					if _, e = io.ReadFull(client, b); e != nil {
						return
					}
					host = net.IP(b).String()
				case 3:
					n := make([]byte, 1)
					_, _ = io.ReadFull(client, n)
					b := make([]byte, int(n[0]))
					if _, e = io.ReadFull(client, b); e != nil {
						return
					}
					host = string(b)
				default:
					return
				}
				port := make([]byte, 2)
				if _, e = io.ReadFull(client, port); e != nil {
					return
				}
				upstream, e := net.DialTimeout("tcp", net.JoinHostPort(host, fmt.Sprint(binary.BigEndian.Uint16(port))), time.Second)
				if e != nil {
					return
				}
				defer upstream.Close()
				connects.Add(1)
				_, _ = client.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0})
				go func() { _, _ = io.Copy(upstream, client); _ = upstream.Close() }()
				_, _ = io.Copy(client, upstream)
			}()
		}
	}()
	p := testPool(t, 8)
	consume(t, p, request(t, target.URL, validBody, "s"), "socks5h://"+listener.Addr().String(), "scope")
	if connects.Load() != 1 {
		t.Fatal("SOCKS proxy bypassed")
	}
}

func TestResponseIdentityMismatch(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, e := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if e != nil {
			return
		}
		defer ws.Close()
		_, _, _ = ws.ReadMessage()
		_ = emit(ws, "response.created", "one", "in_progress")
		_ = ws.WriteJSON(map[string]any{"type": "response.output_text.delta", "response_id": "other-user", "delta": "wrong"})
	}))
	t.Cleanup(s.Close)
	p := testPool(t, 4)
	response, h, e := p.Try(request(t, s.URL, validBody, "s"), "", "scope")
	if e != nil || !h {
		t.Fatal(e)
	}
	raw, e := io.ReadAll(response.Body)
	_ = response.Body.Close()
	var sent *SentError
	if !errors.As(e, &sent) || bytes.Contains(raw, []byte("wrong")) {
		t.Fatal("identity mismatch delivered", e)
	}
}
func TestExpiredConnectionNeverReused(t *testing.T) {
	s, connections, _, _ := successServer(t)
	p := testPool(t, 4)
	p.config.idle = 5 * time.Millisecond
	consume(t, p, request(t, s.URL, validBody, "s"), "", "scope")
	time.Sleep(10 * time.Millisecond)
	consume(t, p, request(t, s.URL, validBody, "s"), "", "scope")
	if connections.Load() != 2 {
		t.Fatal("expired connection reused")
	}
}
func TestFrameLimitAndCancellationBackpressure(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, e := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if e != nil {
			return
		}
		defer ws.Close()
		_, _, _ = ws.ReadMessage()
		_ = emit(ws, "response.created", "r", "in_progress")
		_ = ws.WriteJSON(map[string]any{"type": "response.output_text.delta", "response_id": "r", "delta": strings.Repeat("x", maxFrameBytes+1)})
	}))
	t.Cleanup(s.Close)
	p := testPool(t, 4)
	response, h, e := p.Try(request(t, s.URL, validBody, "s"), "", "scope")
	if e != nil || !h {
		t.Fatal(e)
	}
	_, e = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	var sent *SentError
	if !errors.As(e, &sent) {
		t.Fatal("oversize frame not bounded", e)
	}
}
func TestIdleUnsolicitedFramePoisonsConnection(t *testing.T) {
	unsolicited := make(chan struct{})
	sent := make(chan struct{})
	var connections atomic.Int64
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, e := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if e != nil {
			return
		}
		defer ws.Close()
		n := connections.Add(1)
		_, _, _ = ws.ReadMessage()
		id := fmt.Sprint(n)
		_ = emit(ws, "response.created", id, "in_progress")
		_ = emit(ws, "response.completed", id, "completed")
		if n == 1 {
			<-unsolicited
			_ = emit(ws, "response.created", "stale", "in_progress")
			close(sent)
		}
		_, _, _ = ws.ReadMessage()
	}))
	t.Cleanup(s.Close)
	p := testPool(t, 4)
	consume(t, p, request(t, s.URL, validBody, "s"), "", "scope")
	close(unsolicited)
	<-sent
	deadline := time.Now().Add(time.Second)
	for p.Stats()["connections"].(int) != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	consume(t, p, request(t, s.URL, validBody, "s"), "", "scope")
	if connections.Load() != 2 {
		t.Fatal("unsolicited connection reused")
	}
}

func TestCodexControlEventsBeforeCreatedPreserved(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, e := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if e != nil {
			return
		}
		defer ws.Close()
		_, _, _ = ws.ReadMessage()
		_ = ws.WriteJSON(map[string]any{"type": "codex.rate_limits", "rate_limits": map[string]any{"remaining": 10}})
		_ = ws.WriteJSON(map[string]any{"type": "codex.response.metadata", "headers": map[string]string{"x-request-id": "test-request"}})
		_ = emit(ws, "response.created", "r", "in_progress")
		_ = ws.WriteJSON(map[string]any{"type": "response.output_text.delta", "delta": "OK"})
		_ = ws.WriteJSON(map[string]any{"type": "responsesapi.websocket_timing", "timing_metrics": map[string]int{"test": 1}})
		_ = emit(ws, "response.completed", "r", "completed")
		_, _, _ = ws.ReadMessage()
	}))
	t.Cleanup(s.Close)
	p := testPool(t, 4)
	output := consume(t, p, request(t, s.URL, validBody, "s"), "", "scope")
	for _, event := range []string{"codex.rate_limits", "codex.response.metadata", "responsesapi.websocket_timing", "response.completed"} {
		if !strings.Contains(output, "event: "+event) {
			t.Fatal("control metadata lost", event)
		}
	}
	if p.Stats()["completed"] != uint64(1) || p.Stats()["stream_failed"] != nil {
		t.Fatal(p.Stats())
	}
}
func TestUnknownBusinessEventBeforeCreatedStillFails(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, e := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if e != nil {
			return
		}
		defer ws.Close()
		_, _, _ = ws.ReadMessage()
		_ = ws.WriteJSON(map[string]string{"type": "response.output_text.delta", "delta": "must not pass"})
	}))
	t.Cleanup(s.Close)
	p := testPool(t, 4)
	response, h, e := p.Try(request(t, s.URL, validBody, "s"), "", "scope")
	if e != nil || !h {
		t.Fatal(e)
	}
	raw, e := io.ReadAll(response.Body)
	_ = response.Body.Close()
	var sent *SentError
	if !errors.As(e, &sent) || bytes.Contains(raw, []byte("must not pass")) {
		t.Fatal("business guard lost")
	}
	if p.Stats()["stream_failure_before_created"] != uint64(1) {
		t.Fatal("missing safe reason", p.Stats())
	}
}
func TestExpiredSweepDetachesBeforeClose(t *testing.T) {
	s, connections, _, _ := successServer(t)
	p := testPool(t, 4)
	consume(t, p, request(t, s.URL, validBody, "s"), "", "scope")
	p.expireIdle(time.Now().Add(p.config.idle + time.Second))
	if p.Stats()["connections"] != 0 {
		t.Fatal("expired member available for reacquisition")
	}
	consume(t, p, request(t, s.URL, validBody, "s"), "", "scope")
	if connections.Load() != 2 {
		t.Fatal("expired lease reacquired")
	}
}

func TestOversizeRequestsFallbackBeforeParsingUntouched(t *testing.T) {
	s, _, requests, _ := successServer(t)
	p := testPool(t, 4)
	large := "{" + strings.Repeat(" ", maxRequestBytes) + "}"
	r := request(t, s.URL, large, "s")
	r.GetBody = func() (io.ReadCloser, error) {
		t.Error("known oversized body copied")
		return nil, errors.New("must not copy")
	}
	resp, h, e := p.Try(r, "", "scope")
	if h || e != nil || resp != nil {
		t.Fatal("known oversize request handled")
	}
	original, _ := io.ReadAll(r.Body)
	if string(original) != large {
		t.Fatal("original known-length body changed")
	}
	r = request(t, s.URL, large, "s")
	r.ContentLength = -1
	resp, h, e = p.Try(r, "", "scope")
	if h || e != nil || resp != nil {
		t.Fatal("unknown-length oversize request handled")
	}
	original, _ = io.ReadAll(r.Body)
	if string(original) != large {
		t.Fatal("original unknown-length body changed")
	}
	if requests.Load() != 0 || p.Stats()["request_too_large"] != uint64(2) {
		t.Fatal("oversize dispatched", p.Stats())
	}
}
func TestPolicyAndSizeCloseSuppressFutureRequestsWithoutReplay(t *testing.T) {
	for _, code := range []int{1008, 1009} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			var generation, connections atomic.Int64
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ws, e := (&websocket.Upgrader{}).Upgrade(w, r, nil)
				if e != nil {
					return
				}
				defer ws.Close()
				connections.Add(1)
				if _, _, e = ws.ReadMessage(); e != nil {
					return
				}
				generation.Add(1)
				_ = ws.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, "sensitive-upstream-detail"), time.Now().Add(time.Second))
			}))
			t.Cleanup(s.Close)
			p := testPool(t, 4)
			r := request(t, s.URL, validBody, "s")
			response, h, e := p.Try(r, "", "scope")
			if e != nil || !h {
				t.Fatal(e)
			}
			_, e = io.ReadAll(response.Body)
			_ = response.Body.Close()
			var sent *SentError
			if !errors.As(e, &sent) || strings.Contains(e.Error(), "sensitive-upstream-detail") {
				t.Fatal("dispatch semantics or secret leak", e)
			}
			next := request(t, s.URL, validBody, "different")
			resp, h, e := p.Try(next, "", "other-scope")
			if h || e != nil || resp != nil {
				t.Fatal("capability should cool down")
			}
			raw, _ := io.ReadAll(next.Body)
			if string(raw) != validBody {
				t.Fatal("fallback consumed original")
			}
			if generation.Load() != 1 || connections.Load() != 1 {
				t.Fatal("request replayed or cooldown bypassed")
			}
			stats := p.Stats()
			if stats["stream_failure_request_le_64k"] != uint64(1) || stats["unsupported_cached"] != uint64(1) {
				t.Fatal(stats)
			}
			p.mu.Lock()
			for k := range p.negative {
				p.negative[k] = time.Now().Add(-time.Second)
			}
			p.mu.Unlock()
			response, h, e = p.Try(request(t, s.URL, validBody, "s"), "", "scope")
			if e != nil || !h {
				t.Fatal("capability never recovered", e)
			}
			_, e = io.ReadAll(response.Body)
			_ = response.Body.Close()
			if generation.Load() != 2 {
				t.Fatal("expired cooldown blocked probe")
			}
		})
	}
}
