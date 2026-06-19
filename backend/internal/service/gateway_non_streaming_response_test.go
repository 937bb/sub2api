//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestHandleNonStreamingResponse_NonJSON2xxTriggersFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	body := []byte("(upstream request failed)")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/plain"},
			"X-Request-Id": []string{"rid-invalid-json"},
		},
		Body: io.NopCloser(bytes.NewReader(body)),
	}
	svc := &GatewayService{
		cfg:              &config.Config{},
		rateLimitService: &RateLimitService{},
	}

	usage, err := svc.handleNonStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, "claude-sonnet-4-6", "claude-sonnet-4-6")

	require.Nil(t, usage)
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr))
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.Equal(t, body, failoverErr.ResponseBody)
	require.Equal(t, "rid-invalid-json", failoverErr.ResponseHeaders.Get("x-request-id"))
	require.False(t, c.Writer.Written())
}

func TestHandleNonStreamingResponse_ValidJSONUnchanged(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	body := []byte(`{"id":"msg_1","type":"message","usage":{"input_tokens":12,"output_tokens":7}}`)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
	svc := &GatewayService{
		cfg:              &config.Config{},
		rateLimitService: &RateLimitService{},
	}

	usage, err := svc.handleNonStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, "claude-sonnet-4-6", "claude-sonnet-4-6")

	require.NoError(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 12, usage.InputTokens)
	require.Equal(t, 7, usage.OutputTokens)
	require.JSONEq(t, string(body), rec.Body.String())
}

func TestHandleNonStreamingResponseAnthropicAPIKeyPassthrough_NonJSON2xxTriggersFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	body := []byte("(upstream request failed)")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/plain"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
	svc := &GatewayService{cfg: &config.Config{}}

	usage, err := svc.handleNonStreamingResponseAnthropicAPIKeyPassthrough(context.Background(), resp, c, &Account{ID: 2})

	require.Nil(t, usage)
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr))
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.Equal(t, body, failoverErr.ResponseBody)
	require.False(t, c.Writer.Written())
}
