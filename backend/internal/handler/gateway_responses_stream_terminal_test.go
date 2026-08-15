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

func TestHandleFailoverExhaustedPreservesClassifiedOverload503(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, tt := range []struct {
		name                   string
		requestScopedTransient bool
		wantStatus             int
	}{
		{name: "classified model load", requestScopedTransient: true, wantStatus: http.StatusServiceUnavailable},
		{name: "generic upstream 503", requestScopedTransient: false, wantStatus: http.StatusBadGateway},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

			h := &OpenAIGatewayHandler{}
			h.handleFailoverExhausted(c, &service.UpstreamFailoverError{
				StatusCode:             http.StatusServiceUnavailable,
				ResponseBody:           []byte(`{"error":{"message":"Our servers are currently overloaded. Please try again later."}}`),
				RequestScopedTransient: tt.requestScopedTransient,
			}, false)

			require.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}
