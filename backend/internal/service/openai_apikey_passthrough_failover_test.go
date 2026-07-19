package service

import (
	"bytes"
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
	"github.com/tidwall/gjson"
)

type passthroughReadOnceBody struct {
	reader io.Reader
	reads  int
	closed int
}

func (b *passthroughReadOnceBody) Read(p []byte) (int, error) {
	b.reads++
	if b.reads > 2 { // one data read plus the EOF read used by io.ReadAll
		return 0, errors.New("body read after exhaustion")
	}
	return b.reader.Read(p)
}

func (b *passthroughReadOnceBody) Close() error { b.closed++; return nil }

type passthroughCountingBody struct {
	reader io.Reader
	reads  int
}

func (b *passthroughCountingBody) Read(p []byte) (int, error) {
	b.reads++
	return b.reader.Read(p)
}

func (b *passthroughCountingBody) Close() error { return nil }

func newAPIKeyPassthroughAccount() *Account {
	return &Account{
		ID: 91, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://api.example.test"},
		Extra:       map[string]any{"openai_passthrough": true}, Status: StatusActive, Schedulable: true,
	}
}

func TestAPIKeyPassthroughTransient5xxFailsOverWithSanitizedExhaustion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	requestBody := []byte(`{"model":"gpt-5.2","stream":false,"input":"hello"}`)
	for _, status := range []int{500, 502, 503, 504, 520, 521, 522, 523, 524} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
			body := &passthroughReadOnceBody{reader: strings.NewReader(`{"error":{"message":"secret.example sk-upstream-secret"},"debug":"private"}`)}
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: status, Header: http.Header{
				"Content-Type": {"text/html"}, "Retry-After": {"17"}, "Set-Cookie": {"secret=1"}, "X-Request-Id": {"private-id"},
			}, Body: body}}
			svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: false}}, httpUpstream: upstream}

			result, err := svc.Forward(context.Background(), c, newAPIKeyPassthroughAccount(), requestBody)

			require.Nil(t, result)
			require.Error(t, err)
			var failoverErr *UpstreamFailoverError
			require.True(t, errors.As(err, &failoverErr))
			safeStatus, safeBody, safeHeaders, ok := failoverErr.SanitizedClientResponse()
			require.True(t, ok)
			require.Equal(t, status, safeStatus)
			require.Equal(t, "Upstream service temporarily unavailable", gjson.GetBytes(safeBody, "error.message").String())
			require.NotContains(t, string(safeBody), "secret")
			require.Equal(t, "application/json; charset=utf-8", safeHeaders.Get("Content-Type"))
			require.Equal(t, "no-store", safeHeaders.Get("Cache-Control"))
			require.Equal(t, "17", safeHeaders.Get("Retry-After"))
			require.Empty(t, rec.Body.String())
			require.Equal(t, 1, body.closed)
			require.Equal(t, 2, body.reads)
			require.Equal(t, requestBody, upstream.lastBody)
		})
	}
}

func TestAPIKeyPassthroughContextWindowMessageUsesOnlyMatchedTrustedField(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
	upstreamBody := `{"error":{"message":"Your input exceeds the context window; api_key=sk-upstream-secret"},"debug":"secret.example"}`
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: false}}, httpUpstream: &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadGateway, Header: http.Header{"Content-Type": {"text/html"}, "Set-Cookie": {"secret=1"}}, Body: io.NopCloser(strings.NewReader(upstreamBody)),
	}}}

	_, err := svc.Forward(context.Background(), c, newAPIKeyPassthroughAccount(), []byte(`{"model":"gpt-5.2","input":"hello"}`))

	var failoverErr *UpstreamFailoverError
	require.Error(t, err)
	require.False(t, errors.As(err, &failoverErr))
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Equal(t, openAIContextWindowClientMessage(), gjson.Get(rec.Body.String(), "error.message").String())
	require.NotContains(t, rec.Body.String(), "sk-upstream-secret")
	require.NotContains(t, rec.Body.String(), "secret.example")
	require.False(t, gjson.Get(rec.Body.String(), "debug").Exists())
	require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	require.Empty(t, rec.Header().Get("Set-Cookie"))
}

