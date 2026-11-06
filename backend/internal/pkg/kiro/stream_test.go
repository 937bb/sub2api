package kiro

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNoContentDiagnosticsNeverCompleteOrBill(t *testing.T) {
	for _, tc := range []struct {
		name   string
		frames []byte
		want   StreamSummary
	}{
		{name: "empty"},
		{name: "context only", frames: frameForTest("contextUsageEvent", `{"contextUsagePercentage":0.68}`), want: StreamSummary{Frames: 1, Context: 1}},
		{name: "reasoning only", frames: frameForTest("reasoningContentEvent", `{"signature":"private-signature","text":"private-reasoning"}`), want: StreamSummary{Frames: 1, Reasoning: 1}},
		{name: "empty assistant", frames: frameForTest("assistantResponseEvent", `{"content":""}`), want: StreamSummary{Frames: 1, Assistant: 1}},
		{name: "unrecognized", frames: frameForTest("private-event-name", `{"content":"private-content"}`), want: StreamSummary{Frames: 1, Other: 1}},
		{name: "metering only", frames: frameForTest("meteringEvent", `{"unit":"credit","usage":0.1}`), want: StreamSummary{Frames: 1, Metering: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var emitted []string
			finalized := false
			out, err := DecodeWithUsageFinalizer(bytes.NewReader(tc.frames), "model", 12, func(name string, _ any) error {
				emitted = append(emitted, name)
				return nil
			}, func(*Response) { finalized = true }, nil)
			var empty *NoContentError
			require.ErrorAs(t, err, &empty)
			require.Equal(t, tc.want, empty.Summary)
			require.NotContains(t, err.Error(), "private")
			require.Empty(t, out.Content)
			require.False(t, finalized)
			require.NotContains(t, emitted, "message_start")
			require.NotContains(t, emitted, "message_stop")
			require.NotContains(t, emitted, "message_delta")
		})
	}
}

func frameForTest(kind, payload string) []byte {
	var h bytes.Buffer
	for _, pair := range [][2]string{{":message-type", "event"}, {":event-type", kind}} {
		h.WriteByte(byte(len(pair[0])))
		h.WriteString(pair[0])
		h.WriteByte(7)
		_ = binary.Write(&h, binary.BigEndian, uint16(len(pair[1])))
		h.WriteString(pair[1])
	}
	frame := make([]byte, 12)
	binary.BigEndian.PutUint32(frame, uint32(16+h.Len()+len(payload)))
	binary.BigEndian.PutUint32(frame[4:], uint32(h.Len()))
	binary.BigEndian.PutUint32(frame[8:], crc32.ChecksumIEEE(frame[:8]))
	frame = append(frame, h.Bytes()...)
	frame = append(frame, []byte(payload)...)
	crc := crc32.ChecksumIEEE(frame)
	frame = binary.BigEndian.AppendUint32(frame, crc)
	return frame
}
func TestToolEndRequiresTerminalEvidence(t *testing.T) {
	tool := frameForTest("toolUseEvent", `{"name":"lookup","toolUseId":"test-id","input":"{}"}`)
	_, err := Decode(bytes.NewReader(tool), "model", 10, nil)
	require.Error(t, err)
	metered := append(append([]byte{}, tool...), frameForTest("meteringEvent", `{"unit":"credit","usage":0.001}`)...)
	result, err := Decode(bytes.NewReader(metered), "model", 10, nil)
	require.NoError(t, err)
	require.Equal(t, "tool_use", result.StopReason)
	require.JSONEq(t, `{}`, string(result.Content[0].Input))
}
func TestTruncatedArgumentsNeverBecomeCompletedTool(t *testing.T) {
	tool := frameForTest("toolUseEvent", `{"name":"lookup","toolUseId":"test-id","input":"{\"x\":"}`)
	tool = append(tool, frameForTest("meteringEvent", `{"unit":"credit","usage":0.001}`)...)
	_, err := Decode(bytes.NewReader(tool), "model", 10, nil)
	require.Error(t, err)
}
func TestCorruptedOrTruncatedFramesFail(t *testing.T) {
	frame := frameForTest("assistantResponseEvent", `{"content":"test"}`)
	_, err := ReadFrame(bytes.NewReader(frame[:len(frame)-1]))
	require.Error(t, err)
	frame[len(frame)-1] ^= 1
	_, err = ReadFrame(bytes.NewReader(frame))
	require.ErrorContains(t, err, "CRC")
}
func TestNoSyntheticCacheAccounting(t *testing.T) {
	result, err := Decode(bytes.NewReader(frameForTest("assistantResponseEvent", `{"content":"test"}`)), "model", 42, nil)
	require.NoError(t, err)
	require.Equal(t, "estimated", result.UsageSource)
	require.Zero(t, result.Usage.CacheReadInputTokens)
	require.Zero(t, result.Usage.CacheCreationInputTokens)
}
