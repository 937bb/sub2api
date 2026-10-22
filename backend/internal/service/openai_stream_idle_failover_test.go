package service

import (
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

func TestOpenAIStreamIdleBeforeSemanticOutputTriggersFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{
		StreamDataIntervalTimeout: 1,
		StreamKeepaliveInterval:   0,
		MaxLineSize:               defaultMaxLineSize,
	}}}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: pr}

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c,
		&Account{ID: 1, Platform: PlatformOpenAI}, time.Now(), "model", "model")
	_ = pw.Close()
	_ = pr.Close()

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusGatewayTimeout, failoverErr.StatusCode)
	require.True(t, failoverErr.SafeToFailoverAfterWrite)
	require.Empty(t, rec.Body.String(), "no terminal error may be committed before account failover")
}

func TestOpenAIStreamIdleAfterSemanticOutputDoesNotReplay(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{
		StreamDataIntervalTimeout: 1,
		StreamKeepaliveInterval:   0,
		MaxLineSize:               defaultMaxLineSize,
	}}}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	pr, pw := io.Pipe()
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: pr}
	wrote := make(chan struct{})
	go func() {
		_, _ = pw.Write([]byte("event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n"))
		close(wrote)
	}()

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c,
		&Account{ID: 1, Platform: PlatformOpenAI}, time.Now(), "model", "model")
	_ = pw.Close()
	_ = pr.Close()
	<-wrote

	var failoverErr *UpstreamFailoverError
	require.Error(t, err)
	require.False(t, errors.As(err, &failoverErr), "semantic output must never be replayed on another account")
	require.Contains(t, err.Error(), "stream data interval timeout")
	require.Contains(t, rec.Body.String(), `"delta":"partial"`)
	require.Contains(t, rec.Body.String(), "stream_timeout")
}