func TestClassifyOpenAIContextWindowErrorTrustedFieldContract(t *testing.T) {
	oversized := strings.Repeat("x", 700) + " exceeds the context window"
	tests := []struct {
		name        string
		body        []byte
		field       string
		shouldMatch bool
	}{
		{"debug only", []byte(`{"error":{"message":"secret.example"},"debug":"context_length_exceeded"}`), "", false},
		{"html", []byte(`<html>maximum context length secret.example</html>`), "", false},
		{"trailing", []byte(`{"error":{"code":"context_length_exceeded"}} secret.example`), "", false},
		{"trusted code unrelated message", []byte(`{"error":{"code":"context_length_exceeded","message":"secret.example sk-secret"}}`), "error.code", true},
		{"trusted message", []byte(`{"error":{"message":"Maximum context length exceeded; CUSTOMER_PRIVATE_MARKER_8472"}}`), "error.message", true},
		{"code message mismatch", []byte(`{"error":{"code":"context_length_exceeded","message":"Maximum context length exceeded; secret.example"}}`), "error.code", true},
		{"unrelated code matching message", []byte(`{"error":{"code":"other","message":"Maximum context length exceeded"}}`), "error.message", true},
		{"nested unknown", []byte(`{"outer":{"error":{"code":"context_length_exceeded"}},"error":{"message":"secret.example"}}`), "", false},
		{"response nested", []byte(`{"response":{"error":{"message":"input exceeds the context window"}}}`), "response.error.message", true},
		{"root array", []byte(`[{"error":{"code":"context_length_exceeded"}}]`), "", false},
		{"error array", []byte(`{"error":[{"code":"other"},{"code":"context_length_exceeded"}]}`), "", false},
		{"response array", []byte(`{"response":[{"error":{"code":"context_length_exceeded"}}]}`), "", false},
		{"response error array", []byte(`{"response":{"error":[{"message":"ordinary"},{"message":"maximum context length"}]}}`), "", false},
		{"root multi element spoof", []byte(`[{"message":"ordinary"},{"message":"maximum context length"}]`), "", false},
		{"duplicate object", []byte(`{"error":{"code":"other"},"error":{"code":"context_too_large"}}`), "", false},
		{"duplicate merge through omission", []byte(`{"error":{"code":"context_length_exceeded"},"error":{"message":"ordinary transient outage"}}`), "", false},
		{"duplicate trusted leaf", []byte(`{"error":{"code":"other","code":"context_too_large"}}`), "", false},
		{"case colliding code with exact match", []byte(`{"error":{"code":"context_length_exceeded","Code":"other"}}`), "", false},
		{"case colliding message with exact match", []byte(`{"error":{"message":"maximum context length","Message":"ordinary"}}`), "", false},
		{"matching code plus non-string message", []byte(`{"error":{"code":"context_length_exceeded","message":{"text":"ordinary"}}}`), "", false},
		{"matching message plus non-string code", []byte(`{"error":{"message":"maximum context length","code":17}}`), "", false},
		{"matching nested leaf plus malformed response", []byte(`{"error":{"code":"context_length_exceeded"},"response":[]}`), "", false},
		{"object trusted leaf", []byte(`{"error":{"code":{"nested":"context_length_exceeded"},"message":"ordinary outage"}}`), "", false},
		{"case variant", []byte(`{"Error":{"code":"context_length_exceeded"}}`), "", false},
		{"malformed", []byte(`{"error":{"code":"context_length_exceeded"}`), "", false},
		{"invalid utf8", append([]byte(`{"error":{"message":"context window exceeded `), 0xff), "", false},
		{"oversized hostile message", []byte(`{"message":` + fmt.Sprintf("%q", oversized) + `}`), "message", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			match := classifyOpenAIContextWindowError(tc.body)
			require.Equal(t, tc.shouldMatch, match.matched())
			require.Equal(t, tc.field, match.field)
			require.Equal(t, tc.name != "html" && tc.name != "trailing" && tc.name != "root array" && tc.name != "error array" && tc.name != "response array" && tc.name != "response error array" && tc.name != "root multi element spoof" && tc.name != "duplicate object" && tc.name != "duplicate merge through omission" && tc.name != "duplicate trusted leaf" && tc.name != "case colliding code with exact match" && tc.name != "case colliding message with exact match" && tc.name != "matching code plus non-string message" && tc.name != "matching message plus non-string code" && tc.name != "matching nested leaf plus malformed response" && tc.name != "object trusted leaf" && tc.name != "case variant" && tc.name != "malformed" && tc.name != "invalid utf8", match.valid)
			if tc.shouldMatch {
				text := openAIContextWindowClientMessage()
				require.NotContains(t, text, "CUSTOMER_PRIVATE_MARKER_8472")
				require.NotContains(t, text, "secret.example")
			}
		})
	}
}

