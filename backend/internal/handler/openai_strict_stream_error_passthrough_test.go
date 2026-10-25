//go:build unit

package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type strictOpenAIErrorUpstream struct {
	service.HTTPUpstream
	status      int
	body        string
	contentType string
	calls       int
}

func (u *strictOpenAIErrorUpstream) Do(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.calls++
	return &http.Response{
		StatusCode: u.status,
		Header:     http.Header{"Content-Type": {u.contentType}, "Retry-After": {"2"}},
		Body:       io.NopCloser(strings.NewReader(u.body)),
	}, nil
}

func TestStrictOpenAIAPIKeyResponsesPreservesHTTPAndStreamErrors(t *testing.T) {
	rateLimit := `{"type":"too_many_requests","code":"rate_limit_exceeded","message":"Your requests to gpt-6.1-sol for gpt-6.1-sol in eastus have exceeded token rate limit.","param":null}`
	encrypted := `{"type":"invalid_request_error","code":"invalid_encrypted_content","message":"The encrypted content could not be decrypted or parsed.","param":""}`
	azureAuth := `{"type":"authentication_error","code":"InvalidAuthenticationToken","message":"Access token validation failure","param":null,"status":401,"innererror":{"code":"ExpiredAuthenticationToken","date":"2026-10-25T01:02:03Z"},"headers":{"x-ms-fe-error":"true"}}`
	azureAuthBody := `{"error":` + azureAuth + `,"request_id":"az-request-401"}`
	for _, test := range []struct {
		name           string
		upstreamStatus int
		body           string
		contentType    string
		errorJSON      string
		stream         bool
		wantStatus     int
		wantCalls      int
		wantBody       string
	}{
		{"http429 stream", 429, `{"error":` + rateLimit + `}`, "application/json", rateLimit, true, 429, 2, ""},
		{"http429 nonstream", 429, `{"error":` + rateLimit + `}`, "application/json", rateLimit, false, 429, 2, ""},
		{"http400 stream", 400, `{"error":` + encrypted + `}`, "application/json", encrypted, true, 400, 1, ""},
		{"http400 nonstream", 400, `{"error":` + encrypted + `}`, "application/json", encrypted, false, 400, 1, ""},
		{"az bare SSE rate limit", 200, "event: error\ndata: {\"type\":\"error\",\"error\":" + rateLimit + ",\"sequence_number\":1}\n\n", "text/event-stream", rateLimit, true, 429, 2, ""},
		{"azure 401 nonstream", 401, azureAuthBody, "application/json", azureAuth, false, 401, 1, azureAuthBody},
	} {
		t.Run(test.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			groupID := int64(4205)
			accounts := []service.Account{{
				ID: 9913, Name: "strict-error-upstream", Platform: service.PlatformOpenAI,
				Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Priority: 1,
				Credentials: map[string]any{
					"api_key": "sk-test", "base_url": "https://api.example.test",
					"pool_mode": true, "pool_mode_retry_count": float64(1),
					"pool_mode_retry_status_codes": []any{float64(400), float64(429)},
				},
				Extra: map[string]any{"openai_passthrough": true},
			}}
			cfg := &config.Config{RunMode: config.RunModeSimple}
			cfg.Default.RateMultiplier = 1
			cfg.Security.URLAllowlist.Enabled = false
			cfg.Gateway.MaxAccountSwitches = 1
			upstream := &strictOpenAIErrorUpstream{status: test.upstreamStatus, body: test.body, contentType: test.contentType}
			billingCache := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
			t.Cleanup(billingCache.Stop)
			gateway := service.NewOpenAIGatewayService(
				&openAIWSFailoverHandlerAccountRepoStub{accounts: accounts},
				nil, nil, nil, nil, nil, nil, cfg, nil, nil,
				service.NewBillingService(cfg, nil), nil, billingCache, upstream, &service.DeferredService{},
				nil, nil, nil, nil, nil, nil, nil,
			)
			h := NewOpenAIGatewayHandler(gateway, service.NewConcurrencyService(nil), billingCache,
				service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg), nil, nil, nil, nil, cfg)
			c, recorder := newGinContextForEndpoint(t, EndpointResponses)
			c.Request = httptest.NewRequest(http.MethodPost, EndpointResponses, strings.NewReader(`{"model":"gpt-6.1-sol","input":"hello","stream":`+strconv.FormatBool(test.stream)+`}`))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
				ID: 1805, GroupID: &groupID, User: &service.User{ID: 1705, Status: service.StatusActive},
				Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive},
			})
			c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1705, Concurrency: 0})

			h.Responses(c)

			require.Equal(t, test.wantCalls, upstream.calls)
			require.Equal(t, test.wantStatus, recorder.Code)
			require.Contains(t, recorder.Header().Get("Content-Type"), "application/json")
			require.JSONEq(t, test.errorJSON, gjson.GetBytes(recorder.Body.Bytes(), "error").Raw)
			if test.wantBody != "" {
				require.JSONEq(t, test.wantBody, recorder.Body.String())
			}
			if test.wantStatus == http.StatusTooManyRequests {
				require.Equal(t, "2", recorder.Header().Get("Retry-After"))
			}
		})
	}
}

