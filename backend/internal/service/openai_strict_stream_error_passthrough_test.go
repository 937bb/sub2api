//go:build unit

package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func strictOpenAIAPIKeyPassthroughTestAccount() *Account {
	return &Account{
		ID:       937,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"base_url": "https://newapi.example.com/v1",
		},
		Extra: map[string]any{"openai_passthrough": true},
	}
}

func TestStrictOpenAIAPIKeyStreamRateLimitPreservesUpstreamError(t *testing.T) {
	payload := []byte(`{"type":"error","error":{"type":"too_many_requests","code":"rate_limit_exceeded","message":"Your requests to gpt-6.1-sol in eastus have exceeded token rate limit.","param":null}}`)
	message := extractOpenAISSEErrorMessage(payload)

	failoverErr := (&OpenAIGatewayService{}).newOpenAIStreamFailoverErrorWithModel(
		nil, strictOpenAIAPIKeyPassthroughTestAccount(), true, "", payload, message, "gpt-6.1-sol",
	)

	require.Equal(t, http.StatusTooManyRequests, failoverErr.StatusCode)
	require.True(t, failoverErr.StrictOpenAIAPIKeyPassthrough)
	require.Equal(t, "too_many_requests", gjson.GetBytes(failoverErr.ResponseBody, "error.type").String())
	require.Equal(t, "rate_limit_exceeded", gjson.GetBytes(failoverErr.ResponseBody, "error.code").String())
	require.Equal(t, message, gjson.GetBytes(failoverErr.ResponseBody, "error.message").String())
}

func TestStrictOpenAIAPIKeyInvalidEncryptedContentIsDeterministic400(t *testing.T) {
	payload := []byte(`{"type":"response.failed","response":{"error":{"type":"invalid_request_error","code":"invalid_encrypted_content","message":"The encrypted content could not be decrypted or parsed.","param":"input[4]"}}}`)
	message := extractOpenAISSEErrorMessage(payload)

	require.Equal(t, http.StatusBadRequest, openAIStreamFailedEventSemanticStatus(payload, message))
	require.Equal(t, http.StatusBadRequest, openAIStreamFailureStatus(payload, message))
	require.False(t, openAIStreamFailedEventShouldFailover(payload, message))

	failoverErr := (&OpenAIGatewayService{}).newOpenAIStreamFailoverErrorWithModel(
		nil, strictOpenAIAPIKeyPassthroughTestAccount(), true, "", payload, message, "gpt-6.1-sol",
	)

	require.Equal(t, http.StatusBadRequest, failoverErr.StatusCode)
	require.True(t, failoverErr.StrictOpenAIAPIKeyPassthrough)
	require.Equal(t, "invalid_request_error", gjson.GetBytes(failoverErr.ResponseBody, "error.type").String())
	require.Equal(t, "invalid_encrypted_content", gjson.GetBytes(failoverErr.ResponseBody, "error.code").String())
	require.Equal(t, "input[4]", gjson.GetBytes(failoverErr.ResponseBody, "error.param").String())
}

func TestStrictOpenAIAPIKeyStreamErrorClassification(t *testing.T) {
	for _, test := range []struct {
		name    string
		payload string
		status  int
		retry   bool
	}{
		{
			name:    "rate limit type without code",
			payload: `{"type":"error","error":{"type":"too_many_requests","message":"Too many requests"}}`,
			status:  http.StatusTooManyRequests,
			retry:   true,
		},
		{
			name:    "encrypted error with retry wording",
			payload: `{"type":"error","error":{"code":"invalid_encrypted_content","message":"The encrypted content could not be verified. Please retry."}}`,
			status:  http.StatusBadRequest,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			payload := []byte(test.payload)
			message := extractOpenAISSEErrorMessage(payload)
			require.Equal(t, test.status, openAIStreamFailureStatus(payload, message))
			require.Equal(t, test.retry, openAIStreamFailedEventShouldFailover(payload, message))
			require.Equal(t, test.retry, openAIStreamErrorEventShouldFailover(payload, message))
		})
	}
}

func TestStrictOpenAIAPIKeyNestedErrorPreservesEmptyOrNullParam(t *testing.T) {
	for _, param := range []string{`""`, `null`} {
		t.Run(param, func(t *testing.T) {
			payload := []byte(`{"type":"response.failed","response":{"error":{"code":"rate_limit_exceeded","message":"Token rate limit exceeded","param":` + param + `}}}`)
			failoverErr := (&OpenAIGatewayService{}).newOpenAIStreamFailoverErrorWithModel(
				nil, strictOpenAIAPIKeyPassthroughTestAccount(), true, "", payload,
				extractOpenAISSEErrorMessage(payload), "gpt-6.1-sol")

			require.True(t, failoverErr.StrictOpenAIAPIKeyPassthrough)
			require.True(t, gjson.GetBytes(failoverErr.ResponseBody, "error.param").Exists())
			require.Equal(t, param, gjson.GetBytes(failoverErr.ResponseBody, "error.param").Raw)
		})
	}
}

