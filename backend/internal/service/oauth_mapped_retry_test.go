package service

import (
	"context"
	"errors"
	"fmt"
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

func TestOAuthMappedRetryPreservesAccountPolicy(t *testing.T) {
	accessFailure := newOpenAIUpstreamFailoverError(403, http.Header{}, []byte(`{"error":{"code":"deactivated_workspace"}}`), "workspace deactivated", false)
	for _, tt := range []struct {
		name string
		err  *UpstreamFailoverError
	}{
		{"access_failure", accessFailure},
		{"credential_failure", &UpstreamFailoverError{StatusCode: 401, Stage: GatewayFailureStageAccountAuth, NextAccountAction: NextAccountRetry}},
		{"same_account_disallowed", &UpstreamFailoverError{StatusCode: 502, NextAccountAction: NextAccountRetry}},
		{"request_must_stop", &UpstreamFailoverError{StatusCode: 502, RetryableOnSameAccount: true, NextAccountAction: NextAccountStop}},
		{"bounded_429_probe", &UpstreamFailoverError{StatusCode: 429, RetryableOnSameAccount: true, SameAccountRetryMax: 1, SameAccountRetryDeadline: time.Now().Add(time.Minute)}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
			calls := 0
			_, err := runOAuthMappedRetry(context.Background(), c, 1, nil, OAuthRetrySettings{Enabled: true, MaxRetries: 7, StatusCodes: []int{429, 502, 503}}, func([]byte) (*OpenAIForwardResult, error) {
				calls++
				return nil, tt.err
			})
			require.Same(t, tt.err, err, "the scheduler must receive the original policy and retry limits")
			require.Equal(t, 1, calls)
			require.False(t, c.Writer.Written())
		})
	}
}

func TestOAuthMappedRetryExhaustionAllowsNextAccount(t *testing.T) {
	for _, limit := range []int{0, 1} {
		for _, action := range []NextAccountAction{NextAccountLegacyRetry, NextAccountRetry} {
			t.Run(fmt.Sprintf("limit_%d_action_%d", limit, action), func(t *testing.T) {
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
				originalWriter := c.Writer
				c.Header("X-Request-ID", "request-original")
				c.Set(OpsSkipPassthroughKey, false)
				original := &UpstreamFailoverError{StatusCode: 502, RetryableOnSameAccount: true, NextAccountAction: action}
				calls := 0
				_, err := runOAuthMappedRetry(context.Background(), c, 1, nil, OAuthRetrySettings{Enabled: true, MaxRetries: limit, StatusCodes: []int{502}}, func([]byte) (*OpenAIForwardResult, error) {
					calls++
					c.Header("X-Failed-Attempt", "discard")
					c.JSON(502, gin.H{"error": "transient"})
					MarkResponseCommitted(c)
					c.Set(OpsSkipPassthroughKey, true)
					return nil, fmt.Errorf("wrapped: %w", original)
				})
				var failover *UpstreamFailoverError
				require.ErrorAs(t, err, &failover)
				require.Equal(t, limit+1, calls)
				require.Equal(t, action, failover.NextAccountAction)
				require.True(t, failover.ShouldRetryNextAccount())
				require.False(t, failover.RetryableOnSameAccount, "the handler must not restart the same-account budget")
				require.True(t, original.RetryableOnSameAccount, "do not mutate the original error")
				require.Same(t, originalWriter, c.Writer)
				require.False(t, c.Writer.Written())
				require.False(t, IsResponseCommitted(c))
				require.False(t, c.GetBool(OpsSkipPassthroughKey))
				require.False(t, oauthMappedRetryActive(c))
				require.Empty(t, recorder.Body.String())
				require.Empty(t, c.Writer.Header().Get("X-Failed-Attempt"))
				require.Equal(t, "request-original", c.Writer.Header().Get("X-Request-ID"))
				c.String(200, "next account recovered")
				require.Equal(t, 200, recorder.Code)
				require.Equal(t, "next account recovered", recorder.Body.String())
			})
		}
	}
}

func TestOAuthMappedRetryDoesNotReplayResultOrCancellation(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancelled_%v", cancelled), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest("POST", "/v1/responses", nil).WithContext(ctx)
			billable := &OpenAIForwardResult{}
			if cancelled {
				billable = nil
				cancel()
			}
			calls := 0
			result, err := runOAuthMappedRetry(ctx, c, 1, nil, OAuthRetrySettings{Enabled: true, MaxRetries: 7, StatusCodes: []int{502}}, func([]byte) (*OpenAIForwardResult, error) {
				calls++
				return billable, errors.New("stream failed")
			})
			require.Error(t, err)
			require.Equal(t, 1, calls)
			require.Equal(t, billable, result)
		})
	}
}

func TestOAuthMappedStreamPreservesCredentialClassification(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(oauthMappedRetryKey, true)
	c.Set("oauth_mapped_retry_settings", OAuthRetrySettings{Enabled: true, MaxRetries: 7, StatusCodes: []int{502}})
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	payload := []byte(`{"type":"response.failed","response":{"error":{"code":"deactivated_workspace","message":"workspace deactivated"}}}`)
	require.False(t, svc.shouldRetryOAuthMappedStream(c, account, payload, "workspace deactivated"), "credential errors must reach normal account-health handling")
}
