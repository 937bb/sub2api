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

// Keep an observed empty stream distinct from a transport failure. Never infer
// a policy refusal from a context-only event or expose request contents.
func writeKiroEmptyResponse(c *gin.Context, err error, protocol string) bool {
	var empty *kiro.NoContentError
	if !errors.As(err, &empty) {
		return false
	}
	SetOpsUpstreamError(c, http.StatusOK, kiroEmptyResponseMessage, empty.Error())
	MarkResponseCommitted(c)
	if protocol == "gemini" {
		payload := gin.H{"error": gin.H{"code": http.StatusBadGateway, "status": "INTERNAL", "message": kiroEmptyResponseCode + ": " + kiroEmptyResponseMessage}}
		if c.Writer.Written() {
			MarkOpsStreamError(c, "api_error", kiroEmptyResponseMessage, http.StatusBadGateway)
			data, _ := json.Marshal(payload)
			_, _ = fmt.Fprintf(c.Writer, "data: %s\n\n", data)
			c.Writer.Flush()
		} else {
			c.JSON(http.StatusBadGateway, payload)
		}
		return true
	}
	errorBody := gin.H{"type": "api_error", "code": kiroEmptyResponseCode, "message": kiroEmptyResponseMessage}
	if c.Writer.Written() {
		MarkOpsStreamError(c, "api_error", kiroEmptyResponseMessage, http.StatusBadGateway)
		if protocol == "responses" {
			c.SSEvent("response.failed", gin.H{"type": "response.failed", "sequence_number": 0, "response": gin.H{"id": "resp_" + uuid.NewString(), "object": "response", "created_at": time.Now().Unix(), "status": "failed", "output": []any{}, "error": gin.H{"code": kiroEmptyResponseCode, "message": kiroEmptyResponseMessage}}})
		} else if protocol == "chat" {
			data, _ := json.Marshal(gin.H{"error": errorBody})
			_, _ = fmt.Fprintf(c.Writer, "data: %s\n\n", data)
		} else {
			c.SSEvent("error", gin.H{"type": "error", "error": errorBody})
		}
		c.Writer.Flush()
		return true
	}
	c.JSON(http.StatusBadGateway, gin.H{"type": "error", "error": errorBody})
	return true
}