func TestStrictOpenAIAPIKeyStreamReaderPreservesRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, eventType := range []string{"error", "response.failed"} {
		t.Run(eventType, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			errorJSON := `{"type":"too_many_requests","code":"rate_limit_exceeded","headers":{"x-ms-fe-error":"true"},"message":"Your requests to gpt-6.1-sol in eastus have exceeded token rate limit.","param":null}`
			payload := `{"type":"error","error":` + errorJSON + `,"sequence_number":1}`
			if eventType == "response.failed" {
				payload = `{"type":"response.failed","response":{"id":"resp_rate_limit","status":"failed","error":` + errorJSON + `}}`
			}
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": {"text/event-stream"},
					"Retry-After":  {"2"},
				},
				Body: io.NopCloser(strings.NewReader("event: " + eventType + "\ndata: " + payload + "\n\n")),
			}
			t.Cleanup(func() { _ = resp.Body.Close() })
			svc := &OpenAIGatewayService{cfg: &config.Config{}}

			_, err := svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c,
				strictOpenAIAPIKeyPassthroughTestAccount(), time.Now(), "gpt-6.1-sol", "gpt-6.1-sol")

			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.Equal(t, http.StatusTooManyRequests, failoverErr.StatusCode)
			require.True(t, failoverErr.StrictOpenAIAPIKeyPassthrough)
			require.Equal(t, "too_many_requests", gjson.GetBytes(failoverErr.ResponseBody, "error.type").String())
			require.Equal(t, "rate_limit_exceeded", gjson.GetBytes(failoverErr.ResponseBody, "error.code").String())
			require.Equal(t, "2", failoverErr.ResponseHeaders.Get("Retry-After"))
			require.False(t, c.Writer.Written())
			require.Empty(t, recorder.Body.String())
		})
	}
}

func TestStrictOpenAIAPIKeyStreamReaderRateLimitAfterOutputDoesNotReplay(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, eventType := range []string{"error", "response.failed"} {
		t.Run(eventType, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			errorJSON := `{"type":"too_many_requests","code":"rate_limit_exceeded","message":"Your requests to gpt-6.1-sol in eastus have exceeded token rate limit.","param":null}`
			payload := `{"type":"error","error":` + errorJSON + `,"sequence_number":2}`
			if eventType == "response.failed" {
				payload = `{"type":"response.failed","response":{"id":"resp_rate_limit","status":"failed","error":` + errorJSON + `}}`
			}
			firstOutput := "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n"
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"text/event-stream"}},
				Body:       io.NopCloser(strings.NewReader(firstOutput + "event: " + eventType + "\ndata: " + payload + "\n\n")),
			}
			t.Cleanup(func() { _ = resp.Body.Close() })
			svc := &OpenAIGatewayService{cfg: &config.Config{}}

			_, err := svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c,
				strictOpenAIAPIKeyPassthroughTestAccount(), time.Now(), "gpt-6.1-sol", "gpt-6.1-sol")

			require.Error(t, err)
			var failoverErr *UpstreamFailoverError
			require.NotErrorAs(t, err, &failoverErr)
			require.Equal(t, http.StatusOK, recorder.Code)
			require.Equal(t, 1, strings.Count(recorder.Body.String(), firstOutput))
			require.Equal(t, 1, strings.Count(recorder.Body.String(), "event: response.failed\n"))
			for _, line := range strings.Split(recorder.Body.String(), "\n") {
				data, ok := extractOpenAISSEDataLine(line)
				if ok && gjson.Get(data, "type").String() == "response.failed" {
					require.JSONEq(t, errorJSON, gjson.Get(data, "response.error").Raw)
				}
			}
		})
	}
}

