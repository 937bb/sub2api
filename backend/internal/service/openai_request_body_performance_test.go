package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestQuotaSuffixPreservesOriginalJSONAndBuffer(t *testing.T) {
	for _, input := range []string{
		`[]`,
		`[ \n ]`,
		`[{"type":"message","role":"user","content":[{"type":"input_image","image_url":"data:image/png;base64,AAAA"}],"metadata":{"n":9007199254740993,"f":1.000e-9}}]`,
		`[{"type":"function_call_output","call_id":"real","output":"done"},{"type":"message","role":"user","content":"continue"}]`,
	} {
		input = strings.ReplaceAll(input, `\n`, "\n")
		t.Run(input[:min(30, len(input))], func(t *testing.T) {
			body := []byte(" \n{\"model\":\"gpt-5.6-terra\",\"inpu\\u0074\":" + input + ",\"metadata\":{\"n\":9007199254740993}} \n")
			original := bytes.Clone(body)
			before := gjson.GetBytes(body, "input")
			got, changed := InjectFunctionCallOutputSuffix(body)
			require.True(t, changed)
			require.True(t, json.Valid(got))
			require.Equal(t, original, body, "injection must not mutate shared input")
			requireQuotaBypassPairs(t, got, len(before.Array()), 1)
			end := before.Index + len(before.Raw) - 1
			require.True(t, bytes.HasPrefix(got, body[:end]))
			require.True(t, bytes.HasSuffix(got, body[end:]))
			require.Equal(t, "9007199254740993", gjson.GetBytes(got, "metadata.n").Raw)
			for i, item := range before.Array() {
				require.Equal(t, item.Raw, gjson.GetBytes(got, fmt.Sprintf("input.%d", i)).Raw)
			}
		})
	}
}

func TestQuotaRequestInspectionPreservesEscapedCompactionAndToolOutputs(t *testing.T) {
	for _, body := range []string{
		`{"input":[{"type":"\u0063ompaction_trigger"}]}`,
		`{"inpu\u0074":[{"ty\u0070e":"compaction_trigger"},{"type":"message"}]}`,
	} {
		require.True(t, HasCompactionTriggerInInput([]byte(body)))
		require.Equal(t, OpenAIQuotaBypassRequestUnavailable, ClassifyOpenAIQuotaBypassRequest([]byte(body)))
		got, changed := InjectFunctionCallOutputSuffix([]byte(body))
		require.False(t, changed)
		require.Equal(t, body, string(got))
	}
	for _, body := range []string{
		`{"input":[{"type":"function_call_output","call_id":"call_real","output":"ok"}]}`,
		`{"input":[{"type":"custom_tool_call_output","call_id":"call_real","output":"ok"}]}`,
	} {
		require.Equal(t, OpenAIQuotaBypassRequestNativeToolOutput, ClassifyOpenAIQuotaBypassRequest([]byte(body)))
		got, changed := InjectFunctionCallOutputSuffix([]byte(body))
		require.False(t, changed)
		require.Equal(t, body, string(got))
	}
}

func TestLegacyIngressFastPathKeepsValidationAndEscapedKeys(t *testing.T) {
	for _, body := range []string{`{"input":`, `{"input":[]} true`, `[]`, `"hello"`, `42`} {
		_, _, err := normalizeOpenAIResponsesLegacyIngress([]byte(body))
		require.Error(t, err, body)
	}
	for _, body := range []string{
		`{"model":"gpt-5.6-terra","messag\u0065s":[{"role":"user","content":"hello"}]}`,
		`{"model":"gpt-5.6-terra","pr\u006fmpt":"hello"}`,
	} {
		got, changed, err := normalizeOpenAIResponsesLegacyIngress([]byte(body))
		require.NoError(t, err)
		require.True(t, changed)
		require.True(t, gjson.GetBytes(got, "input").Exists())
	}
}

func TestToolSchemaLookaroundPrefilterPreservesUnicodeEscapes(t *testing.T) {
	for _, pattern := range []string{`(?=x)`, `\u0028?=x)`, `(\u003f=x)`, `(\u003F=x)`, `\u0028\u003f=x)`} {
		body := []byte(`{"tools":[{"type":"function","parameters":{"type":"object","properties":{"x":{"type":"string","pattern":"` + pattern + `"}}}}]}`)
		got, changed, err := sanitizeOpenAIResponsesToolSchemasForPlatform(body, PlatformOpenAI)
		require.NoError(t, err)
		require.True(t, changed, pattern)
		require.False(t, gjson.GetBytes(got, "tools.0.parameters.properties.x.pattern").Exists())
	}
	require.False(t, openAIResponsesBodyMayContainRegexLookaround([]byte(`{"input":"func() \u4e2d\u6587"}`)))
}

func BenchmarkOpenAIRequestBodyHotPaths(b *testing.B) {
	for _, size := range []int{512 << 10, 4 << 20} {
		body := []byte(`{"model":"gpt-5.6-terra","stream":true,"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"` + strings.Repeat("x", size) + `"}]}],"tools":[{"type":"function","name":"lookup","parameters":{"type":"object","properties":{"q":{"type":"string","pattern":"^(?=x).*"}}}}]}`)
		for _, name := range []string{"legacy", "compaction", "quota_classification", "quota_injection", "tool_schemas"} {
			b.Run(fmt.Sprintf("%s/%dKiB", name, size>>10), func(b *testing.B) {
				b.SetBytes(int64(len(body)))
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					switch name {
					case "legacy":
						if _, changed, err := normalizeOpenAIResponsesLegacyIngress(body); err != nil || changed {
							b.Fatalf("native request changed: %v", err)
						}
					case "compaction":
						if HasCompactionTriggerInInput(body) {
							b.Fatal("unexpected compaction")
						}
					case "quota_classification":
						if ClassifyOpenAIQuotaBypassRequest(body) != OpenAIQuotaBypassRequestInjectable {
							b.Fatal("request is not injectable")
						}
					case "quota_injection":
						if _, changed := InjectFunctionCallOutputSuffix(body); !changed {
							b.Fatal("missing quota suffix")
						}
					case "tool_schemas":
						if _, changed, err := sanitizeOpenAIResponsesToolSchemasForPlatform(body, PlatformOpenAI); err != nil || !changed {
							b.Fatalf("missing schema repair: %v", err)
						}
					}
				}
			})
		}
	}
}