func TestStrictOpenAIAPIKeyFailoverErrorPreservesJSONRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"error":{"type":"too_many_requests","code":"rate_limit_exceeded","message":"Your requests to gpt-6.1-sol in eastus have exceeded token rate limit.","param":"model"}}`)

	(&OpenAIGatewayHandler{}).handleFailoverExhausted(c, &service.UpstreamFailoverError{
		StatusCode:                    http.StatusTooManyRequests,
		ResponseBody:                  body,
		StrictOpenAIAPIKeyPassthrough: true,
	}, false)

	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	require.Equal(t, "too_many_requests", gjson.GetBytes(recorder.Body.Bytes(), "error.type").String())
	require.Equal(t, "rate_limit_exceeded", gjson.GetBytes(recorder.Body.Bytes(), "error.code").String())
	require.Equal(t, "model", gjson.GetBytes(recorder.Body.Bytes(), "error.param").String())
	require.Equal(t, gjson.GetBytes(body, "error.message").String(), gjson.GetBytes(recorder.Body.Bytes(), "error.message").String())
}

func TestStrictOpenAIAPIKeyFailoverErrorPreservesCompleteAzure401(t *testing.T) {
	body := []byte(`{"error":{"type":"authentication_error","code":"InvalidAuthenticationToken","message":"Access token validation failure","param":null,"status":401,"innererror":{"code":"ExpiredAuthenticationToken","date":"2026-10-25T01:02:03Z"},"headers":{"x-ms-fe-error":"true"}},"request_id":"az-request-401"}`)

	t.Run("http", func(t *testing.T) {
		c, recorder := newGinContextForEndpoint(t, EndpointResponses)
		(&OpenAIGatewayHandler{}).handleFailoverExhausted(c, &service.UpstreamFailoverError{
			StatusCode:                    http.StatusUnauthorized,
			ResponseBody:                  body,
			ResponseHeaders:               http.Header{"Content-Type": {"application/json; charset=utf-8"}},
			StrictOpenAIAPIKeyPassthrough: true,
		}, false)

		require.Equal(t, http.StatusUnauthorized, recorder.Code)
		require.Equal(t, "application/json; charset=utf-8", recorder.Header().Get("Content-Type"))
		require.JSONEq(t, string(body), recorder.Body.String())
	})

	t.Run("responses stream", func(t *testing.T) {
		c, recorder := newGinContextForEndpoint(t, EndpointResponses)
		c.Header("Content-Type", "text/event-stream")
		c.Writer.Flush()
		(&OpenAIGatewayHandler{}).handleFailoverExhausted(c, &service.UpstreamFailoverError{
			StatusCode:                    http.StatusUnauthorized,
			ResponseBody:                  body,
			StrictOpenAIAPIKeyPassthrough: true,
		}, true)

		_, errObj := parseResponsesFailedSSE(t, recorder.Body.String())
		require.Equal(t, "InvalidAuthenticationToken", errObj["code"])
		require.Equal(t, float64(401), errObj["status"])
		require.Equal(t, "ExpiredAuthenticationToken", errObj["innererror"].(map[string]any)["code"])
		require.Equal(t, "true", errObj["headers"].(map[string]any)["x-ms-fe-error"])
	})
}

func TestStrictOpenAIAPIKeyFailoverErrorMissingTypeUsesRateLimitFallback(t *testing.T) {
	c, recorder := newGinContextForEndpoint(t, EndpointResponses)
	body := []byte(`{"error":{"code":"rate_limit_exceeded","message":"Token rate limit exceeded","param":null}}`)

	(&OpenAIGatewayHandler{}).handleFailoverExhausted(c, &service.UpstreamFailoverError{
		StatusCode:                    http.StatusTooManyRequests,
		ResponseBody:                  body,
		StrictOpenAIAPIKeyPassthrough: true,
	}, false)

	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	require.Equal(t, "rate_limit_error", gjson.GetBytes(recorder.Body.Bytes(), "error.type").String())
	require.Equal(t, "rate_limit_exceeded", gjson.GetBytes(recorder.Body.Bytes(), "error.code").String())
	require.True(t, gjson.GetBytes(recorder.Body.Bytes(), "error.param").Exists())
	require.Equal(t, gjson.Null, gjson.GetBytes(recorder.Body.Bytes(), "error.param").Type)
}

func TestStrictOpenAIAPIKeyFailoverErrorPreservesStreamingRateLimit(t *testing.T) {
	c, recorder := newGinContextForEndpoint(t, EndpointResponses)
	body := []byte(`{"error":{"type":"too_many_requests","code":"rate_limit_exceeded","message":"Your requests to gpt-6.1-sol in eastus have exceeded token rate limit."}}`)

	(&OpenAIGatewayHandler{}).handleFailoverExhausted(c, &service.UpstreamFailoverError{
		StatusCode:                    http.StatusTooManyRequests,
		ResponseBody:                  body,
		StrictOpenAIAPIKeyPassthrough: true,
	}, true)

	_, errObj := parseResponsesFailedSSE(t, recorder.Body.String())
	require.Equal(t, "too_many_requests", errObj["type"])
	require.Equal(t, "rate_limit_exceeded", errObj["code"])
	require.Equal(t, gjson.GetBytes(body, "error.message").String(), errObj["message"])
}

func TestStrictOpenAIAPIKeyFailoverErrorPreservesStreamingParam(t *testing.T) {
	for _, param := range []string{`""`, `null`, `"input[4]"`} {
		t.Run(param, func(t *testing.T) {
			c, recorder := newGinContextForEndpoint(t, EndpointResponses)
			body := []byte(`{"error":{"type":"invalid_request_error","code":"invalid_encrypted_content","message":"Encrypted content could not be verified","param":` + param + `}}`)
			c.Header("Content-Type", "text/event-stream")
			c.Writer.Flush()

			(&OpenAIGatewayHandler{}).handleFailoverExhausted(c, &service.UpstreamFailoverError{
				StatusCode: http.StatusBadRequest, ResponseBody: body, StrictOpenAIAPIKeyPassthrough: true,
			}, true)

			require.Equal(t, http.StatusOK, recorder.Code)
			_, errObj := parseResponsesFailedSSE(t, recorder.Body.String())
			require.Equal(t, "invalid_request_error", errObj["type"])
			require.Contains(t, errObj, "param")
			require.JSONEq(t, gjson.GetBytes(body, "error").Raw, gjson.Get(recorder.Body.String()[len("event: response.failed\ndata: "):], "response.error").Raw)
		})
	}
}

func TestStrictOpenAIAPIKeyFailoverErrorPreservesInvalidEncryptedContent(t *testing.T) {
	c, recorder := newGinContextForEndpoint(t, EndpointResponses)
	body := []byte(`{"error":{"type":"invalid_request_error","code":"invalid_encrypted_content","message":"The encrypted content could not be decrypted or parsed.","param":"input[4]"}}`)

	(&OpenAIGatewayHandler{}).handleFailoverExhausted(c, &service.UpstreamFailoverError{
		StatusCode:                    http.StatusBadRequest,
		ResponseBody:                  body,
		StrictOpenAIAPIKeyPassthrough: true,
	}, false)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Equal(t, "invalid_request_error", gjson.GetBytes(recorder.Body.Bytes(), "error.type").String())
	require.Equal(t, "invalid_encrypted_content", gjson.GetBytes(recorder.Body.Bytes(), "error.code").String())
	require.Equal(t, "input[4]", gjson.GetBytes(recorder.Body.Bytes(), "error.param").String())
	require.Equal(t, gjson.GetBytes(body, "error.message").String(), gjson.GetBytes(recorder.Body.Bytes(), "error.message").String())
}

func TestStrictOpenAIAPIKeyFailoverErrorPreservesStreamingInvalidEncryptedContent(t *testing.T) {
	c, recorder := newGinContextForEndpoint(t, EndpointResponses)
	body := []byte(`{"error":{"type":"invalid_request_error","code":"invalid_encrypted_content","message":"The encrypted content could not be decrypted or parsed."}}`)
	c.Header("Content-Type", "text/event-stream")
	c.Writer.Flush()

	(&OpenAIGatewayHandler{}).handleFailoverExhausted(c, &service.UpstreamFailoverError{
		StatusCode:                    http.StatusBadRequest,
		ResponseBody:                  body,
		StrictOpenAIAPIKeyPassthrough: true,
	}, true)

	require.Equal(t, http.StatusOK, recorder.Code)
	_, errObj := parseResponsesFailedSSE(t, recorder.Body.String())
	require.Equal(t, "invalid_encrypted_content", errObj["code"])
	require.Equal(t, gjson.GetBytes(body, "error.message").String(), errObj["message"])
}
