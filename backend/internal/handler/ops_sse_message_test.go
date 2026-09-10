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

const overloadMessage = "Our servers are currently overloaded. Please try again later."
const overloadSSEFrame = "data: {\"type\":\"error\",\"error\":{\"code\":\"server_is_overloaded\",\"type\":\"service_unavailable_error\",\"message\":\"" + overloadMessage + "\"},\"sequence_number\":2}\n\n"
const genericFailedSSEFrame = "event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"upstream_error\",\"message\":\"Upstream request failed\"}}}\n\n"

func TestParseOpsSSEFailure_PreservesUpstreamMessageBeforeGenericTerminal(t *testing.T) {
	for _, separator := range []string{"\n", "\r\n", "\r"} {
		body := strings.ReplaceAll(overloadSSEFrame+genericFailedSSEFrame, "\n", separator)
		parsed, ok := parseOpsSSEFailure([]byte(body))
		require.True(t, ok)
		require.Equal(t, overloadMessage, parsed.Message)
		require.Equal(t, "upstream_error", parsed.Code, "keep terminal classification")
		require.Equal(t, "upstream_error", parsed.ErrorType)
	}

	customTerminal := strings.ReplaceAll(genericFailedSSEFrame, "Upstream request failed", "Custom rule message")
	parsed, ok := parseOpsSSEFailure([]byte(overloadSSEFrame + customTerminal))
	require.True(t, ok)
	require.Equal(t, "Custom rule message", parsed.Message, "explicit terminal messages remain authoritative")

	parsed, ok = parseOpsSSEFailure([]byte(genericFailedSSEFrame))
	require.True(t, ok)
	require.Equal(t, "Upstream request failed", parsed.Message, "missing upstream message keeps the fallback")
}

func TestOpsErrorLoggerMiddleware_PreservesUpstreamMessageBeforeGenericTerminal(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusBadGateway} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			setupOpsErrorLogTestQueue(t, 2)
			gin.SetMode(gin.TestMode)
			ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
			router := gin.New()
			router.Use(OpsErrorLoggerMiddleware(ops))
			router.POST("/v1/responses", func(c *gin.Context) {
				c.Status(status)
				for _, frame := range []string{overloadSSEFrame, genericFailedSSEFrame} {
					_, _ = c.Writer.WriteString(frame[:len(frame)/2])
					_, _ = c.Writer.WriteString(frame[len(frame)/2:])
				}
			})
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/responses", nil))
			require.Equal(t, status, recorder.Code)
			require.Equal(t, int64(1), OpsErrorLogQueueLength())
			job := <-opsErrorLogQueue
			require.Equal(t, overloadMessage, job.entry.ErrorMessage)
			require.NotNil(t, job.entry.UpstreamErrorMessage)
			require.Equal(t, overloadMessage, *job.entry.UpstreamErrorMessage)
			require.Equal(t, http.StatusBadGateway, job.entry.StatusCode)
		})
	}
}

func TestOpsCaptureWriter_PreservesMessageWhenOnlyFallbackRemainsInProbe(t *testing.T) {
	state := &opsCaptureWriterState{limit: opsCaptureWriterLimit}
	state.captureResponseChunk([]byte(overloadSSEFrame), http.StatusOK)
	state.captureResponseChunk([]byte(genericFailedSSEFrame), http.StatusOK)
	state.finalizeResponseCapture()
	require.True(t, state.terminalFound)
	require.Equal(t, overloadMessage, state.terminalError.Message)
}

func TestOpenAIEnsureForwardErrorResponse_PreservesDeliveredUpstreamFailure(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	terminal := strings.ReplaceAll(genericFailedSSEFrame, "Upstream request failed", overloadMessage)
	_, _ = c.Writer.WriteString(overloadSSEFrame + terminal)
	service.MarkResponseCommitted(c)
	before := c.Writer.Size()
	require.False(t, (&OpenAIGatewayHandler{}).ensureForwardErrorResponse(c, true))
	require.Equal(t, before, c.Writer.Size(), "do not append another terminal frame")
	require.NotContains(t, recorder.Body.String(), "Upstream request failed")
}
