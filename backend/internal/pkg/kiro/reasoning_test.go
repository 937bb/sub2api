package kiro

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNativeReasoningRoundTripAndBlockOrder(t *testing.T) {
	frames := bytes.Join([][]byte{
		frameForTest("reasoningContentEvent", `{"signature":"opaque-upstream"}`),
		frameForTest("reasoningContentEvent", `{"text":"Think "}`),
		frameForTest("reasoningContentEvent", `{"text":"carefully."}`),
		frameForTest("reasoningContentEvent", `{"redactedContent":"opaque-ciphertext"}`),
		frameForTest("toolUseEvent", `{"toolUseId":"call1","name":"Read","input":"{}","stop":true}`),
		frameForTest("reasoningContentEvent", `{"text":"Verify result."}`),
		frameForTest("assistantResponseEvent", `{"content":"Done."}`),
	}, nil)
	var wire []map[string]any
	finalized := false
	options := DecodeOptionsForRequest([]byte(`{"thinking":{"type":"adaptive"}}`))
	out, err := DecodeWithOptions(bytes.NewReader(frames), "claude-opus-4.6", 100, func(_ string, event any) error {
		encoded, e := json.Marshal(event)
		require.NoError(t, e)
		var row map[string]any
		require.NoError(t, json.Unmarshal(encoded, &row))
		wire = append(wire, row)
		return nil
	}, func(out *Response) {
		finalized = true
		require.Equal(t, estimatedOutputTokens(out.Content), out.Usage.OutputTokens)
		out.UsageSource = "local_prefix_estimated"
	}, options, map[string]string{"Read": "Read"})
	require.NoError(t, err)
	require.True(t, finalized)
	require.Equal(t, []Block{
		{Type: "thinking", Thinking: "Think carefully.", Signature: "opaque-upstream"},
		{Type: "redacted_thinking", Data: "opaque-ciphertext"},
		{Type: "tool_use", ID: "call1", Name: "Read", Input: json.RawMessage(`{}`)},
		{Type: "thinking", Thinking: "Verify result.", Signature: thinkingSignaturePlaceholder},
		{Type: "text", Text: "Done."},
	}, out.Content)
	active := -1
	signed := false
	for _, event := range wire {
		index, _ := event["index"].(float64)
		switch event["type"] {
		case "content_block_start":
			require.Equal(t, -1, active, "blocks cannot overlap")
			active = int(index)
			signed = false
		case "content_block_delta":
			require.Equal(t, active, int(index))
			delta, ok := event["delta"].(map[string]any)
			require.True(t, ok)
			if delta["type"] == "signature_delta" {
				signed = true
			}
		case "content_block_stop":
			require.Equal(t, active, int(index))
			if out.Content[active].Type == "thinking" {
				require.True(t, signed, "signature must precede block stop")
			}
			active = -1
		}
	}
	require.Equal(t, -1, active)
	// Clients can return the complete assistant content with tool results. Opaque
	// thinking signatures and ciphertext must not enter the upstream history.
	raw, err := json.Marshal(map[string]any{"max_tokens": 1024, "messages": []any{
		map[string]any{"role": "user", "content": "Read a file"},
		map[string]any{"role": "assistant", "content": out.Content},
		map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": "call1", "content": "OK"}}},
	}})
	require.NoError(t, err)
	body, _, err := Convert(raw, "claude-opus-4.6", "session", nil)
	require.NoError(t, err)
	require.NotContains(t, string(body), "opaque-")
	require.Contains(t, string(body), "Done.")
}

func TestNativeReasoningVisibilityAndUsage(t *testing.T) {
	frames := append(frameForTest("reasoningContentEvent", `{"text":"Hidden thought","signature":"private","redactedContent":"encrypted"}`), frameForTest("assistantResponseEvent", `{"content":"OK"}`)...)
	for _, raw := range []string{`{}`, `{"thinking":{"type":"disabled"}}`, `{"thinking":{"type":"enabled","display":"omitted"}}`} {
		out, err := DecodeWithOptions(bytes.NewReader(frames), "model", 20, nil, nil, DecodeOptionsForRequest([]byte(raw)), nil)
		require.NoError(t, err)
		require.Equal(t, []Block{{Type: "text", Text: "OK"}}, out.Content)
		require.Equal(t, EstimateTokens("Hidden thought")+EstimateTokens("OK")+8, out.Usage.OutputTokens)
	}
	frames = append(frames, frameForTest("meteringEvent", `{"inputTokens":20,"outputTokens":99}`)...)
	out, err := DecodeWithOptions(bytes.NewReader(frames), "model", 20, nil, nil, DecodeOptions{ThinkingEnabled: true}, nil)
	require.NoError(t, err)
	require.Equal(t, 99, out.Usage.OutputTokens, "provider output usage must win")
}

func TestNativeReasoningErrorsAndSignatureOnly(t *testing.T) {
	for _, payload := range []string{`{"signature":"private"}`, `{"text":""}`, `{}`} {
		out, err := DecodeWithOptions(bytes.NewReader(frameForTest("reasoningContentEvent", payload)), "model", 20, nil, nil, DecodeOptions{ThinkingEnabled: true}, nil)
		var empty *NoContentError
		require.ErrorAs(t, err, &empty)
		require.Empty(t, out.Content)
	}
	_, err := DecodeWithOptions(bytes.NewReader(frameForTest("reasoningContentEvent", `{"text":42}`)), "model", 20, nil, nil, DecodeOptions{ThinkingEnabled: true}, nil)
	require.ErrorContains(t, err, "invalid Kiro reasoning event")
	sentinel := errors.New("client canceled")
	finalized := false
	_, err = DecodeWithOptions(bytes.NewReader(frameForTest("reasoningContentEvent", `{"text":"thinking"}`)), "model", 20, func(name string, _ any) error {
		if name == "content_block_delta" {
			return sentinel
		}
		return nil
	}, func(*Response) { finalized = true }, DecodeOptions{ThinkingEnabled: true}, nil)
	require.ErrorIs(t, err, sentinel)
	require.False(t, finalized)
}

func TestNativeReasoningRefusalDoesNotBill(t *testing.T) {
	frames := append(frameForTest("reasoningContentEvent", `{"text":"thinking"}`), frameForTest("metadataEvent", `{"stopReason":"CONTENT_FILTERED"}`)...)
	finalized := false
	out, err := DecodeWithOptions(bytes.NewReader(frames), "model", 20, nil, func(*Response) { finalized = true }, DecodeOptions{ThinkingEnabled: true}, nil)
	require.NoError(t, err)
	require.Equal(t, "refusal", out.StopReason)
	require.Zero(t, out.Usage)
	require.False(t, finalized)
}

func TestReasoningOptions(t *testing.T) {
	for _, mode := range []string{"enabled", "adaptive", "between_tools"} {
		require.True(t, DecodeOptionsForRequest([]byte(`{"thinking":{"type":"`+mode+`"}}`)).ThinkingEnabled)
	}
	for _, raw := range []string{`{}`, `{"thinking":null}`, `{"thinking":false}`, `{"thinking":{"type":"disabled"}}`} {
		require.False(t, DecodeOptionsForRequest([]byte(raw)).ThinkingEnabled)
	}
	require.Zero(t, estimatedOutputTokens([]Block{{Type: "thinking", Signature: strings.Repeat("x", 10000)}}), "signature size is not output usage")
}
