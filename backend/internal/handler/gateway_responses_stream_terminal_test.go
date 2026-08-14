package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestHandleResponsesFailoverExhaustedWritesTerminalEventAfterStreamStarted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Header("Content-Type", "text/event-stream")
	_, err := c.Writer.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n"))
	require.NoError(t, err)

	h := &GatewayHandler{}
	h.handleResponsesFailoverExhausted(c, &service.UpstreamFailoverError{
		StatusCode:   http.StatusBadGateway,
		ResponseBody: []byte(`{"error":{"message":"upstream connection reset"}}`),
	}, true)

	body := rec.Body.String()
	require.Contains(t, body, "partial")
	require.Equal(t, 1, strings.Count(body, `"type":"response.failed"`))
	require.Contains(t, body, "All available accounts exhausted")
	streamErr, ok := service.GetOpsStreamError(c)
	require.True(t, ok)
	require.Equal(t, "server_error", streamErr.ErrType)
}
