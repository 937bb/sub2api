package service

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAICompactSSEKeepaliveCleanupRestoresOwnedWriter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/openai/v1/responses/compact", nil)
	c.Set(openAICompactClientStreamKey, true)
	original := c.Writer

	stop := startOpenAICompactSSEKeepalive(c, time.Hour)
	require.NotSame(t, original, c.Writer)

	stop()
	require.Same(t, original, c.Writer)
}

func TestOpenAICompactSSEKeepaliveCleanupPreservesLaterWriter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/openai/v1/responses/compact", nil)
	c.Set(openAICompactClientStreamKey, true)

	stop := startOpenAICompactSSEKeepalive(c, time.Hour)
	later := &openAICompactKeepaliveWriter{ResponseWriter: c.Writer}
	c.Writer = later

	stop()
	require.Same(t, later, c.Writer)
}

func TestStopOpenAICompactSSEKeepaliveCommittedSynchronizesHeartbeat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/openai/v1/responses/compact", nil)
	c.Set(openAICompactClientStreamKey, true)
	stop := startOpenAICompactSSEKeepalive(c, time.Millisecond)
	t.Cleanup(stop)

	require.Eventually(t, func() bool { return c.Writer.Written() }, time.Second, time.Millisecond)
	require.True(t, StopOpenAICompactSSEKeepaliveCommitted(c))
	before := rec.Body.String()
	time.Sleep(10 * time.Millisecond)
	require.Equal(t, before, rec.Body.String(), "no heartbeat may write after helper returns")
}
