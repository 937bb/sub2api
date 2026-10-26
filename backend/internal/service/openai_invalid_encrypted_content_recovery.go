package service

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type openAIInvalidEncryptedContentSignal struct {
	eventType string
	payload   []byte
	message   string
}

func (e *openAIInvalidEncryptedContentSignal) Error() string {
	if e == nil || strings.TrimSpace(e.message) == "" {
		return "upstream rejected encrypted content"
	}
	return e.message
}

func asOpenAIInvalidEncryptedContentSignal(err error) (*openAIInvalidEncryptedContentSignal, bool) {
	var signal *openAIInvalidEncryptedContentSignal
	if !errors.As(err, &signal) || signal == nil {
		return nil, false
	}
	return signal, true
}

func newOpenAIInvalidEncryptedContentSignal(eventType string, payload []byte, message string) error {
	if openAIStreamFailedEventErrorCode(payload) != "invalid_encrypted_content" {
		return nil
	}
	return &openAIInvalidEncryptedContentSignal{
		eventType: strings.TrimSpace(eventType),
		payload:   append([]byte(nil), payload...),
		message:   sanitizeUpstreamErrorMessage(strings.TrimSpace(message)),
	}
}

// recoverOpenAIInvalidEncryptedContentRequest removes every recoverable stale
// encrypted artifact in one pass. This avoids a sequence of one-error-per-item
// retries when a conversation contains reasoning and tool output ciphertext.
func recoverOpenAIInvalidEncryptedContentRequest(reqBody map[string]any, upstreamBody []byte) (bool, string) {
	if len(reqBody) == 0 {
		return false, ""
	}
	actions := make([]string, 0, 3)
	if len(upstreamBody) > 0 && dropOpenAIRejectedAgentItems(reqBody, upstreamBody) {
		actions = append(actions, "rejected_agent_items")
	}
	if trimOpenAIEncryptedReasoningItems(reqBody) {
		actions = append(actions, "encrypted_reasoning")
	}
	if replaced := trimOpenAIEncryptedToolOutputParts(reqBody); replaced > 0 {
		actions = append(actions, fmt.Sprintf("encrypted_tool_output:%d", replaced))
	}
	return len(actions) > 0, strings.Join(actions, ",")
}

// writeOpenAIInvalidEncryptedContentSignal preserves the original terminal SSE
// if recovery is impossible or the single retry has already been consumed.
func writeOpenAIInvalidEncryptedContentSignal(c *gin.Context, signal *openAIInvalidEncryptedContentSignal) {
	if c == nil || c.Writer == nil || signal == nil || len(signal.payload) == 0 {
		return
	}
	eventType := strings.TrimSpace(signal.eventType)
	if eventType == "" {
		eventType = "response.failed"
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	_, _ = fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", eventType, signal.payload)
	if flusher, ok := c.Writer.(http.Flusher); ok {
		flusher.Flush()
	}
	MarkResponseCommitted(c)
}
