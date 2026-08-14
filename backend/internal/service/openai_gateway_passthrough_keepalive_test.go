package service

import (
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

func TestOpenAIStreamingPassthroughSendsKeepaliveBeforeFirstOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{
		MaxLineSize:             defaultMaxLineSize,
		StreamKeepaliveInterval: 1,
	}}}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	reader, writer := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Body: reader, Header: http.Header{}}
	resultCh := make(chan error, 1)
	go func() {
		_, err := svc.handleStreamingResponsePassthrough(
			c.Request.Context(), resp, c, &Account{ID: 11, Platform: PlatformOpenAI}, time.Now(), "gpt-test", "gpt-test",
		)
		resultCh <- err
	}()

	time.Sleep(1200 * time.Millisecond)
	_, err := writer.Write([]byte(strings.Join([]string{
		`data: {"type":"response.output_text.delta","delta":"ok"}`,
		"",
		`data: {"type":"response.completed","response":{"id":"resp_keepalive","status":"completed","output":[{"type":"message"}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`,
		"",
	}, "\n")))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	require.NoError(t, <-resultCh)
	require.True(t, OpenAIStreamHeartbeatPresent(c))
	require.True(t, strings.HasPrefix(rec.Body.String(), ":\n\n"), rec.Body.String())
	require.Contains(t, rec.Body.String(), "response.completed")
}

func TestOpenAIStreamingPassthroughHeartbeatDoesNotBlockPreOutputFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{
		MaxLineSize:             defaultMaxLineSize,
		StreamKeepaliveInterval: 1,
	}}}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	reader, writer := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Body: reader, Header: http.Header{}}
	resultCh := make(chan error, 1)
	go func() {
		_, err := svc.handleStreamingResponsePassthrough(
			c.Request.Context(), resp, c, &Account{ID: 14, Platform: PlatformOpenAI}, time.Now(), "gpt-test", "gpt-test",
		)
		resultCh <- err
	}()

	time.Sleep(1200 * time.Millisecond)
	_, err := writer.Write([]byte(strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_retry"}}`,
		"",
		`data: {"type":"response.failed","response":{"id":"resp_retry","error":{"message":"upstream processing failed"}}}`,
		"",
	}, "\n")))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	err = <-resultCh
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, ":\n\n", rec.Body.String())
	require.Equal(t, -1, OpenAIStreamAdjustedWrittenSize(c))
}

func TestOpenAIStreamingPassthroughNormalizesDoneToTypedTerminal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("data: [DONE]\n\n")),
		Header:     http.Header{},
	}

	_, err := svc.handleStreamingResponsePassthrough(
		c.Request.Context(), resp, c, &Account{ID: 12, Platform: PlatformOpenAI}, time.Now(), "gpt-test", "gpt-test",
	)
	require.NoError(t, err)
	require.Contains(t, rec.Body.String(), `"type":"response.completed"`)
	require.NotContains(t, rec.Body.String(), "[DONE]")
}

func TestOpenAIStreamingPassthroughIdleTimeoutCanFailOverBeforeOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{
		MaxLineSize:               defaultMaxLineSize,
		StreamDataIntervalTimeout: 1,
	}}}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	reader, writer := io.Pipe()
	defer writer.Close()
	resp := &http.Response{StatusCode: http.StatusOK, Body: reader, Header: http.Header{}}

	_, err := svc.handleStreamingResponsePassthrough(
		c.Request.Context(), resp, c, &Account{ID: 13, Platform: PlatformOpenAI}, time.Now(), "gpt-test", "gpt-test",
	)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Contains(t, string(failoverErr.ResponseBody), "stream data interval timeout")
	require.False(t, c.Writer.Written())
}

func TestOpenAIStreamAdjustedWrittenSizeExcludesHeartbeat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	n, err := c.Writer.Write([]byte(":\n\n"))
	require.NoError(t, err)
	recordOpenAIStreamHeartbeatBytes(c, n)
	require.Equal(t, -1, OpenAIStreamAdjustedWrittenSize(c))

	_, err = c.Writer.Write([]byte("data: semantic\n\n"))
	require.NoError(t, err)
	require.Greater(t, OpenAIStreamAdjustedWrittenSize(c), 0)
	require.True(t, OpenAIStreamHeartbeatPresent(c))
}
