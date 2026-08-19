//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func newOpenAIInvalidImagePolicyHarness() (*OpenAIGatewayService, *Account, *errorPolicyRepoStub, string) {
	repo := &errorPolicyRepoStub{}
	svc := &OpenAIGatewayService{
		cfg:              &config.Config{},
		rateLimitService: NewRateLimitService(repo, nil, &config.Config{}, nil, nil),
	}
	account := newOpenAIUpstreamErrorTestAccount()
	account.Status = StatusActive
	account.Schedulable = true
	account.Credentials = map[string]any{
		"temp_unschedulable_enabled": true,
		"temp_unschedulable_rules": []any{
			map[string]any{
				"error_code":       float64(http.StatusBadRequest),
				"keywords":         []any{"input_image"},
				"duration_minutes": float64(30),
			},
		},
	}
	upstreamBody := `{"error":{"code":"invalid_value","message":"Invalid value: 'input_image'. Supported values are: 'input_text'.","param":"input[134].content[2]","type":"invalid_request_error"}}`
	return svc, account, repo, upstreamBody
}

func requireOpenAIInvalidImagePolicyUntouched(t *testing.T, repo *errorPolicyRepoStub, account *Account) {
	t.Helper()
	require.Zero(t, repo.tempCalls, "deterministic 400 must not temporarily unschedule the account")
	require.Zero(t, repo.setErrCalls, "deterministic 400 must not disable the account")
	require.Empty(t, repo.modelRateLimitCalls, "deterministic 400 must not block the model")
	require.Equal(t, StatusActive, account.Status)
}

// Production regression: a broad temporary-unschedulable 400 rule used to run
// before the deterministic-client-error branch. The same invalid image message
// then consumed every account and surfaced as a false 503 after pool exhaustion.
func TestHandleErrorResponse_Deterministic400SkipsAccountErrorPolicy(t *testing.T) {
	c, rec := newOpenAIUpstreamErrorTestContext(t)
	svc, account, repo, upstreamBody := newOpenAIInvalidImagePolicyHarness()

	_, err := svc.handleErrorResponse(
		context.Background(),
		newOpenAIUpstreamErrorResponse(http.StatusBadRequest, upstreamBody),
		c, account, []byte(`{"model":"gpt-5.6-terra"}`),
	)

	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "invalid_value", gjson.Get(rec.Body.String(), "error.code").String())
	require.Equal(t, "input[134].content[2]", gjson.Get(rec.Body.String(), "error.param").String())
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	requireOpenAIInvalidImagePolicyUntouched(t, repo, account)
}

func TestHandleCompatErrorResponse_Deterministic400SkipsAccountErrorPolicy(t *testing.T) {
	c, _ := newOpenAIUpstreamErrorTestContext(t)
	svc, account, repo, upstreamBody := newOpenAIInvalidImagePolicyHarness()
	var writtenStatus int
	var writtenType, writtenMessage string

	_, err := svc.handleCompatErrorResponse(
		newOpenAIUpstreamErrorResponse(http.StatusBadRequest, upstreamBody),
		c,
		account,
		func(_ *gin.Context, statusCode int, errType, message string) {
			writtenStatus = statusCode
			writtenType = errType
			writtenMessage = message
		},
		"gpt-5.6-terra",
	)

	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, writtenStatus)
	require.Equal(t, "invalid_request_error", writtenType)
	require.Contains(t, writtenMessage, "input_image")
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	requireOpenAIInvalidImagePolicyUntouched(t, repo, account)
}

func TestHandleErrorResponsePassthrough_Deterministic400SkipsAccountErrorPolicy(t *testing.T) {
	c, rec := newOpenAIUpstreamErrorTestContext(t)
	svc, account, repo, upstreamBody := newOpenAIInvalidImagePolicyHarness()
	resp := newOpenAIUpstreamErrorResponse(http.StatusBadRequest, upstreamBody)

	err := svc.handleErrorResponsePassthrough(
		context.Background(),
		resp,
		c,
		account,
		[]byte(`{"model":"gpt-5.6-terra"}`),
		[]byte(upstreamBody),
	)

	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	requireOpenAIInvalidImagePolicyUntouched(t, repo, account)
}
