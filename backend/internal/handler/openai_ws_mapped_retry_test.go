package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

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
