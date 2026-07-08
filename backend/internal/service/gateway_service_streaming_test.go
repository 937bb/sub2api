package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type upstreamContextTestKey string

func TestGatewayService_StreamingReusesScannerBufferAndStillParsesUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			MaxLineSize:               defaultMaxLineSize,
		},
	}

	svc := &GatewayService{
		cfg:              cfg,
		rateLimitService: &RateLimitService{},
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: pr}

	go func() {
		defer func() { _ = pw.Close() }()
		// Minimal SSE event to trigger parseSSEUsage
		_, _ = pw.Write([]byte("data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":3}}}\n\n"))
		_, _ = pw.Write([]byte("data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":7}}\n\n"))
		_, _ = pw.Write([]byte("data: [DONE]\n\n"))
	}()

	result, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "model", "model", false)
	_ = pr.Close()
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.usage)
	require.Equal(t, 3, result.usage.InputTokens)
	require.Equal(t, 7, result.usage.OutputTokens)
}

func TestDetachUpstreamContextIgnoresClientCancel(t *testing.T) {
	parent, cancel := context.WithCancel(context.WithValue(context.Background(), upstreamContextTestKey("test-key"), "test-value"))
	upstreamCtx, release := detachUpstreamContext(parent)
	defer release()

	cancel()

	require.NoError(t, upstreamCtx.Err())
	require.Equal(t, "test-value", upstreamCtx.Value(upstreamContextTestKey("test-key")))
}

func TestGatewayService_ClaudeCodeKeepaliveUsesNoopContentDeltaInsideBlock(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newGatewayServiceForKeepaliveTest()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	setClaudeCodeKeepaliveHeaders(c.Request)

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: pr}

	go func() {
		defer func() { _ = pw.Close() }()
		_, _ = pw.Write([]byte("event: content_block_start\n"))
		_, _ = pw.Write([]byte("data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n"))
		time.Sleep(1200 * time.Millisecond)
		_, _ = pw.Write([]byte("event: content_block_delta\n"))
		_, _ = pw.Write([]byte("data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}\n\n"))
		_, _ = pw.Write([]byte("event: content_block_stop\n"))
		_, _ = pw.Write([]byte("data: {\"type\":\"content_block_stop\",\"index\":0}\n\n"))
		_, _ = pw.Write([]byte("event: message_delta\n"))
		_, _ = pw.Write([]byte("data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":1}}\n\n"))
		_, _ = pw.Write([]byte("event: message_stop\n"))
		_, _ = pw.Write([]byte("data: {\"type\":\"message_stop\"}\n\n"))
	}()

	result, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "model", "model", false)
	_ = pr.Close()
	require.NoError(t, err)
	require.NotNil(t, result)
	body := rec.Body.String()
	require.Contains(t, body, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":""}}`)
	require.NotContains(t, body, "event: ping")
	require.Less(t, strings.Index(body, `"text":""`), strings.Index(body, `"text":"hello"`))
}

func TestGatewayService_KeepalivePingWhenClaudeCodeSignalIsOnlyUserAgent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newGatewayServiceForKeepaliveTest()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("User-Agent", "claude-cli/2.1.193 (external, cli)")

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: pr}

	go writeDelayedTextBlockForKeepaliveTest(pw)

	result, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "model", "model", false)
	_ = pr.Close()
	require.NoError(t, err)
	require.NotNil(t, result)
	body := rec.Body.String()
	require.Contains(t, body, "event: ping")
	require.NotContains(t, body, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":""}}`)
}

func TestGatewayService_KeepalivePingForOlderClaudeCodeContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newGatewayServiceForKeepaliveTest()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	setClaudeCodeKeepaliveHeaders(c.Request)
	c.Request.Header.Set("User-Agent", "claude-cli/2.1.192 (external, cli)")

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: pr}

	go writeDelayedTextBlockForKeepaliveTest(pw)

	result, err := svc.handleStreamingResponse(SetClaudeCodeClient(context.Background(), true), resp, c, &Account{ID: 1}, time.Now(), "model", "model", false)
	_ = pr.Close()
	require.NoError(t, err)
	require.NotNil(t, result)
	body := rec.Body.String()
	require.Contains(t, body, "event: ping")
	require.NotContains(t, body, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":""}}`)
}

func TestGatewayService_KeepalivePingForMimicClaudeCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newGatewayServiceForKeepaliveTest()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	setClaudeCodeKeepaliveHeaders(c.Request)

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: pr}

	go writeDelayedTextBlockForKeepaliveTest(pw)

	result, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "model", "model", true)
	_ = pr.Close()
	require.NoError(t, err)
	require.NotNil(t, result)
	body := rec.Body.String()
	require.Contains(t, body, "event: ping")
	require.NotContains(t, body, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":""}}`)
}

func TestGatewayService_KeepaliveDeltaBeforeStartDoesNotEnableNoop(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newGatewayServiceForKeepaliveTest()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	setClaudeCodeKeepaliveHeaders(c.Request)

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: pr}

	go func() {
		defer func() { _ = pw.Close() }()
		_, _ = pw.Write([]byte("event: content_block_delta\n"))
		_, _ = pw.Write([]byte("data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"early\"}}\n\n"))
		time.Sleep(2200 * time.Millisecond)
		_, _ = pw.Write([]byte("event: message_delta\n"))
		_, _ = pw.Write([]byte("data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":1}}\n\n"))
		_, _ = pw.Write([]byte("event: message_stop\n"))
		_, _ = pw.Write([]byte("data: {\"type\":\"message_stop\"}\n\n"))
	}()

	result, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "model", "model", false)
	_ = pr.Close()
	require.NoError(t, err)
	require.NotNil(t, result)
	body := rec.Body.String()
	require.Contains(t, body, "event: ping")
	require.NotContains(t, body, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":""}}`)
}

func newGatewayServiceForKeepaliveTest() *GatewayService {
	return &GatewayService{
		cfg: &config.Config{Gateway: config.GatewayConfig{
			StreamDataIntervalTimeout: 0,
			StreamKeepaliveInterval:   1,
			MaxLineSize:               defaultMaxLineSize,
		}},
		rateLimitService: &RateLimitService{},
	}
}

func setClaudeCodeKeepaliveHeaders(r *http.Request) {
	r.Header.Set("User-Agent", "claude-cli/2.1.193 (external, cli)")
	r.Header.Set("X-App", "claude-code")
	r.Header.Set("anthropic-beta", "claude-code-20250219")
	r.Header.Set("anthropic-version", "2023-06-01")
	r.Header.Set("X-Claude-Code-Session-Id", "session-123")
}

func writeDelayedTextBlockForKeepaliveTest(pw *io.PipeWriter) {
	defer func() { _ = pw.Close() }()
	_, _ = pw.Write([]byte("event: content_block_start\n"))
	_, _ = pw.Write([]byte("data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n"))
	time.Sleep(1200 * time.Millisecond)
	_, _ = pw.Write([]byte("event: content_block_stop\n"))
	_, _ = pw.Write([]byte("data: {\"type\":\"content_block_stop\",\"index\":0}\n\n"))
	_, _ = pw.Write([]byte("event: message_delta\n"))
	_, _ = pw.Write([]byte("data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":1}}\n\n"))
	_, _ = pw.Write([]byte("event: message_stop\n"))
	_, _ = pw.Write([]byte("data: {\"type\":\"message_stop\"}\n\n"))
}
