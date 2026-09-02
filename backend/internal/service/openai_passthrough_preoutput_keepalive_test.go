package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// These tests ensure the downstream connection does not remain silent before
// the first visible output.
//
// The passthrough path buffers response.created and response.in_progress in
// pendingLines, so no bytes or response headers reach the downstream client
// before the first visible output. A long reasoning phase can therefore hit an
// intermediary idle timeout. The Forward path already prevents this with
// keepalives; the passthrough path must provide the same behavior.

func newPassthroughKeepaliveTestContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	// Do not call MarkOpenAICompactClientStream: ordinary /v1/responses
	// passthrough requests do not carry the compact marker.
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	// Production requests arrive through the reverse proxy. The official header
	// policy only emits the nginx-specific X-Accel-Buffering header in this case.
	c.Request.Header.Set("X-Forwarded-For", "192.0.2.1")
	return c, rec
}

// startOpenAISSEKeepalive must work without a compact marker; otherwise an
// ordinary passthrough request remains silent.
func TestStartOpenAISSEKeepalive_WorksWithoutCompactMarker(t *testing.T) {
	c, rec := newPassthroughKeepaliveTestContext(t)

	// The public compact-aware entry point must remain a no-op here.
	stop := StartOpenAICompactSSEKeepalive(c, keepaliveTestInterval)
	waitForKeepaliveBeats()
	stop()
	require.Zero(t, rec.Body.Len(), "无 compact 标记时 StartOpenAICompactSSEKeepalive 应当 no-op")

	// The internal entry point intentionally bypasses the marker check.
	c, rec = newPassthroughKeepaliveTestContext(t)
	stop = startOpenAISSEKeepalive(c, keepaliveTestInterval)
	defer stop()
	waitForKeepaliveBeats()

	require.True(t, StopOpenAICompactSSEKeepaliveCommitted(c), "心跳应当提交响应头")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
	require.Equal(t, "no", rec.Header().Get("X-Accel-Buffering"))
	require.Contains(t, rec.Body.String(), ": keepalive\n\n")
}

// Keepalive bytes must not count as semantic client output. The passthrough
// pre-output failover relies on this invariant to retry upstream 429/5xx errors.
func TestPassthroughKeepaliveDoesNotBlockPreOutputFailover(t *testing.T) {
	c, rec := newPassthroughKeepaliveTestContext(t)
	stop := startOpenAISSEKeepalive(c, keepaliveTestInterval)
	defer stop()
	waitForKeepaliveBeats()
	require.True(t, StopOpenAICompactSSEKeepaliveCommitted(c))
	require.NotZero(t, rec.Body.Len(), "前提:心跳确实写出了字节")

	// Keepalive-only output is not semantic client output.
	require.False(t, openAIStreamClientOutputStarted(c, false),
		"心跳字节不构成语义输出,pre-output failover 必须仍然可用")

	// A real event changes the output state.
	_, err := c.Writer.Write([]byte("data: {\"type\":\"response.output_text.delta\"}\n\n"))
	require.NoError(t, err)
	require.True(t, openAIStreamClientOutputStarted(c, false),
		"真实语义输出之后应当判定为已输出")
}

// No keepalive bytes may be written after the main loop takes over the writer.
func TestPassthroughKeepaliveStopsBeforeHandingOverWriter(t *testing.T) {
	c, rec := newPassthroughKeepaliveTestContext(t)
	stop := startOpenAISSEKeepalive(c, keepaliveTestInterval)
	waitForKeepaliveBeats()
	stop()

	before := rec.Body.String()
	waitForKeepaliveBeats()
	require.Equal(t, before, rec.Body.String(), "停拍后不应再有字节写出")

	// Main-loop output must not be interleaved with stopped keepalives.
	_, err := c.Writer.Write([]byte("data: real\n\n"))
	require.NoError(t, err)
	waitForKeepaliveBeats()
	require.True(t, strings.HasSuffix(rec.Body.String(), "data: real\n\n"),
		"停拍后写入应当是响应体的最后一段")
}

// A disabled interval must leave the writer untouched.
func TestPassthroughKeepaliveDisabledKeepsWriterUntouched(t *testing.T) {
	c, rec := newPassthroughKeepaliveTestContext(t)
	stop := startOpenAISSEKeepalive(c, 0)
	waitForKeepaliveBeats()
	stop()
	require.Zero(t, rec.Body.Len())
	require.False(t, StopOpenAICompactSSEKeepaliveCommitted(c))
}
