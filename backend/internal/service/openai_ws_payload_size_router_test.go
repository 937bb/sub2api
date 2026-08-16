package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpenAIWSPayloadSizeRouterLearnsSmallestFailure(t *testing.T) {
	now := time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC)
	router := openAIWSPayloadSizeRouter{nowFunc: func() time.Time { return now }}
	wsURL := "wss://ChatGPT.com/backend-api/codex/responses"

	threshold, updated := router.observeRemoteMessageTooBig(wsURL, openAIWSPayloadSizeMinFailureSample-1)
	require.Zero(t, threshold)
	require.False(t, updated)

	threshold, updated = router.observeRemoteMessageTooBig(wsURL, 100_000)
	require.Equal(t, 95_000, threshold)
	require.True(t, updated)

	gotThreshold, bypass := router.shouldBypass(wsURL, 94_999)
	require.Equal(t, 95_000, gotThreshold)
	require.False(t, bypass)

	gotThreshold, bypass = router.shouldBypass(wsURL, 95_000)
	require.Equal(t, 95_000, gotThreshold)
	require.True(t, bypass)

	threshold, updated = router.observeRemoteMessageTooBig(wsURL, 120_000)
	require.Equal(t, 95_000, threshold)
	require.False(t, updated)

	threshold, updated = router.observeRemoteMessageTooBig(wsURL, 80_000)
	require.Equal(t, 76_000, threshold)
	require.True(t, updated)
}

func TestOpenAIWSPayloadSizeRouterExpiresLearning(t *testing.T) {
	now := time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC)
	router := openAIWSPayloadSizeRouter{nowFunc: func() time.Time { return now }}
	wsURL := "wss://chatgpt.com/backend-api/codex/responses"

	_, updated := router.observeRemoteMessageTooBig(wsURL, 100_000)
	require.True(t, updated)

	now = now.Add(openAIWSPayloadSizeLearningTTL + time.Second)
	threshold, bypass := router.shouldBypass(wsURL, 100_000)
	require.Zero(t, threshold)
	require.False(t, bypass)
}

func TestOpenAIWSPayloadSizeRouteKey(t *testing.T) {
	require.Equal(
		t,
		"chatgpt.com/backend-api/codex/responses",
		openAIWSPayloadSizeRouteKey("WSS://ChatGPT.com/backend-api/codex/responses/"),
	)
}
