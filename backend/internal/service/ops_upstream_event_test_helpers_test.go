package service

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func requireLastOpsUpstreamErrorEvent(t *testing.T, c *gin.Context) *OpsUpstreamErrorEvent {
	t.Helper()

	raw, ok := c.Get(OpsUpstreamErrorsKey)
	require.True(t, ok)
	events, ok := raw.([]*OpsUpstreamErrorEvent)
	require.True(t, ok)
	require.NotEmpty(t, events)
	require.NotNil(t, events[len(events)-1])
	return events[len(events)-1]
}
