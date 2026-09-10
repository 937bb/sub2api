package handler

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCodexBootstrapFastPathPreservesEscapedEnvelopes(t *testing.T) {
	delegation := []byte(`{"input":[{"type":"function_call_output","namespace":"codex_app","name":"create_thread","output":"` + delegationEnvelope + `"}]}`)
	delegation = bytes.ReplaceAll(delegation, []byte("codex_delegation"), []byte(`\u0063odex_delegation`))
	_, changed := normalizeCodexDelegationBootstrap(delegation)
	require.True(t, changed)
	escapedKeys := bytes.ReplaceAll(delegation, []byte(`"input"`), []byte(`"inpu\u0074"`))
	escapedKeys = bytes.ReplaceAll(escapedKeys, []byte(`"type"`), []byte(`"ty\u0070e"`))
	escapedKeys = bytes.ReplaceAll(escapedKeys, []byte(`"name"`), []byte(`"na\u006de"`))
	_, changed = normalizeCodexDelegationBootstrap(escapedKeys)
	require.True(t, changed, "escaped JSON keys must not hide a bootstrap candidate")

	automation := codexAutomationBootstrapBody(t, codexAutomationBootstrap("wiki", "never", "review"), "")
	automation = bytes.ReplaceAll(automation, []byte("Automation ID:"), []byte(`\u0041utomation ID:`))
	_, changed = normalizeCodexAutomationBootstrap(automation)
	require.True(t, changed)

	heartbeat := codexAutomationBootstrapBody(t, `<heartbeat><automation_id>wiki</automation_id></heartbeat>`, "")
	heartbeat = bytes.ReplaceAll(heartbeat, []byte("automation_id"), []byte(`\u0061utomation_id`))
	_, changed = normalizeCodexAutomationBootstrap(heartbeat)
	require.True(t, changed)
}

func TestCodexBootstrapOrdinaryImagePayloadHasBoundedAllocations(t *testing.T) {
	body := []byte(`{"input":[{"type":"message","role":"user","content":[{"type":"input_image","image_url":"data:image/png;base64,` + strings.Repeat("A", 1<<20) + `"}]}],"metadata":{"integer":9007199254740993}}`)
	allocations := testing.AllocsPerRun(5, func() {
		first, changed := normalizeCodexAutomationBootstrap(body)
		require.False(t, changed)
		second, changed := normalizeCodexDelegationBootstrap(first)
		require.False(t, changed)
		require.True(t, &second[0] == &body[0], "ordinary input must remain untouched")
	})
	require.LessOrEqual(t, allocations, float64(2), "ordinary payloads must not allocate full JSON trees")
}

func TestCodexBootstrapUnicodeOrdinaryOutputHasBoundedAllocations(t *testing.T) {
	body := []byte(`{"input":[{"type":"function_call_output","name":"exec","call_id":"real","output":"\u4e2d\u6587 ` + strings.Repeat("x", 1<<20) + `"}]}`)
	allocations := testing.AllocsPerRun(5, func() {
		_, changed := normalizeCodexAutomationBootstrap(body)
		require.False(t, changed)
		_, changed = normalizeCodexDelegationBootstrap(body)
		require.False(t, changed)
	})
	require.Less(t, allocations, float64(30), "Unicode must not force full JSON tree decoding")
}

func BenchmarkCodexBootstrapUnicode(b *testing.B) {
	body := []byte(`{"input":[{"type":"function_call_output","name":"exec","call_id":"real","output":"\u4e2d\u6587 ` + strings.Repeat("x", 4<<20) + `"}]}`)
	for _, reference := range []bool{true, false} {
		b.Run(fmt.Sprintf("reference_%t", reference), func(b *testing.B) {
			b.SetBytes(int64(len(body)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if reference {
					normalizeCodexCallOutputBootstrap(body, isCodexAutomationCandidate, false)
					normalizeCodexCallOutputBootstrap(body, isCodexDelegationCandidate, true)
				} else {
					normalizeCodexAutomationBootstrap(body)
					normalizeCodexDelegationBootstrap(body)
				}
			}
		})
	}
}

func BenchmarkCodexBootstrapHotPath(b *testing.B) {
	for _, size := range []int{512 << 10, 4 << 20} {
		b.Run(fmt.Sprintf("%dKiB", size>>10), func(b *testing.B) {
			body := []byte(`{"model":"gpt-5.6-terra","stream":true,"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"` + strings.Repeat("x", size) + `"}]}]}`)
			b.SetBytes(int64(len(body)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				first, changed := normalizeCodexAutomationBootstrap(body)
				if changed {
					b.Fatal("ordinary request changed")
				}
				if _, changed := normalizeCodexDelegationBootstrap(first); changed {
					b.Fatal("ordinary request changed")
				}
			}
		})
	}
}