func TestStrictOpenAIAPIKeyStreamReaderPreservesNonRetryableEncryptedError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, eventType := range []string{"error", "response.failed"} {
		for _, param := range []string{`""`, `null`, `"input[4]"`} {
			t.Run(eventType+"/"+param, func(t *testing.T) {
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
				errorJSON := `{"type":"invalid_request_error","code":"invalid_encrypted_content","message":"The encrypted content could not be decrypted or parsed. Please retry.","param":` + param + `}`
				payload := `{"type":"error","error":` + errorJSON + `,"sequence_number":1}`
				if eventType == "response.failed" {
					payload = `{"type":"response.failed","response":{"id":"resp_encrypted","status":"failed","error":` + errorJSON + `}}`
				}
				resp := &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": {"text/event-stream"}},
					Body:       io.NopCloser(strings.NewReader("event: " + eventType + "\ndata: " + payload + "\n\n")),
				}
				t.Cleanup(func() { _ = resp.Body.Close() })
				svc := &OpenAIGatewayService{cfg: &config.Config{}}

				_, err := svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c,
					strictOpenAIAPIKeyPassthroughTestAccount(), time.Now(), "gpt-6.1-sol", "gpt-6.1-sol")

				require.Error(t, err)
				var failoverErr *UpstreamFailoverError
				require.NotErrorAs(t, err, &failoverErr)
				require.Equal(t, http.StatusOK, recorder.Code)
				require.Contains(t, recorder.Body.String(), "event: response.failed\n")
				for _, line := range strings.Split(recorder.Body.String(), "\n") {
					data, ok := extractOpenAISSEDataLine(line)
					if !ok || gjson.Get(data, "type").String() != "response.failed" {
						continue
					}
					require.JSONEq(t, errorJSON, gjson.Get(data, "response.error").Raw)
				}
			})
		}
	}
}

func TestStrictOpenAIAPIKeyHTTPEncryptedErrorDoesNotUsePoolRetry(t *testing.T) {
	account := strictOpenAIAPIKeyPassthroughTestAccount()
	account.Credentials["pool_mode"] = true
	account.Credentials["pool_mode_retry_status_codes"] = []any{float64(http.StatusBadRequest)}
	body := []byte(`{"error":{"code":"invalid_encrypted_content","type":"invalid_request_error","message":"The encrypted content could not be verified.","param":""}}`)

	require.False(t, shouldFailoverOpenAIPassthroughResponse(account, http.StatusBadRequest, body))
}

func TestStrictOpenAIAPIKeyHTTPAuthErrorIsPreservedAfterFailover(t *testing.T) {
	for _, statusCode := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(statusCode), func(t *testing.T) {
			account := strictOpenAIAPIKeyPassthroughTestAccount()
			body := []byte(`{"error":{"type":"authentication_error","code":"InvalidAuthenticationToken","message":"Azure rejected the upstream key","param":null,"innererror":{"code":"ExpiredAuthenticationToken"},"status":` + strconv.Itoa(statusCode) + `},"request_id":"az-request-1"}`)
			failoverErr := newOpenAIUpstreamFailoverError(statusCode, http.Header{"Content-Type": {"application/json"}}, body, "Azure rejected the upstream key", false)

			require.True(t, preserveStrictOpenAIAPIKeyHTTPFailoverError(account, failoverErr, body))
			require.True(t, failoverErr.StrictOpenAIAPIKeyPassthrough)
			require.JSONEq(t, string(body), string(failoverErr.ResponseBody))
		})
	}
}

func TestStrictOpenAIAPIKeyStreamCredentialErrorsKeepManagedEnvelope(t *testing.T) {
	for _, code := range []string{"invalid_api_key", "permission_denied"} {
		t.Run(code, func(t *testing.T) {
			payload := []byte(`{"type":"error","error":{"code":"` + code + `","message":"upstream credential unavailable"}}`)
			failoverErr := (&OpenAIGatewayService{}).newOpenAIStreamFailoverErrorWithModel(
				nil, strictOpenAIAPIKeyPassthroughTestAccount(), true, "", payload,
				"upstream credential unavailable", "gpt-6.1-sol")

			require.False(t, failoverErr.StrictOpenAIAPIKeyPassthrough)
		})
	}
}

func TestOpenAIOAuthStreamErrorKeepsManagedEnvelope(t *testing.T) {
	payload := []byte(`{"type":"error","error":{"type":"too_many_requests","code":"rate_limit_exceeded","message":"quota exhausted"}}`)
	failoverErr := (&OpenAIGatewayService{}).newOpenAIStreamFailoverErrorWithModel(
		nil,
		&Account{ID: 938, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{"openai_passthrough": true}},
		true,
		"",
		payload,
		"quota exhausted",
		"gpt-6.1-sol",
	)

	require.False(t, failoverErr.StrictOpenAIAPIKeyPassthrough)
	require.Equal(t, "rate_limit_error", gjson.GetBytes(failoverErr.ResponseBody, "error.type").String())
	require.False(t, gjson.GetBytes(failoverErr.ResponseBody, "error.code").Exists())
}
