package service

import (
	"context"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOAuthMappedRetryActualPreOutputStreamFailure(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	calls := 0
	_, err := runOAuthMappedRetry(context.Background(), c, 1, []byte("original"), OAuthRetrySettings{Enabled: true, MaxRetries: 1, StatusCodes: []int{502}}, func(body []byte) (*OpenAIForwardResult, error) {
		calls++
		if calls == 2 {
			c.String(200, "recovered")
			return &OpenAIForwardResult{}, nil
		}
		resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"upstream_error\",\"message\":\"Upstream request failed\"}}}\n\n"))}
		_, streamErr := svc.handleStreamingResponse(c.Request.Context(), resp, c, account, time.Now(), "model", "model")
		require.Error(t, streamErr)
		require.Empty(t, recorder.Body.String())
		return nil, streamErr
	})
	require.NoError(t, err)
	require.Equal(t, 2, calls)
	require.Equal(t, "recovered", recorder.Body.String())
}

func TestOAuthMappedRetryStaged502(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	calls := 0
	_, err := runOAuthMappedRetry(context.Background(), c, 1, []byte("original"), OAuthRetrySettings{Enabled: true, MaxRetries: 1, StatusCodes: []int{502}}, func(body []byte) (*OpenAIForwardResult, error) {
		calls++
		require.Equal(t, "original", string(body))
		if calls == 1 {
			body[0] = 'x'
			c.JSON(502, gin.H{"error": "original failure"})
			MarkResponseCommitted(c)
			require.Empty(t, recorder.Body.String())
			return nil, errors.New("failed")
		}
		require.False(t, IsResponseCommitted(c))
		c.String(200, "success")
		return &OpenAIForwardResult{}, nil
	})
	require.NoError(t, err)
	require.Equal(t, 2, calls)
	require.Equal(t, 200, recorder.Code)
	require.Equal(t, "success", recorder.Body.String())
}

func TestOAuthMappedRetryExhaustion(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	calls := 0
	_, err := runOAuthMappedRetry(context.Background(), c, 1, nil, OAuthRetrySettings{Enabled: true, MaxRetries: 1, StatusCodes: []int{502}}, func([]byte) (*OpenAIForwardResult, error) {
		calls++
		c.String(502, "unchanged error")
		return nil, errors.New("failed")
	})
	require.Error(t, err)
	require.Equal(t, 2, calls)
	require.Equal(t, 502, recorder.Code)
	require.Equal(t, "unchanged error", recorder.Body.String())
}

func TestOAuthMappedRetryDoesNotReplayOutput(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	calls := 0
	_, err := runOAuthMappedRetry(context.Background(), c, 1, nil, OAuthRetrySettings{Enabled: true, MaxRetries: 5, StatusCodes: []int{502}}, func([]byte) (*OpenAIForwardResult, error) {
		calls++
		c.String(200, "partial")
		c.Writer.Flush()
		return nil, errors.New("stream failed")
	})
	require.Error(t, err)
	require.Equal(t, 1, calls)
	require.Equal(t, "partial", recorder.Body.String())
}
