package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSmartWSTerminalErrorDoesNotAppendFallback(t *testing.T) {
	for _, body := range []string{
		`{"error":{"type":"invalid_request_error","code":"invalid_value","param":"input[0].content[9].file_data","message":"unsupported MIME type image/mpo"}}`,
		"event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"error\":{\"message\":\"Encrypted function output content could not be decrypted or decoded.\"}}}\n\n",
	} {
		t.Run(body, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, EndpointResponses, nil)
			before := c.Writer.Size()
			_, errWrite := c.Writer.WriteString(body)
			require.NoError(t, errWrite)
			err := fmt.Errorf("outer: %w", &service.PluginTransportError{Code: "SMART_WS_REQUEST_SENT", Message: "upstream rejected input", RequestSent: true, TerminalResponse: true})
			h := &OpenAIGatewayHandler{}
			if !openAIForwardErrorAlreadyCommunicated(c, before, err) {
				h.ensureForwardErrorResponse(c, false)
			}
			require.Equal(t, body, rec.Body.String())
			if body[0] == '{' {
				require.True(t, gjson.ValidBytes(rec.Body.Bytes()))
			}
		})
	}
}

func TestSmartWSWithoutTerminalWriteStillNeedsFallback(t *testing.T) {
	for _, terminal := range []bool{false, true} {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, EndpointResponses, nil)
		before := c.Writer.Size()
		err := &service.PluginTransportError{Code: "SMART_WS_REQUEST_SENT", RequestSent: true, TerminalResponse: terminal}
		require.False(t, openAIForwardErrorAlreadyCommunicated(c, before, err))
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	before := c.Writer.Size()
	_, _ = c.Writer.WriteString(":\n\n")
	require.False(t, openAIForwardErrorAlreadyCommunicated(c, before, &service.PluginTransportError{RequestSent: true}))
}

func TestSmartWSRateLimitRemains429WithoutReplay(t *testing.T) {
	for _, heartbeat := range []bool{false, true} {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, EndpointResponses, nil)
		if heartbeat {
			_, _ = c.Writer.WriteString(":\n\n")
			c.Writer.Flush()
		}
		h := &OpenAIGatewayHandler{}
		err := fmt.Errorf("outer: %w", &service.PluginTransportError{RequestSent: true, RateLimited: true})
		require.True(t, h.ensureForwardErrorResponse(c, false, err))
		require.Contains(t, rec.Body.String(), "rate_limit_exceeded")
		require.NotContains(t, rec.Body.String(), "upstream_error")
		if heartbeat {
			require.Equal(t, http.StatusOK, rec.Code)
			require.Contains(t, rec.Body.String(), "event: response.failed")
		} else {
			require.Equal(t, http.StatusTooManyRequests, rec.Code)
			require.True(t, gjson.ValidBytes(rec.Body.Bytes()))
		}
	}
}