func TestAPIKeyPassthroughRejected429And529DoNotReplay(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		status int
		body   string
	}{
		{http.StatusTooManyRequests, `{"error":{"type":"invalid_request_error","code":"rejected_field","message":"temperature"}}`},
		{529, `{"response":{"error":{"type":"invalid_request","message":"Rejected field: reasoning_effort"}}}`},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: tc.status, Header: http.Header{"Retry-After": {"11"}}, Body: io.NopCloser(strings.NewReader(tc.body))}}
			svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: false}}, httpUpstream: upstream}
			requestBody := []byte(`{"model":"gpt-5.2","input":"hello"}`)

			result, err := svc.Forward(context.Background(), c, newAPIKeyPassthroughAccount(), requestBody)

			require.Nil(t, result)
			require.Error(t, err)
			var failoverErr *UpstreamFailoverError
			require.False(t, errors.As(err, &failoverErr), "rejected requests must not replay or rotate accounts")
			require.Equal(t, tc.status, rec.Code)
			expectedMessage := "Upstream request failed"
			if tc.status >= http.StatusInternalServerError {
				expectedMessage = "Upstream service temporarily unavailable"
			}
			require.Equal(t, expectedMessage, gjson.Get(rec.Body.String(), "error.message").String())
			require.Equal(t, requestBody, upstream.lastBody)
		})
	}
}

func TestRejectedFieldClassificationRequiresStrictTrustedEnvelope(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{"exact code", `{"error":{"code":"rejected_field"}}`, true},
		{"invalid parameter code", `{"error":{"code":"invalid_parameter"}}`, true},
		{"invalid field code", `{"error":{"code":"invalid_field"}}`, true},
		{"invalid parameter text", `{"error":{"message":"invalid parameter"}}`, true},
		{"invalid parameter detail", `{"error":{"message":"Invalid parameter: reasoning_effort"}}`, true},
		{"invalid field detail", `{"error":{"message":"Invalid field: temperature"}}`, true},
		{"substring spoof", `{"error":{"message":"service says not_invalid parameterized capacity"}}`, false},
		{"exact textual message", `{"response":{"error":{"message":"Rejected field: reasoning_effort"}}}`, true},
		{"untrusted debug", `{"error":{"message":"capacity"},"debug":"rejected_field"}`, false},
		{"nested unknown", `{"outer":{"error":{"code":"rejected_field"}}}`, false},
		{"duplicate trusted object", `{"error":{"code":"capacity"},"error":{"code":"rejected_field"}}`, false},
		{"duplicate trusted leaf", `{"error":{"code":"capacity","code":"rejected_field"}}`, false},
		{"case collision", `{"error":{"code":"rejected_field","Code":"capacity"}}`, false},
		{"case variant path", `{"Error":{"code":"rejected_field"}}`, false},
		{"non-string trusted leaf", `{"error":{"code":["rejected_field"]}}`, false},
		{"trailing document", `{"error":{"code":"rejected_field"}} {}`, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			match := classifyOpenAIContextWindowError([]byte(tc.body))
			require.Equal(t, tc.want, match.rejectedRequest)
			require.Equal(t, tc.want || match.valid, match.valid)
			require.Equal(t, !tc.want && match.valid, shouldFailoverOpenAIPassthroughResponse(http.StatusTooManyRequests, match, true))
			require.Equal(t, !tc.want && match.valid, shouldFailoverOpenAIPassthroughResponse(http.StatusBadGateway, match, true))
		})
	}
}

