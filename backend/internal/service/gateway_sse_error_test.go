//go:build unit

package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newSSEErrorTestGatewayService() *GatewayService {
	return &GatewayService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				StreamDataIntervalTimeout: 0,
				MaxLineSize:               defaultMaxLineSize,
			},
		},
		rateLimitService: &RateLimitService{},
	}
}

func TestHandleStreamingResponse_SSEErrorEvent_ReturnsTypedErrorWithRawData(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newSSEErrorTestGatewayService()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	const errorJSON = `{"type":"error","error":{"type":"overloaded_error","message":"Anthropic upstream is overloaded"}}`

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: pr}
	go func() {
		defer func() { _ = pw.Close() }()
		_, _ = pw.Write([]byte("event: error\ndata: " + errorJSON + "\n\n"))
	}()

	result, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "model", "model", false)
	_ = pr.Close()

	require.Error(t, err)
	require.Nil(t, result)
	var sseErr *sseStreamErrorEventError
	require.True(t, errors.As(err, &sseErr))
	require.Equal(t, errorJSON, sseErr.RawData)
	require.Equal(t, "have error in stream", err.Error())
	require.Equal(t, "Anthropic upstream is overloaded", ExtractUpstreamErrorMessage([]byte(sseErr.RawData)))
}

func TestHandleStreamingResponse_SSEErrorEvent_NonJSONDataLine(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newSSEErrorTestGatewayService()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: pr}
	go func() {
		defer func() { _ = pw.Close() }()
		_, _ = pw.Write([]byte("event: error\ndata: not-a-json-payload\n\n"))
	}()

	_, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "model", "model", false)
	_ = pr.Close()

	require.Error(t, err)
	var sseErr *sseStreamErrorEventError
	require.True(t, errors.As(err, &sseErr))
	require.Equal(t, "not-a-json-payload", sseErr.RawData)
	require.NotPanics(t, func() {
		_ = ExtractUpstreamErrorMessage([]byte(sseErr.RawData))
	})
}
