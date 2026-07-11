package apicompat

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResponsesAnthropicCacheCreationUsageNonStreaming(t *testing.T) {
	responsesUsage := &ResponsesUsage{
		InputTokens:              20,
		OutputTokens:             5,
		CacheCreationInputTokens: 6,
		InputTokensDetails:       &ResponsesInputTokensDetails{CachedTokens: 4},
	}

	anthropicUsage := anthropicUsageFromResponsesUsage(responsesUsage)
	require.Equal(t, 10, anthropicUsage.InputTokens)
	require.Equal(t, 4, anthropicUsage.CacheReadInputTokens)
	require.Equal(t, 6, anthropicUsage.CacheCreationInputTokens)

	converted := AnthropicToResponsesResponse(&AnthropicResponse{Usage: anthropicUsage})
	require.Equal(t, 20, converted.Usage.InputTokens)
	require.Equal(t, 4, converted.Usage.InputTokensDetails.CachedTokens)
	require.Equal(t, 6, converted.Usage.CacheCreationInputTokens)
}

func TestResponsesAnthropicCacheCreationUsageStreaming(t *testing.T) {
	t.Run("Responses to Anthropic", func(t *testing.T) {
		state := NewResponsesEventToAnthropicState()
		state.MessageStartSent = true
		events := ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
			Type: "response.completed",
			Response: &ResponsesResponse{Status: "completed", Usage: &ResponsesUsage{
				InputTokens:              20,
				OutputTokens:             5,
				CacheCreationInputTokens: 6,
				InputTokensDetails:       &ResponsesInputTokensDetails{CachedTokens: 4},
			}},
		}, state)

		require.Len(t, events, 2)
		require.Equal(t, 10, events[0].Usage.InputTokens)
		require.Equal(t, 4, events[0].Usage.CacheReadInputTokens)
		require.Equal(t, 6, events[0].Usage.CacheCreationInputTokens)
	})

	t.Run("Anthropic to Responses", func(t *testing.T) {
		state := NewAnthropicEventToResponsesState()
		state.CreatedSent = true
		state.InputTokens = 10
		state.OutputTokens = 5
		state.CacheReadInputTokens = 4
		state.CacheCreationInputTokens = 6

		events := FinalizeAnthropicResponsesStream(state)
		require.Len(t, events, 1)
		require.Equal(t, 20, events[0].Response.Usage.InputTokens)
		require.Equal(t, 4, events[0].Response.Usage.InputTokensDetails.CachedTokens)
		require.Equal(t, 6, events[0].Response.Usage.CacheCreationInputTokens)
	})

	t.Run("Responses top-level terminal usage", func(t *testing.T) {
		state := NewResponsesEventToAnthropicState()
		state.MessageStartSent = true
		events := ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
			Type: "response.completed",
			Usage: &ResponsesUsage{
				InputTokens:              20,
				OutputTokens:             5,
				CacheCreationInputTokens: 6,
				InputTokensDetails:       &ResponsesInputTokensDetails{CachedTokens: 4},
			},
		}, state)
		require.Len(t, events, 2)
		require.Equal(t, 10, events[0].Usage.InputTokens)
		require.Equal(t, 6, events[0].Usage.CacheCreationInputTokens)
	})

	t.Run("Responses synthetic finalization", func(t *testing.T) {
		state := NewResponsesEventToAnthropicState()
		state.MessageStartSent = true
		state.InputTokens = 10
		state.OutputTokens = 5
		state.CacheReadInputTokens = 4
		state.CacheCreationInputTokens = 6
		events := FinalizeResponsesAnthropicStream(state)
		require.Len(t, events, 2)
		require.Equal(t, 6, events[0].Usage.CacheCreationInputTokens)
	})
}