func TestSanitizedFailoverMarkerConstructionBoundary(t *testing.T) {
	ordinary := &UpstreamFailoverError{
		StatusCode:      http.StatusTooManyRequests,
		ResponseBody:    []byte(`{"error":{"message":"spoof"}}`),
		ResponseHeaders: http.Header{"Cache-Control": {"no-store"}},
	}
	_, _, _, ok := ordinary.SanitizedClientResponse()
	require.False(t, ok, "public failover fields must not manufacture the bypass")
	_, _, _, ok = (*UpstreamFailoverError)(nil).SanitizedClientResponse()
	require.False(t, ok)

	sanitized := newSanitizedUpstreamFailoverError(
		http.StatusTooManyRequests,
		[]byte(`{"error":{"message":"Upstream request failed"}}`),
		http.Header{"Cache-Control": {"no-store"}},
		false,
	)
	_, _, _, ok = sanitized.SanitizedClientResponse()
	require.True(t, ok)
	_, body, headers, _ := sanitized.SanitizedClientResponse()
	body[0] = 'X'
	headers.Set("Cache-Control", "public")
	_, bodyAgain, headersAgain, _ := sanitized.SanitizedClientResponse()
	require.Equal(t, byte('{'), bodyAgain[0])
	require.Equal(t, "no-store", headersAgain.Get("Cache-Control"))
}

func TestAPIKeyPassthroughFailoverStatusMatrix(t *testing.T) {
	for _, status := range []int{500, 502, 503, 504, 520, 521, 522, 523, 524, 429, 529} {
		require.True(t, shouldFailoverOpenAIPassthroughResponse(status, openAIContextWindowMatch{valid: true}, true), status)
	}
	for _, status := range []int{400, 401, 403, 404, 409, 413, 422} {
		require.False(t, shouldFailoverOpenAIPassthroughResponse(status, openAIContextWindowMatch{valid: true}, true), status)
	}
	for _, status := range []int{500, 502, 503, 504, 520, 521, 522, 523, 524} {
		require.False(t, shouldFailoverOpenAIPassthroughResponse(status, openAIContextWindowMatch{}, false), status)
	}
	rejected := openAIContextWindowMatch{rejectedRequest: true, valid: true}
	contextWindow := openAIContextWindowMatch{field: "error.code", valid: true}
	for _, status := range []int{429, 529, 500, 502, 503, 504, 520, 521, 522, 523, 524} {
		require.False(t, shouldFailoverOpenAIPassthroughResponse(status, rejected, true), status)
		require.False(t, shouldFailoverOpenAIPassthroughResponse(status, contextWindow, true), status)
	}
}

func TestAPIKeyPassthroughUntrustedContextSignalsPreserveCapacityFailover(t *testing.T) {
	for _, body := range []string{
		`{"error":{"message":"secret.example sk-secret"},"debug":"context_length_exceeded"}`,
		`<html>maximum context length secret.example</html>`,
		`{"error":{"message":"secret.example"}} trailing maximum context length`,
	} {
		match := classifyOpenAIContextWindowError([]byte(body))
		require.Equal(t, strings.HasPrefix(body, "{") && strings.HasSuffix(body, "}"), match.valid)
		require.Equal(t, match.valid, shouldFailoverOpenAIPassthroughResponse(http.StatusTooManyRequests, match, true))
		require.Equal(t, match.valid, shouldFailoverOpenAIPassthroughResponse(http.StatusBadGateway, match, true))
	}
}

