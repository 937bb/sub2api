package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type admittedCancelHTTPUpstream struct {
	responses []*http.Response
	errors    []error
	onDo      func(int)
	calls     int
}

func (u *admittedCancelHTTPUpstream) Do(
	_ *http.Request,
	_ string,
	_ int64,
	_ int,
) (*http.Response, error) {
	u.calls++
	if u.onDo != nil {
		u.onDo(u.calls)
	}
	var resp *http.Response
	if u.calls <= len(u.responses) {
		resp = u.responses[u.calls-1]
	}
	var err error
	if u.calls <= len(u.errors) {
		err = u.errors[u.calls-1]
	}
	return resp, err
}

func (u *admittedCancelHTTPUpstream) DoWithTLS(
	req *http.Request,
	proxyURL string,
	accountID int64,
	accountConcurrency int,
	_ *tlsfingerprint.Profile,
) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

func newGenericRetryGatewayForTest(upstream HTTPUpstream) (*GatewayService, *Account) {
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	return &GatewayService{
			httpUpstream:        upstream,
			cfg:                 cfg,
			rateLimitService:    &RateLimitService{},
			tlsFPProfileService: &TLSFingerprintProfileService{},
		}, &Account{
			ID:          1,
			Name:        "generic-retry-test",
			Platform:    PlatformAnthropic,
			Type:        AccountTypeAPIKey,
			Concurrency: 1,
			Credentials: map[string]any{
				"api_key":                    "test-key",
				"base_url":                   "https://example.com",
				"custom_error_codes_enabled": true,
				"custom_error_codes":         []any{float64(http.StatusBadRequest)},
			},
		}
}

func newGenericRetryRequest(t *testing.T) (*gin.Context, *ParsedRequest, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	parsed, err := ParseGatewayRequest(
		NewRequestBodyRef([]byte(`{"model":"claude-3-5-sonnet-latest","messages":[]}`)),
		PlatformAnthropic,
	)
	require.NoError(t, err)
	return c, parsed, rec
}

func TestGatewayServiceForward_RetryCancellationPreservesCompletedError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	admissions := 0
	ctx = WithHTTPAttemptAdmissionHook(ctx, func() {
		admissions++
		if admissions == 2 {
			cancel()
		}
	})
	upstream := &admittedCancelHTTPUpstream{responses: []*http.Response{{
		StatusCode: 529,
		Header:     http.Header{"X-Request-Id": []string{"completed"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"completed overload"}}`)),
	}}}
	svc, account := newGenericRetryGatewayForTest(upstream)
	c, parsed, rec := newGenericRetryRequest(t)

	_, err := svc.Forward(ctx, c, account, parsed)

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, 529, failoverErr.StatusCode)
	require.Contains(t, string(failoverErr.ResponseBody), "completed overload")
	require.Equal(t, 1, upstream.calls)
	require.Equal(t, 2, admissions)
	require.Zero(t, rec.Body.Len())
}

func TestGatewayServiceForward_AdmittedRetryContextCanceledWithLiveDownstreamWritesError(t *testing.T) {
	ctx := context.Background()
	upstream := &admittedCancelHTTPUpstream{
		responses: []*http.Response{{
			StatusCode: 529,
			Header:     http.Header{"X-Request-Id": []string{"completed"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"completed overload"}}`)),
		}, nil},
		errors: []error{nil, context.Canceled},
	}
	svc, account := newGenericRetryGatewayForTest(upstream)
	c, parsed, rec := newGenericRetryRequest(t)

	_, err := svc.Forward(ctx, c, account, parsed)

	require.EqualError(t, err, "upstream request failed: context canceled")
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.Equal(t, 2, upstream.calls)
	require.Contains(t, rec.Body.String(), "Upstream request failed")
}

func TestCompletedResponseSnapshot_RestoresOnlyUnadmittedRetries(t *testing.T) {
	snapshot := snapshotCompletedResponse(&http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Header:     http.Header{"X-Request-Id": []string{"completed"}},
	}, []byte("completed"))

	for _, test := range []struct {
		name    string
		err     error
		restore bool
	}{
		{
			name:    "logical call never admitted",
			err:     &HTTPUpstreamAttemptNotAdmittedError{cause: context.Canceled},
			restore: true,
		},
		{
			name:    "later retry not admitted",
			err:     &httpUpstreamRetryNotAdmittedError{cause: context.Canceled},
			restore: true,
		},
		{
			name: "admitted transport cancellation",
			err:  context.Canceled,
		},
		{
			name: "admitted transport failure",
			err:  errors.New("transport failed"),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			resp, restored := snapshot.ifRetryNotAdmitted(test.err)
			require.Equal(t, test.restore, restored)
			if !test.restore {
				require.Nil(t, resp)
				return
			}
			require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
			require.Equal(t, "completed", resp.Header.Get("X-Request-Id"))
			require.NoError(t, resp.Body.Close())
		})
	}
}

