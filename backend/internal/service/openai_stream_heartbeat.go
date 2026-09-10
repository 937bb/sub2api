package service

import (
	"bytes"
	"net/http"
	"sync/atomic"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const openAIStreamHeartbeatTrackerKey = "openai_stream_heartbeat_tracker"

type openAIStreamHeartbeatTracker struct {
	bytes atomic.Int64
}

func recordOpenAIStreamHeartbeatBytes(c *gin.Context, written int) {
	if c == nil || written <= 0 {
		return
	}
	value, ok := c.Get(openAIStreamHeartbeatTrackerKey)
	if !ok {
		tracker := &openAIStreamHeartbeatTracker{}
		c.Set(openAIStreamHeartbeatTrackerKey, tracker)
		value = tracker
	}
	tracker, _ := value.(*openAIStreamHeartbeatTracker)
	if tracker != nil {
		tracker.bytes.Add(int64(written))
	}
}

func openAIStreamHeartbeatBytes(c *gin.Context) int {
	if c == nil {
		return 0
	}
	value, ok := c.Get(openAIStreamHeartbeatTrackerKey)
	if !ok {
		return 0
	}
	tracker, _ := value.(*openAIStreamHeartbeatTracker)
	if tracker == nil {
		return 0
	}
	written := tracker.bytes.Load()
	if written <= 0 {
		return 0
	}
	return int(written)
}

// OpenAIStreamHeartbeatPresent reports whether the gateway has committed only
// transport-level SSE comments while waiting for semantic upstream output.
func OpenAIStreamHeartbeatPresent(c *gin.Context) bool {
	return openAIStreamHeartbeatBytes(c) > 0
}

// OpenAIStreamAdjustedWrittenSize excludes transport-level SSE comments from
// response-size decisions. Heartbeats keep intermediaries alive, but they must
// not disable pre-output failover or suppress a terminal response.failed event.
func OpenAIStreamAdjustedWrittenSize(c *gin.Context) int {
	size := OpenAICompactKeepaliveAdjustedWrittenSize(c)
	if size < 0 {
		return size
	}
	real := size - openAIStreamHeartbeatBytes(c)
	if real > 0 {
		return real
	}
	return -1
}

// OpenAIStreamHasCommittedOutput distinguishes actual downstream output from
// SSE keepalives and attempt-local staged errors. Header-only commits without
// a tracked keepalive retain their previous no-replay semantics.
func OpenAIStreamHasCommittedOutput(c *gin.Context) bool {
	if c == nil {
		return false
	}
	return openAIStreamWriterHasCommittedOutput(c, c.Writer)
}

func openAIStreamWriterHasCommittedOutput(c *gin.Context, writer gin.ResponseWriter) bool {
	for {
		staged, ok := writer.(*oauthRetryWriter)
		if !ok {
			break
		}
		writer = staged.ResponseWriter
	}
	if writer == nil {
		return false
	}
	written, size := writer.Written(), writer.Size()
	heartbeatBytes := openAIStreamHeartbeatBytes(c)
	if c != nil {
		if value, ok := c.Get(openAIStreamKeepaliveBytesKey); ok {
			count, _ := value.(int)
			heartbeatBytes += count
		}
		if value, ok := c.Get(openAICompactSSEKeepaliveKey); ok {
			if keepalive, ok := value.(*openAICompactSSEKeepalive); ok && keepalive != nil {
				keepalive.mu.Lock()
				written, size = keepalive.writer.Written(), keepalive.writer.Size()
				heartbeatBytes += keepalive.bytes
				keepalive.mu.Unlock()
			}
		}
	}
	return written && (heartbeatBytes <= 0 || size > heartbeatBytes)
}

// Once a heartbeat has sent HTTP 200, a final HTTP error must terminate the
// already-open Responses SSE protocol. Failed attempts stay private; only the
// retry owner (or a final, non-retryable failure) calls this function.
func writeOpenAIErrorAfterHeartbeat(c *gin.Context, writer gin.ResponseWriter, status int, body []byte) error {
	message := extractOpenAISSEErrorMessage(body)
	if message == "" {
		if value := gjson.GetBytes(body, "error"); value.Type == gjson.String {
			message = value.String()
		} else if !gjson.ValidBytes(body) {
			message = string(bytes.TrimSpace(body))
		}
	}
	if message == "" {
		message = http.StatusText(status)
	}
	payload := buildOpenAIResponseFailedEvent("", "", body, message)
	if sanitized, changed := sanitizeOpenAICapacityShedErrorCodeForClient(payload); changed {
		payload = sanitized
	}
	frame := append([]byte("event: response.failed\ndata: "), payload...)
	frame = append(frame, '\n', '\n')
	_, err := writer.Write(frame)
	if err != nil {
		return err
	}
	writer.Flush()
	MarkOpsStreamFailure(c,
		gjson.GetBytes(payload, "response.error.type").String(),
		gjson.GetBytes(payload, "response.error.code").String(),
		gjson.GetBytes(payload, "response.error.message").String(), status)
	MarkResponseCommitted(c)
	return nil
}