func TestOpenAIPassthroughRetryAfterValidation(t *testing.T) {
	now := time.Now().UTC()
	for _, tc := range []struct {
		raw  string
		want bool
	}{
		{"17", true}, {"0", false}, {"-1", false}, {"+17", false}, {"1.5", false}, {"1e3", false},
		{"18446744073709551616", false}, {now.Add(time.Hour).Format(http.TimeFormat), true}, {now.Add(-time.Hour).Format(http.TimeFormat), false},
	} {
		require.Equal(t, tc.want, validOpenAIPassthroughRetryAfter(tc.raw, now), tc.raw)
	}
}

func TestOAuthPassthrough502RemainsUnchanged(t *testing.T) {
	require.False(t, shouldFailoverOpenAIPassthroughResponse(http.StatusBadGateway, openAIContextWindowMatch{}, false))
	require.False(t, shouldFailoverOpenAIPassthroughResponse(http.StatusTooManyRequests, openAIContextWindowMatch{}, false))
	require.False(t, shouldFailoverOpenAIPassthroughResponse(529, openAIContextWindowMatch{}, false))
	require.False(t, shouldFailoverOpenAIPassthroughResponse(http.StatusRequestEntityTooLarge, openAIContextWindowMatch{}, true))
}

func TestPassthroughContextClassificationRunsOnceForAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	calls := 0
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
	body := &passthroughReadOnceBody{reader: strings.NewReader(`{"error":{"code":"context_length_exceeded"}}`)}
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: false}},
		httpUpstream: &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusBadRequest, Header: make(http.Header), Body: body}},
		classifyContextWindowError: func(body []byte) openAIContextWindowMatch {
			calls++
			return classifyOpenAIContextWindowError(body)
		},
	}

	_, _ = svc.Forward(context.Background(), c, newAPIKeyPassthroughAccount(), []byte(`{"model":"gpt-5.2","input":"hello"}`))

	require.Equal(t, 1, calls)
	require.Equal(t, 2, body.reads)
	require.Equal(t, 1, body.closed)
}

func TestOAuthSkipsAPIKeyContextClassification(t *testing.T) {
	calls := 0
	svc := &OpenAIGatewayService{classifyContextWindowError: func(body []byte) openAIContextWindowMatch {
		calls++
		return classifyOpenAIContextWindowError(body)
	}}

	match := svc.classifyOpenAIContextWindowErrorForAccount(&Account{Type: AccountTypeOAuth}, []byte(`{"error":{"code":"context_length_exceeded"}}`))

	require.False(t, match.matched())
	require.Zero(t, calls)
}

