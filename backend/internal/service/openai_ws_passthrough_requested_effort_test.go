package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestPassthroughPreservesRequestedEffortAcrossMappedTurns(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)
	upstream := newStagedPassthroughConn()
	results := make(chan *OpenAIForwardResult, 2)
	hooks := &OpenAIWSIngressHooks{
		MaxReasoningEffort: "low",
		AfterTurn: func(_ int, result *OpenAIForwardResult, err error) {
			if err == nil && result != nil {
				results <- result
			}
		},
	}
	server, _ := startPassthroughHookRecordingServer(
		t, controlCtx, newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream),
		passthroughLifecycleAccount(), hooks,
	)
	defer server.Close()
	client := dialPassthroughLifecycleClientWithPayload(t, server,
		`{"type":"response.create","model":"gpt-5.1","reasoning":{"effort":"high"}}`)
	defer func() { _ = client.CloseNow() }()

	for i, requested := range []string{"high", "medium"} {
		if i > 0 {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			err := client.Write(ctx, coderws.MessageText,
				[]byte(`{"type":"response.create","reasoning":{"effort":"medium"}}`))
			cancel()
			require.NoError(t, err)
		}
		payload := requirePassthroughUpstreamWrite(t, upstream, time.Second)
		require.Equal(t, "low", gjson.GetBytes(payload, "reasoning.effort").String())
		upstream.Send(fmt.Sprintf(`{"type":"response.completed","response":{"id":"resp_effort_%d","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`, i+1))
		event, err := readPassthroughLifecycleFrame(t, client, time.Second)
		require.NoError(t, err)
		require.Equal(t, "response.completed", gjson.GetBytes(event, "type").String())
		select {
		case result := <-results:
			require.NotNil(t, result.RequestedReasoningEffort)
			require.Equal(t, requested, *result.RequestedReasoningEffort)
			require.NotNil(t, result.ReasoningEffort)
			require.Equal(t, "low", *result.ReasoningEffort)
		case <-time.After(time.Second):
			t.Fatal("missing passthrough usage result")
		}
	}
}
