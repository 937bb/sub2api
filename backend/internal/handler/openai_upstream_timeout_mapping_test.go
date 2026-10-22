package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAITimeoutFailoverExhaustionReturnsServiceUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, upstreamStatus := range []int{http.StatusRequestTimeout, http.StatusGatewayTimeout} {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		(&OpenAIGatewayHandler{}).handleFailoverExhausted(c, &service.UpstreamFailoverError{
			StatusCode: upstreamStatus,
		}, false)

		require.Equal(t, http.StatusServiceUnavailable, rec.Code)
		require.Contains(t, rec.Body.String(), "Upstream request timed out")
	}
}