func TestOpenAIErrorClassificationFixedWorkBoundsFailClosed(t *testing.T) {
	validAtByteBoundary := []byte(`{"error":{"code":"context_length_exceeded"},"padding":"` +
		strings.Repeat("x", openAIErrorClassificationMaxBytes-len(`{"error":{"code":"context_length_exceeded"},"padding":""}`)) + `"}`)
	require.Len(t, validAtByteBoundary, openAIErrorClassificationMaxBytes)
	require.True(t, classifyOpenAIContextWindowError(validAtByteBoundary).matched())

	overByteBoundary := append(append([]byte(nil), validAtByteBoundary...), ' ')
	require.Equal(t, openAIContextWindowMatch{overflow: true}, classifyOpenAIContextWindowError(overByteBoundary))
	require.False(t, shouldFailoverOpenAIPassthroughResponse(http.StatusBadGateway, classifyOpenAIContextWindowError(overByteBoundary), true))

	deep := `{"error":{"code":"context_length_exceeded"},"x":` + strings.Repeat(`[`, openAIErrorClassificationMaxDepth) +
		`0` + strings.Repeat(`]`, openAIErrorClassificationMaxDepth) + `}`
	require.Equal(t, openAIContextWindowMatch{overflow: true}, classifyOpenAIContextWindowError([]byte(deep)))
	require.False(t, shouldFailoverOpenAIPassthroughResponse(http.StatusBadGateway, classifyOpenAIContextWindowError([]byte(deep)), true))

	items := make([]string, openAIErrorClassificationMaxTokens)
	for i := range items {
		items[i] = `0`
	}
	tokenHeavy := `{"error":{"code":"context_length_exceeded"},"items":[` + strings.Join(items, `,`) + `]}`
	require.Less(t, len(tokenHeavy), openAIErrorClassificationMaxBytes)
	require.Equal(t, openAIContextWindowMatch{overflow: true}, classifyOpenAIContextWindowError([]byte(tokenHeavy)))
	require.False(t, shouldFailoverOpenAIPassthroughResponse(http.StatusBadGateway, classifyOpenAIContextWindowError([]byte(tokenHeavy)), true))
}

func TestAPIKeyPassthroughLargeLoggingConfigKeepsSingleReadBoundedAndDoesNotReplay(t *testing.T) {
	body := &passthroughCountingBody{reader: bytes.NewReader(bytes.Repeat([]byte{'x'}, int(openAIUpstreamErrorBodyReadLimit)*2))}
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{
		LogUpstreamErrorBody: true, LogUpstreamErrorBodyMaxBytes: int(openAIUpstreamErrorBodyReadLimit) * 64,
	}}}
	resp := &http.Response{StatusCode: http.StatusBadGateway, Body: body}
	read := svc.readUpstreamErrorBody(resp)

	require.Len(t, read, int(openAIUpstreamErrorBodyReadLimit))
	require.Equal(t, int64(openAIUpstreamErrorBodyReadLimit), openAIUpstreamErrorBodyReadLimitForConfig(svc.cfg))
	match := svc.classifyOpenAIContextWindowErrorForAccount(newAPIKeyPassthroughAccount(), read)
	require.False(t, match.valid)
	require.False(t, shouldFailoverOpenAIPassthroughResponse(resp.StatusCode, match, true))
	readsAfterBoundedRead := body.reads
	require.Greater(t, readsAfterBoundedRead, 0)
	require.Equal(t, readsAfterBoundedRead, body.reads, "classification must use the buffered body without a second upstream read")
}

func TestAPIKeyClassificationIsIndependentOfLogBodyLimit(t *testing.T) {
	body := []byte(`{"error":{"code":"context_length_exceeded"}}`)
	for _, configured := range []int{1, len(body) - 1, len(body), openAIErrorClassificationMaxBytes * 2} {
		svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{
			LogUpstreamErrorBody:         true,
			LogUpstreamErrorBodyMaxBytes: configured,
		}}}
		require.True(t, svc.classifyOpenAIContextWindowErrorForAccount(newAPIKeyPassthroughAccount(), body).matched(), configured)
	}
}

func TestOAuthSkipsClassificationAtFixedBoundaries(t *testing.T) {
	calls := 0
	svc := &OpenAIGatewayService{classifyContextWindowError: func([]byte) openAIContextWindowMatch {
		calls++
		return openAIContextWindowMatch{field: "error.code", rejectedRequest: true}
	}}
	for _, size := range []int{openAIErrorClassificationMaxBytes, openAIErrorClassificationMaxBytes + 1} {
		body := bytes.Repeat([]byte{'x'}, size)
		match := svc.classifyOpenAIContextWindowErrorForAccount(&Account{Type: AccountTypeOAuth}, body)
		require.Equal(t, openAIContextWindowMatch{}, match)
	}
	require.Zero(t, calls)
}
