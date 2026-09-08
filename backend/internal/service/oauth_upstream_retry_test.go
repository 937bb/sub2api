package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOAuthHTTPRetry(t *testing.T) {
	for _, status := range []int{429, 502, 503, 504} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			req := httptest.NewRequest("POST", "http://upstream/responses", nil)
			req.Body = io.NopCloser(strings.NewReader("payload"))
			req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("payload")), nil }
			calls := 0
			resp, err, exhausted := retryOAuthHTTP(context.Background(), req, OAuthRetrySettings{Enabled: true, MaxRetries: 1, StatusCodes: []int{status}}, func(r *http.Request) (*http.Response, error) {
				calls++
				data, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				require.Equal(t, "payload", string(data))
				code := status
				if calls == 2 {
					code = 200
				}
				return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("response"))}, nil
			})
			require.NoError(t, err)
			require.False(t, exhausted)
			require.Equal(t, 200, resp.StatusCode)
			require.Equal(t, 2, calls)
		})
	}
}

func TestOAuthHTTPRetryExhaustionPreservesFinalResponse(t *testing.T) {
	req, _ := http.NewRequest("POST", "http://upstream/responses", bytes.NewBufferString("payload"))
	calls := 0
	resp, err, exhausted := retryOAuthHTTP(context.Background(), req, OAuthRetrySettings{Enabled: true, MaxRetries: 2, StatusCodes: []int{503}}, func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 503, Header: http.Header{"Retry-After": []string{"0"}}, Body: io.NopCloser(strings.NewReader("original error"))}, nil
	})
	require.NoError(t, err)
	require.True(t, exhausted)
	require.Equal(t, 3, calls)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	require.Error(t, writeOAuthRetryExhausted(c, resp))
	require.Equal(t, 503, recorder.Code)
	require.Equal(t, "original error", recorder.Body.String())
	require.Equal(t, "0", recorder.Header().Get("Retry-After"))
}

func TestOAuthHTTPRetrySkippedAndCancelled(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		req, _ := http.NewRequest("POST", "http://upstream/responses", bytes.NewBufferString("payload"))
		calls := 0
		_, err, exhausted := retryOAuthHTTP(context.Background(), req, OAuthRetrySettings{Enabled: enabled, MaxRetries: 3, StatusCodes: []int{502}}, func(r *http.Request) (*http.Response, error) { calls++; return &http.Response{StatusCode: 400}, nil })
		require.NoError(t, err)
		require.False(t, exhausted)
		require.Equal(t, 1, calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequest("POST", "http://upstream/responses", bytes.NewBufferString("payload"))
	calls := 0
	_, err, exhausted := retryOAuthHTTP(ctx, req, OAuthRetrySettings{Enabled: true, MaxRetries: 3, StatusCodes: []int{502}}, func(r *http.Request) (*http.Response, error) {
		calls++
		cancel()
		return &http.Response{StatusCode: 502, Header: make(http.Header)}, nil
	})
	require.NoError(t, err)
	require.True(t, exhausted)
	require.Equal(t, 1, calls)
	_, err, exhausted = retryOAuthHTTP(context.Background(), req, OAuthRetrySettings{Enabled: true, MaxRetries: 3, StatusCodes: []int{502}}, func(r *http.Request) (*http.Response, error) { return nil, errors.New("transport") })
	require.Error(t, err)
	require.False(t, exhausted)
}

func TestOAuthRetryDelayAndValidation(t *testing.T) {
	for i, want := range []time.Duration{100, 200, 400, 800, 800} {
		require.Equal(t, want*time.Millisecond, oauthRetryDelay(i, "", time.Now()))
	}
	require.Equal(t, 800*time.Millisecond, oauthRetryDelay(0, "120", time.Now()))
	v := DefaultOAuthRetrySettings()
	require.NoError(t, ValidateOAuthRetrySettings(v))
	v.MaxRetries = 11
	require.Error(t, ValidateOAuthRetrySettings(v))
	v.MaxRetries = 3
	v.StatusCodes = []int{502, 502}
	require.Error(t, ValidateOAuthRetrySettings(v))
	v.StatusCodes = []int{200}
	require.Error(t, ValidateOAuthRetrySettings(v))
}
