package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const kiroEmptyResponseCode = "kiro_empty_response"
const kiroEmptyResponseMessage = "Kiro ended the upstream stream without assistant text or tool calls. The upstream did not provide a reason. Review the full request context, including automatically attached instructions and tools."

const kiroContentFilteredCode = "kiro_content_filtered"
const kiroContentFilteredMessage = "Kiro rejected the request with CONTENT_FILTERED. The evaluated input includes system instructions, conversation history, and tool definitions, not only the last message."

// Keep an observed empty stream distinct from a transport failure. Never infer
// a policy refusal from a context-only event or expose request contents.
func writeKiroEmptyResponse(c *gin.Context, err error, protocol string) bool {
	var empty *kiro.NoContentError
	var filtered *kiro.ContentFilteredError
	status, code, message, errorType, geminiStatus := http.StatusBadGateway, kiroEmptyResponseCode, kiroEmptyResponseMessage, "api_error", "INTERNAL"
	switch {
	case errors.As(err, &filtered):
		status, code, message, errorType, geminiStatus = http.StatusBadRequest, kiroContentFilteredCode, kiroContentFilteredMessage, "invalid_request_error", "INVALID_ARGUMENT"
		if category := filtered.SafeCategory(); category != "" {
			message += " Upstream refusal category: " + category + "."
		}
	case errors.As(err, &empty):
	default:
		return false
	}
	SetOpsUpstreamError(c, http.StatusOK, message, err.Error())
	MarkResponseCommitted(c)
	if protocol == "gemini" {
		payload := gin.H{"error": gin.H{"code": status, "status": geminiStatus, "message": code + ": " + message}}
		if c.Writer.Written() {
			MarkOpsStreamError(c, errorType, message, status)
			data, _ := json.Marshal(payload)
			_, _ = fmt.Fprintf(c.Writer, "data: %s\n\n", data)
			c.Writer.Flush()
		} else {
			c.JSON(status, payload)
		}
		return true
	}
	errorBody := gin.H{"type": errorType, "code": code, "message": message}
	if c.Writer.Written() {
		MarkOpsStreamError(c, errorType, message, status)
		if protocol == "responses" {
			c.SSEvent("response.failed", gin.H{"type": "response.failed", "sequence_number": 0, "response": gin.H{"id": "resp_" + uuid.NewString(), "object": "response", "created_at": time.Now().Unix(), "status": "failed", "output": []any{}, "error": gin.H{"code": code, "message": message}}})
		} else if protocol == "chat" {
			data, _ := json.Marshal(gin.H{"error": errorBody})
			_, _ = fmt.Fprintf(c.Writer, "data: %s\n\n", data)
		} else {
			c.SSEvent("error", gin.H{"type": "error", "error": errorBody})
		}
		c.Writer.Flush()
		return true
	}
	c.JSON(status, gin.H{"type": "error", "error": errorBody})
	return true
}
