package service

import (
	"sync/atomic"

	"github.com/gin-gonic/gin"
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
