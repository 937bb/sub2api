package handler

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func writeModelNotFoundIfPureSupportMiss(c *gin.Context, err error) bool {
	return writeModelNotFoundIfPureSupportMissForModel(c, err, "")
}

func writeModelNotFoundIfPureSupportMissForModel(c *gin.Context, err error, publicRequestedModel string) bool {
	model, ok := service.ModelNotSupportedRequestedModel(err)
	if !ok {
		return false
	}
	if publicRequestedModel = strings.TrimSpace(publicRequestedModel); publicRequestedModel != "" {
		model = publicRequestedModel
	}
	message := fmt.Sprintf("The model %q was not found.", strings.TrimSpace(model))
	c.JSON(http.StatusNotFound, gin.H{
		"error": gin.H{
			"code":    "model_not_found",
			"type":    "model_not_found",
			"message": message,
		},
	})
	return true
}

func writeAnthropicModelNotFoundIfPureSupportMissForModel(c *gin.Context, err error, publicRequestedModel string) bool {
	model, ok := service.ModelNotSupportedRequestedModel(err)
	if !ok {
		return false
	}
	if publicRequestedModel = strings.TrimSpace(publicRequestedModel); publicRequestedModel != "" {
		model = publicRequestedModel
	}
	message := modelNotFoundMessage(model)
	c.JSON(http.StatusNotFound, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    "model_not_found",
			"message": message,
		},
	})
	return true
}

func writeGeminiModelNotFoundIfPureSupportMissForModel(c *gin.Context, err error, publicRequestedModel string) bool {
	model, ok := service.ModelNotSupportedRequestedModel(err)
	if !ok {
		return false
	}
	if publicRequestedModel = strings.TrimSpace(publicRequestedModel); publicRequestedModel != "" {
		model = publicRequestedModel
	}
	googleError(c, http.StatusNotFound, modelNotFoundMessage(model))
	return true
}

func handleModelNotFoundIfPureSupportMiss(c *gin.Context, h interface {
	handleStreamingAwareError(*gin.Context, int, string, string, bool)
}, err error, streamStarted bool) bool {
	return handleModelNotFoundIfPureSupportMissForModel(c, h, err, streamStarted, "")
}

func handleModelNotFoundIfPureSupportMissForModel(c *gin.Context, h interface {
	handleStreamingAwareError(*gin.Context, int, string, string, bool)
}, err error, streamStarted bool, publicRequestedModel string) bool {
	return handleModelNotFoundWithWriterIfPureSupportMissForModel(c, writeModelNotFoundIfPureSupportMissForModel, h.handleStreamingAwareError, err, streamStarted, publicRequestedModel)
}

func handleChatCompletionsModelNotFoundIfPureSupportMiss(c *gin.Context, h interface {
	handleStreamingAwareError(*gin.Context, int, string, string, bool)
}, err error, streamStarted bool) bool {
	return handleModelNotFoundWithWriterIfPureSupportMissForModel(c, writeModelNotFoundIfPureSupportMissForModel, h.handleStreamingAwareError, err, streamStarted, "")
}

func handleResponsesModelNotFoundIfPureSupportMiss(c *gin.Context, h interface {
	handleStreamingAwareError(*gin.Context, int, string, string, bool)
}, err error, streamStarted bool) bool {
	return handleModelNotFoundWithWriterIfPureSupportMissForModel(c, writeModelNotFoundIfPureSupportMissForModel, h.handleStreamingAwareError, err, streamStarted, "")
}

func handleAnthropicModelNotFoundIfPureSupportMissForModel(c *gin.Context, h interface {
	anthropicStreamingAwareError(*gin.Context, int, string, string, bool)
}, err error, streamStarted bool, publicRequestedModel string) bool {
	return handleModelNotFoundWithWriterIfPureSupportMissForModel(c, writeAnthropicModelNotFoundIfPureSupportMissForModel, h.anthropicStreamingAwareError, err, streamStarted, publicRequestedModel)
}

func handleGatewayMessagesModelNotFoundIfPureSupportMissForModel(c *gin.Context, h interface {
	anthropicStreamingAwareError(*gin.Context, int, string, string, bool)
}, err error, streamStarted bool, publicRequestedModel string) bool {
	return handleModelNotFoundWithWriterIfPureSupportMissForModel(c, writeAnthropicModelNotFoundIfPureSupportMissForModel, h.anthropicStreamingAwareError, err, streamStarted, publicRequestedModel)
}

func handleModelNotFoundWithWriterIfPureSupportMissForModel(
	c *gin.Context,
	writeNonStreamError func(*gin.Context, error, string) bool,
	writeStreamAwareError func(*gin.Context, int, string, string, bool),
	err error,
	streamStarted bool,
	publicRequestedModel string,
) bool {
	model, ok := service.ModelNotSupportedRequestedModel(err)
	if !ok {
		return false
	}
	if publicRequestedModel = strings.TrimSpace(publicRequestedModel); publicRequestedModel != "" {
		model = publicRequestedModel
	}
	message := fmt.Sprintf("The model %q was not found.", strings.TrimSpace(model))
	if !streamStarted {
		writeNonStreamError(c, err, model)
		return true
	}
	writeStreamAwareError(c, http.StatusNotFound, "model_not_found", message, streamStarted)
	return true
}

func modelNotFoundMessage(model string) string {
	return fmt.Sprintf("The model %q was not found.", strings.TrimSpace(model))
}
