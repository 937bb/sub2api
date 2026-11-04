package handler

import (
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *OpenAIGatewayHandler) handleConversationBindingError(c *gin.Context, err error, streamStarted, anthropic bool) bool {
	var bindingErr *service.OpenAIConversationError
	if !errors.As(err, &bindingErr) {
		return false
	}
	if bindingErr.Status == 503 {
		c.Header("Retry-After", "5")
	}
	if anthropic {
		h.anthropicStreamingAwareError(c, bindingErr.Status, bindingErr.Code, bindingErr.Message, streamStarted)
	} else {
		h.handleStreamingAwareErrorWithCode(c, bindingErr.Status, "invalid_request_error", bindingErr.Code, bindingErr.Message, streamStarted, false)
	}
	return true
}
