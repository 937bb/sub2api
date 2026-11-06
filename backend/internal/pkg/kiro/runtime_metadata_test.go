package kiro

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRuntimeContentFilteredNeverCompletes(t *testing.T) {
	for _, partial := range []bool{false, true} {
		var frames []byte
		if partial {
			frames = frameForTest("assistantResponseEvent", `{"content":"partial"}`)
		}
		frames = append(frames, frameForTest("metadataEvent", `{"stopReason":"CONTENT_FILTERED","private":"do not log"}`)...)
		frames = append(frames, frameForTest("contextUsageEvent", `{"contextUsagePercentage":7.4}`)...)
		var emitted []string
		finalized := false
		_, err := DecodeWithUsageFinalizer(bytes.NewReader(frames), "claude-opus-5.5", 123, func(name string, _ any) error { emitted = append(emitted, name); return nil }, func(*Response) { finalized = true }, nil)
		var filtered *ContentFilteredError
		require.ErrorAs(t, err, &filtered)
		require.NotContains(t, err.Error(), "private")
		require.NotContains(t, emitted, "message_stop")
		require.NotContains(t, emitted, "message_delta")
		if !partial {
			require.Empty(t, emitted)
		}
		require.False(t, finalized)
	}
}

func TestRuntimeMetadataDoesNotInventFiltering(t *testing.T) {
	for _, metadata := range []string{`{}`, `{"stopReason":"END_TURN"}`, `{"stopReason":"unknown"}`} {
		_, err := Decode(bytes.NewReader(frameForTest("metadataEvent", metadata)), "model", 1, nil)
		var empty *NoContentError
		require.ErrorAs(t, err, &empty)
	}
	for _, metadata := range []string{`{"stopReason":123}`, `{`} {
		_, err := Decode(bytes.NewReader(frameForTest("metadataEvent", metadata)), "model", 1, nil)
		require.ErrorContains(t, err, "invalid Kiro metadata")
	}
}

func TestRuntimeMetadataPreservesToolAndTokenStops(t *testing.T) {
	tool := frameForTest("toolUseEvent", `{"name":"read_probe","toolUseId":"call1","input":"{}","isComplete":true}`)
	wire := append(tool, frameForTest("metadataEvent", `{"stopReason":"END_TURN"}`)...)
	out, err := Decode(bytes.NewReader(wire), "model", 1, nil)
	require.NoError(t, err)
	require.Equal(t, "tool_use", out.StopReason)
	require.Equal(t, "read_probe", out.Content[0].Name)
	wire = append(frameForTest("assistantResponseEvent", `{"content":"partial"}`), frameForTest("metadataEvent", `{"stopReason":"MAX_TOKENS"}`)...)
	out, err = Decode(bytes.NewReader(wire), "model", 1, nil)
	require.NoError(t, err)
	require.Equal(t, "max_tokens", out.StopReason)
}

func TestKiroModelContextSuffixIsNotAnUpstreamSKU(t *testing.T) {
	for _, name := range []string{"claude-opus-5.5", "claude-opus-5-5", "claude-opus-5.5[1m]", "kiro/claude-opus-5-5[1m]"} {
		require.Equal(t, "claude-opus-5.5", ModelID(name))
	}
	require.Equal(t, "gpt-5.6-sol", ModelID("gpt-5.6-sol"))
}
