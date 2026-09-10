package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type wsRetryPassthroughRepo struct {
	service.ErrorPassthroughRepository
	rule *model.ErrorPassthroughRule
}

func TestCancelledRequestWithPriorRetryRemainsClientFailure(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/responses", nil)
	service.SetOpsUpstreamError(c, 502, "earlier upstream overload", "")
	service.MarkOpenAIForwardTerminalFailure(c, &service.OpenAIForwardResult{ClientDisconnect: true}, context.Canceled)
	marked, ok := service.GetOpsStreamError(c)
	require.True(t, ok)
	phase, limited, owner, source := classifyOpsErrorLog(c, marked.ErrType, marked.Message, marked.Code, marked.IntendedStatus)
	require.Equal(t, "request", phase)
	require.False(t, limited)
	require.Equal(t, "client", owner)
	require.Equal(t, "client_request", source)
}

func (r wsRetryPassthroughRepo) List(context.Context) ([]*model.ErrorPassthroughRule, error) {
	return []*model.ErrorPassthroughRule{r.rule}, nil
}

func TestWSMappedRetryExhaustionHonorsSemanticPassthroughRule(t *testing.T) {
	const custom = "custom overload message"
	status := 502
	message := custom
	rules := service.NewErrorPassthroughService(wsRetryPassthroughRepo{rule: &model.ErrorPassthroughRule{
		Name: "semantic overload", Enabled: true, ErrorCodes: []int{503}, Keywords: []string{"overloaded"},
		Platforms: []string{"openai"}, ResponseCode: &status, CustomMessage: &message, SkipMonitoring: true,
	}}, nil)
	for _, started := range []bool{false, true} {
		t.Run(map[bool]string{false: "http", true: "stream"}[started], func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			(&OpenAIGatewayHandler{errorPassthroughService: rules}).handleFailoverExhausted(c, &service.UpstreamFailoverError{
				Reason: service.OpenAIWSMappedRetryReason, StatusCode: 502, ClientStatusCode: 502,
				ClientMessage: "Our servers are currently overloaded. Please try again later.", RequestScopedTransient: true,
				ResponseBody: []byte(`{"error":{"code":"server_is_overloaded","type":"service_unavailable_error","message":"Our servers are currently overloaded. Please try again later."}}`),
			}, started)
			require.Contains(t, rec.Body.String(), custom)
			require.NotContains(t, rec.Body.String(), "Our servers are currently overloaded")
			require.True(t, c.GetBool(service.OpsSkipPassthroughKey))
			if !started {
				require.Equal(t, 502, rec.Code)
			}
		})
	}
}

func TestWSMappedRetryExhaustionPreservesUpstreamMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const message = "Our servers are currently overloaded. Please try again later."
	for _, started := range []bool{false, true} {
		t.Run(map[bool]string{false: "http_error", true: "stream_error"}[started], func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			(&OpenAIGatewayHandler{}).handleFailoverExhausted(c, &service.UpstreamFailoverError{
				Reason: service.OpenAIWSMappedRetryReason, StatusCode: 502, ClientStatusCode: 502,
				ClientMessage: message, RequestScopedTransient: true,
				ResponseBody: []byte(`{"error":{"code":"server_is_overloaded","message":"` + message + `"}}`),
			}, started)
			if !started {
				require.Equal(t, http.StatusBadGateway, rec.Code)
				require.Equal(t, message, gjson.Get(rec.Body.String(), "error.message").String())
			} else {
				require.Equal(t, 1, strings.Count(rec.Body.String(), `"type":"response.failed"`))
				require.Contains(t, rec.Body.String(), message)
			}
			require.NotContains(t, rec.Body.String(), "Upstream request failed")
			require.NotContains(t, rec.Body.String(), "Upstream service temporarily unavailable")
		})
	}
}
