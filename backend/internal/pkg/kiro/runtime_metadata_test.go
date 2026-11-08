package kiro

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type refusalTailReader struct{ reads int }

func (r *refusalTailReader) Read([]byte) (int, error) {
	r.reads++
	return 0, errors.New("upstream tail must not be read after refusal")
}

func TestRuntimeRefusalReturnsWithoutReadingTail(t *testing.T) {
	for _, category := range []string{"CYBER", "PRIVATE_METADATA"} {
		tail := &refusalTailReader{}
		frame := frameForTest("metadataEvent", `{"stopReason":"CONTENT_FILTERED","stopDetails":{"refusal":{"category":"`+category+`","explanation":"private explanation"}}}`)
		out, err := Decode(io.MultiReader(bytes.NewReader(frame), tail), "model", 1, nil)
		require.NoError(t, err)
		require.Equal(t, "refusal", out.StopReason)
		require.Equal(t, Usage{}, out.Usage)
		require.Zero(t, tail.reads)
	}
}

func TestRuntimeContentFilteredEmitsRefusalWithoutCachingOrBilling(t *testing.T) {
	for _, prefix := range []string{"", "text", "tool"} {
		t.Run(prefix, func(t *testing.T) {
			var frames []byte
			switch prefix {
			case "text":
				frames = frameForTest("assistantResponseEvent", `{"content":"partial"}`)
			case "tool":
				frames = frameForTest("toolUseEvent", `{"name":"read_probe","toolUseId":"call1","input":"{}","stop":true}`)
			}
			frames = append(frames, frameForTest("metadataEvent", `{"stopReason":"CONTENT_FILTERED","private":"do not log","tokenUsage":{"uncachedInputTokens":55,"outputTokens":9}}`)...)
			var emitted []string
			finalized := false
			out, err := DecodeWithUsageFinalizer(bytes.NewReader(frames), "claude-opus-5.5", 123, func(name string, data any) error {
				emitted = append(emitted, name)
				wire, _ := json.Marshal(data)
				require.NotContains(t, string(wire), "do not log")
				return nil
			}, func(*Response) { finalized = true }, nil)
			require.NoError(t, err)
			require.Equal(t, "refusal", out.StopReason)
			require.Equal(t, Usage{}, out.Usage)
			require.Equal(t, "message_start", emitted[0])
			require.Equal(t, []string{"message_delta", "message_stop"}, emitted[len(emitted)-2:])
			require.False(t, finalized)
		})
	}
}

func TestRuntimeRefusalNeverCompletesPendingTools(t *testing.T) {
	for _, input := range []string{"", "{", "{}"} {
		tool, _ := json.Marshal(map[string]any{"name": "read_probe", "toolUseId": "call1", "input": input})
		frames := frameForTest("toolUseEvent", string(tool))
		frames = append(frames, frameForTest("meteringEvent", `{"unit":"credit","usage":1}`)...)
		frames = append(frames, frameForTest("metadataEvent", `{"stopReason":"CONTENT_FILTERED"}`)...)
		tail := &refusalTailReader{}
		var events []string
		out, err := Decode(io.MultiReader(bytes.NewReader(frames), tail), "model", 22, func(name string, _ any) error { events = append(events, name); return nil })
		var filtered *ContentFilteredError
		require.ErrorAs(t, err, &filtered)
		require.True(t, filtered.IncompleteTool)
		require.Equal(t, Usage{}, out.Usage)
		require.NotContains(t, events, "content_block_stop")
		require.NotContains(t, events, "message_delta")
		require.NotContains(t, events, "message_stop")
		require.Zero(t, tail.reads)
	}
}

func TestRuntimeProviderUsageSnapshot(t *testing.T) {
	for _, tc := range []struct {
		name, metadata string
		usage          Usage
		source         string
	}{
		{"exact", `{"tokenUsage":{"uncachedInputTokens":101,"outputTokens":23,"cacheReadInputTokens":300,"cacheWriteInputTokens":40}}`, Usage{InputTokens: 101, OutputTokens: 23, CacheReadInputTokens: 300, CacheCreationInputTokens: 40, CacheSource: "upstream"}, "upstream"},
		{"zero", `{"tokenUsage":{"uncachedInputTokens":0,"outputTokens":0}}`, Usage{}, "upstream"},
		{"negative", `{"tokenUsage":{"uncachedInputTokens":-1,"outputTokens":-2,"cacheReadInputTokens":-3,"cacheWriteInputTokens":-4}}`, Usage{}, "upstream"},
		{"missing", `{"stopReason":"END_TURN"}`, Usage{InputTokens: 7, OutputTokens: 1}, "estimated"},
		{"null", `{"tokenUsage":null}`, Usage{InputTokens: 7, OutputTokens: 1}, "estimated"},
		{"empty", `{"tokenUsage":{}}`, Usage{InputTokens: 7, OutputTokens: 1}, "estimated"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			frames := frameForTest("assistantResponseEvent", `{"content":"OK"}`)
			frames = append(frames, frameForTest("metadataEvent", tc.metadata)...)
			out, err := Decode(bytes.NewReader(frames), "model", 7, nil)
			require.NoError(t, err)
			require.Equal(t, tc.usage, out.Usage)
			require.Equal(t, tc.source, out.UsageSource)
		})
	}
	frames := frameForTest("assistantResponseEvent", `{"content":"OK"}`)
	for _, metadata := range []string{`{"tokenUsage":{"uncachedInputTokens":100,"outputTokens":20,"cacheReadInputTokens":30}}`, `{"tokenUsage":{"uncachedInputTokens":4,"outputTokens":2}}`, `{"stopReason":"END_TURN"}`} {
		frames = append(frames, frameForTest("metadataEvent", metadata)...)
	}
	frames = append(frames, frameForTest("meteringEvent", `{"inputTokens":999,"outputTokens":888,"cacheReadInputTokens":777,"unit":"credit","usage":0.1}`)...)
	out, err := Decode(bytes.NewReader(frames), "model", 7, nil)
	require.NoError(t, err)
	require.Equal(t, Usage{InputTokens: 4, OutputTokens: 2}, out.Usage)
	require.Equal(t, 0.1, *out.Credits)
	for _, invalid := range []string{`{"tokenUsage":{"outputTokens":2147483648}}`, `{"tokenUsage":{"outputTokens":1.5}}`, `{"tokenUsage":{"outputTokens":"12"}}`} {
		_, err := Decode(bytes.NewReader(frameForTest("metadataEvent", invalid)), "model", 7, nil)
		require.ErrorContains(t, err, "invalid Kiro metadata")
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

func TestRuntimeNativeTruncationSurvivesLaterTools(t *testing.T) {
	for _, stop := range []string{"MAX_TOKENS", "STOP_SEQUENCE", "MODEL_CONTEXT_WINDOW_EXCEEDED"} {
		frames := frameForTest("metadataEvent", `{"stopReason":"`+stop+`"}`)
		frames = append(frames, frameForTest("toolUseEvent", `{"name":"read_probe","toolUseId":"call1","input":"{}","stop":true}`)...)
		frames = append(frames, frameForTest("metadataEvent", `{"stopReason":"END_TURN"}`)...)
		out, err := Decode(bytes.NewReader(frames), "model", 1, nil)
		require.NoError(t, err)
		require.Equal(t, strings.ToLower(stop), out.StopReason)
	}
}