func TestGatewayServiceForward_AdmittedRetryCancellationDoesNotRestorePriorError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	upstream := &admittedCancelHTTPUpstream{
		responses: []*http.Response{{
			StatusCode: 529,
			Header:     http.Header{"X-Request-Id": []string{"completed"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"completed overload"}}`)),
		}, nil},
		errors: []error{nil, errors.New("admitted transport failure")},
		onDo: func(call int) {
			if call == 2 {
				cancel()
			}
		},
	}
	svc, account := newGenericRetryGatewayForTest(upstream)
	c, parsed, rec := newGenericRetryRequest(t)
	c.Request = c.Request.WithContext(ctx)

	_, err := svc.Forward(ctx, c, account, parsed)

	require.ErrorIs(t, err, context.Canceled)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.Equal(t, 2, upstream.calls)
	require.Zero(t, rec.Body.Len())
}

func newAntigravityOuterRetryParams(ctx context.Context, upstream HTTPUpstream) antigravityRetryLoopParams {
	return antigravityRetryLoopParams{
		ctx:    withHTTPAttemptAuthority(ctx),
		prefix: "[outer-retry-test]",
		account: &Account{
			ID:          1,
			Name:        "outer-retry-test",
			Platform:    PlatformAntigravity,
			Type:        AccountTypeAPIKey,
			Concurrency: 1,
		},
		accessToken:    "token",
		action:         "generateContent",
		body:           []byte(`{"model":"test"}`),
		httpUpstream:   upstream,
		requestedModel: "test",
	}
}

func newAntigravityRetryErrorResponse() *http.Response {
	return &http.Response{
		StatusCode: http.StatusInternalServerError,
		Header:     http.Header{"X-Request-Id": []string{"stale-http-error"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"stale HTTP error"}}`)),
	}
}

func TestAntigravityRetryLoop_TransportErrorCancellationDoesNotRestorePriorHTTPError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	transportErr := errors.New("admitted transport failure")
	upstream := &admittedCancelHTTPUpstream{
		responses: []*http.Response{newAntigravityRetryErrorResponse(), nil},
		errors:    []error{nil, transportErr},
		onDo: func(call int) {
			if call == 2 {
				cancel()
			}
		},
	}
	svc := &AntigravityGatewayService{}

	result, err := svc.antigravityRetryLoop(newAntigravityOuterRetryParams(ctx, upstream))

	require.ErrorIs(t, err, transportErr)
	require.Nil(t, result)
	require.Equal(t, 2, upstream.calls)
}

func TestAntigravityRetryLoop_TransportErrorThenNonAdmissionDoesNotRestorePriorHTTPError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	transportErr := errors.New("admitted transport failure")
	admissions := 0
	ctx = WithHTTPAttemptAdmissionHook(ctx, func() {
		admissions++
		if admissions == 3 {
			cancel()
		}
	})
	upstream := &admittedCancelHTTPUpstream{
		responses: []*http.Response{newAntigravityRetryErrorResponse(), nil},
		errors:    []error{nil, transportErr},
	}
	svc := &AntigravityGatewayService{}

	result, err := svc.antigravityRetryLoop(newAntigravityOuterRetryParams(ctx, upstream))

	require.ErrorIs(t, err, transportErr)
	require.Nil(t, result)
	require.Equal(t, 3, admissions)
	require.Equal(t, 2, upstream.calls)
}

func TestAntigravityRetryLoop_TransportErrorThenSuccessUsesNewestResponse(t *testing.T) {
	transportErr := errors.New("admitted transport failure")
	upstream := &admittedCancelHTTPUpstream{
		responses: []*http.Response{
			newAntigravityRetryErrorResponse(),
			nil,
			{
				StatusCode: http.StatusOK,
				Header:     http.Header{"X-Request-Id": []string{"success"}},
				Body:       io.NopCloser(strings.NewReader(`{"result":"ok"}`)),
			},
		},
		errors: []error{nil, transportErr, nil},
	}
	svc := &AntigravityGatewayService{}

	result, err := svc.antigravityRetryLoop(newAntigravityOuterRetryParams(context.Background(), upstream))

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.resp)
	require.Equal(t, http.StatusOK, result.resp.StatusCode)
	require.Equal(t, "success", result.resp.Header.Get("X-Request-Id"))
	require.Equal(t, 3, upstream.calls)
	require.NoError(t, result.resp.Body.Close())
}
