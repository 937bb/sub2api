package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestIsOpenAIWSTokenEvent_TerminalEventsExcluded 覆盖 isOpenAIWSTokenEvent 的回归用例。
// 重点验证终止事件（response.completed / response.done）不再被当作 token event，
// 否则当上游没有可识别的 delta 时，firstTokenMs 会被填到终止时刻，
// 等于把"总耗时"误报为"首 token 延迟"（issue #2651）。
func TestIsOpenAIWSTokenEvent_TerminalEventsExcluded(t *testing.T) {
	cases := []struct {
		name      string
		eventType string
		want      bool
	}{
		{name: "empty", eventType: "", want: false},
		{name: "whitespace_trimmed_empty", eventType: "   ", want: false},

		{name: "response.created", eventType: "response.created", want: false},
		{name: "response.in_progress", eventType: "response.in_progress", want: false},
		{name: "response.output_item.added", eventType: "response.output_item.added", want: false},
		{name: "response.output_item.done", eventType: "response.output_item.done", want: false},

		{name: "terminal_response.completed", eventType: "response.completed", want: false},
		{name: "terminal_response.done", eventType: "response.done", want: false},
		{name: "terminal_response.completed_padded", eventType: "  response.completed  ", want: false},
		{name: "terminal_response.done_padded", eventType: "  response.done  ", want: false},

		{name: "delta_text", eventType: "response.output_text.delta", want: true},
		{name: "delta_audio_transcript", eventType: "response.audio_transcript.delta", want: true},
		{name: "delta_function_call_arguments", eventType: "response.function_call_arguments.delta", want: true},

		{name: "output_text_done", eventType: "response.output_text.done", want: true},
		{name: "output_text_annotation_added", eventType: "response.output_text.annotation.added", want: true},

		{name: "output_audio_done", eventType: "response.output_audio.done", want: true},

		{name: "reasoning_summary_delta", eventType: "response.reasoning_summary_text.delta", want: true},

		{name: "unrelated_event_error", eventType: "error", want: false},
		{name: "unknown_event_without_match", eventType: "response.reasoning_summary_part.added", want: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := isOpenAIWSTokenEvent(tc.eventType)
			require.Equal(t, tc.want, got, "isOpenAIWSTokenEvent(%q)", tc.eventType)
		})
	}
}

// TestIsOpenAIWSTokenEvent_DisjointWithTerminal 守护「token 事件集合与终止事件集合互斥」的不变量。
// firstTokenMs 的计算依赖于 isTokenEvent && !isTerminalEvent；
// 若两者再次出现交集，则 issue #2651 描述的 latency 误报会重现。
func TestIsOpenAIWSTokenEvent_DisjointWithTerminal(t *testing.T) {
	terminalEvents := []string{
		"response.completed",
		"response.done",
		"response.failed",
		"response.incomplete",
		"response.cancelled",
		"response.canceled",
	}
	for _, ev := range terminalEvents {
		ev := ev
		t.Run(ev, func(t *testing.T) {
			require.True(t, isOpenAIWSTerminalEvent(ev), "expected terminal event %q to be classified as terminal", ev)
			require.False(t, isOpenAIWSTokenEvent(ev), "terminal event %q must NOT be classified as token event (issue #2651)", ev)
		})
	}
}

func TestBuildOpenAIWSCreatePayload(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	req := map[string]any{
		"model":             "gpt-5-codex",
		"input":             []any{map[string]any{"role": "user", "content": "hi"}},
		"stream":            true,
		"background":        false,
		"store":             true,
		"max_output_tokens": 1024,
	}

	payload := svc.buildOpenAIWSCreatePayload(req, account)

	require.Equal(t, "response.create", payload["type"])
	require.NotContains(t, payload, "background")
	require.Equal(t, true, payload["stream"])
	require.NotContains(t, payload, "previous_response_id")
	require.NotContains(t, payload, "generate")
	require.NotContains(t, payload, "max_output_tokens")
	require.Equal(t, false, payload["store"])
	require.Equal(t, true, req["store"])
}

func TestApplyOpenAIOAuthWSAllowlistRawPreservesAllowedRawJSON(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	payload := []byte(`{"type":"response.create","model":"gpt-5-codex","tools":[{"type":"function","name":"big","parameters":{"const":9007199254740993}}],"unknown_field":"drop"}`)

	filtered, err := applyOpenAIOAuthWSAllowlistRaw(payload, account)

	require.NoError(t, err)
	require.Contains(t, string(filtered), `9007199254740993`)
	require.NotContains(t, string(filtered), "9007199254740992")
	require.NotContains(t, string(filtered), "unknown_field")
}

func TestApplyOpenAIOAuthWSAllowlistRawUsesAllowlistOrder(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	payload := []byte(`{"unknown_field":"drop","model":"gpt-5-codex","type":"response.create","input":"hi"}`)

	filtered, err := applyOpenAIOAuthWSAllowlistRaw(payload, account)

	require.NoError(t, err)
	filteredString := string(filtered)
	require.True(t, strings.Index(filteredString, `"type"`) < strings.Index(filteredString, `"model"`))
	require.True(t, strings.Index(filteredString, `"model"`) < strings.Index(filteredString, `"input"`))
	require.NotContains(t, filteredString, "unknown_field")
}

func TestBuildOpenAIWSCreatePayloadAPIKeyKeepsExistingBehavior(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	req := map[string]any{
		"model":             "gpt-5-codex",
		"stream":            false,
		"background":        false,
		"store":             true,
		"max_output_tokens": 1024,
	}

	payload := svc.buildOpenAIWSCreatePayload(req, account)

	require.Equal(t, "response.create", payload["type"])
	require.NotContains(t, payload, "background")
	require.Equal(t, false, payload["stream"])
	require.Equal(t, true, payload["store"])
	require.Equal(t, 1024, payload["max_output_tokens"])
}

func TestBuildOpenAIWSCreatePayloadPreservesExplicitWSFields(t *testing.T) {
	svc := &OpenAIGatewayService{}
	req := map[string]any{
		"model":                "gpt-5-codex",
		"previous_response_id": "resp_123",
		"generate":             true,
	}

	payload := svc.buildOpenAIWSCreatePayload(req, nil)

	require.Equal(t, "resp_123", payload["previous_response_id"])
	require.Equal(t, true, payload["generate"])
	require.Equal(t, true, payload["stream"])
}
