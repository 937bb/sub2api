package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIRequestTimeoutTriggersFailoverBeforeResponseCommit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	body := []byte(`{"error":{"message":"Request body read timed out"}}`)
	svc := &OpenAIGatewayService{
		cfg: &config.Config{},
		httpUpstream: &httpUpstreamRecorder{resp: &http.Response{
			StatusCode: http.StatusRequestTimeout,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(string(body))),
		}},
	}
	account := &Account{
		ID: 1, Name: "oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "test", "chatgpt_account_id": "test"},
		Status:      StatusActive, Schedulable: true,
	}

	_, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-6.1-sol","input":"hi"}`))

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusRequestTimeout, failoverErr.StatusCode)
	require.Contains(t, string(failoverErr.ResponseBody), "Request body read timed out")
	require.False(t, c.Writer.Written())
	require.False(t, IsResponseCommitted(c))
}

func TestOpenAIPassthroughRequestTimeoutIsFailoverEligible(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	require.True(t, shouldFailoverOpenAIPassthroughResponse(
		account,
		http.StatusRequestTimeout,
		[]byte(`{"error":{"message":"Request body read timed out"}}`),
	))
}
