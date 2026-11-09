package kiro

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func prefixFixture(t *testing.T, body map[string]any, auto bool) *PrefixPlan {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	return PlanPrefix(raw, "claude-opus-4-6", auto, 300)
}

func TestPrefixGrowingToolConversation(t *testing.T) {
	body := map[string]any{"system": strings.Repeat("Stable instructions. ", 200), "thinking": map[string]any{"type": "adaptive"}, "tools": []any{map[string]any{"name": "Read", "input_schema": map[string]any{"type": "object"}}}}
	messages := []any{}
	stored := map[string]int{}
	for round := 0; round < 12; round++ {
		messages = append(messages, map[string]any{"role": "user", "content": fmt.Sprintf("Read item %d", round)})
		body["messages"] = messages
		plan := prefixFixture(t, body, true)
		require.NotNil(t, plan)
		matched := 0
		for _, p := range plan.Lookups {
			matched = max(matched, stored[p.Digest])
		}
		if round == 0 {
			require.Zero(t, matched)
		} else {
			require.Positive(t, matched, "growing tool history should reuse a prior checkpoint")
		}
		for _, percent := range []int{0, 30, 90, 100} {
			input, w5, w1, read := plan.Split(10000, matched, percent)
			require.Equal(t, 10000, input+w5+w1+read)
			require.Positive(t, input)
			require.LessOrEqual(t, read, 100*percent)
		}
		for _, p := range plan.Writes {
			stored[p.Digest] = p.Tokens
		}
		messages = append(messages, map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "tool_use", "id": fmt.Sprint(round), "name": "Read", "input": map[string]any{"path": fmt.Sprintf("%d.txt", round)}}}}, map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": fmt.Sprint(round), "content": "Result"}}}, map[string]any{"role": "assistant", "content": "Done"})
	}
}

func TestPrefixCanonicalBlocksAndIsolation(t *testing.T) {
	one := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"hello","cache_control":{"type":"ephemeral"}}]}]}`)
	two := []byte(`{"messages":[{"role":"user","content":"hello"}]}`)
	a := PlanPrefix(one, "claude-opus-4-6", false, 300)
	b := PlanPrefix(two, "claude-opus-4.6", true, 300)
	require.NotNil(t, a)
	require.Equal(t, a.Writes[0].Digest, b.Writes[0].Digest)
	for _, raw := range []string{
		`{"messages":[{"role":"assistant","content":"hello"}]}`,
		`{"messages":[{"role":"user","content":"changed"}]}`,
		`{"thinking":{"type":"adaptive"},"messages":[{"role":"user","content":"hello"}]}`,
		`{"metadata":{"user_id":"other-session"},"messages":[{"role":"user","content":"hello"}]}`,
		`{"tools":[{"name":"Read","input_schema":{"type":"object"}}],"messages":[{"role":"user","content":"hello"}]}`,
	} {
		other := PlanPrefix([]byte(raw), "claude-opus-4.6", true, 300)
		require.NotNil(t, other)
		require.NotEqual(t, a.Writes[0].Digest, other.Writes[len(other.Writes)-1].Digest)
	}
	require.NotEqual(t, a.Writes[0].Digest, PlanPrefix(two, "claude-sonnet-4.6", true, 300).Writes[0].Digest)
}

func TestPrefixInvalidControlsAndLookback(t *testing.T) {
	for _, raw := range []string{
		`{"cache_control":null,"messages":[{"role":"user","content":"hi"}]}`,
		`{"cache_control":{"type":"persistent"},"messages":[{"role":"user","content":"hi"}]}`,
		`{"messages":[{"role":"user","content":[{"type":"text","text":"hi","cache_control":{"type":"ephemeral","ttl":"2h"}}]}]}`,
		`{"messages":[{"role":"user","content":[{"type":"thinking","thinking":"hi","cache_control":{"type":"ephemeral"}}]}]}`,
		`{"messages":[{"role":"user","content":[{"type":"text","text":"hi","cache_control":{"type":"ephemeral","ttl":"5m"}},{"type":"text","text":"there","cache_control":{"type":"ephemeral","ttl":"1h"}}]}]}`,
	} {
		require.Nil(t, PlanPrefix([]byte(raw), "claude-opus-4.6", true, 300))
	}
	body := map[string]any{"messages": []any{map[string]any{"role": "user", "content": "hi"}}}
	require.Nil(t, prefixFixture(t, body, false), "no declared cache control means no metering when auto is disabled")
	blocks := []any{}
	for i := 0; i < 25; i++ {
		blocks = append(blocks, map[string]any{"type": "text", "text": fmt.Sprint(i)})
	}
	body["cache_control"] = map[string]any{"type": "ephemeral"}
	body["messages"] = []any{map[string]any{"role": "user", "content": blocks}}
	plan := prefixFixture(t, body, false)
	require.Len(t, plan.Lookups, 20)
	for i := range blocks {
		blocks[i] = map[string]any{"type": "tool_result", "tool_use_id": fmt.Sprint(i), "content": "OK"}
	}
	plan = prefixFixture(t, body, false)
	require.Len(t, plan.Lookups, 25, "parallel tool results form one lookback position")
	for i := 0; i < 5; i++ {
		blocks[i].(map[string]any)["cache_control"] = map[string]any{"type": "ephemeral"}
	}
	require.Nil(t, prefixFixture(t, body, false), "more than four checkpoints disables metering")
}

func TestPrefixTTLAndReadCapConservation(t *testing.T) {
	p := &PrefixPlan{TotalTokens: 1000, CoveredTokens: 900, Writes: []PrefixPoint{{Tokens: 500, TTLSeconds: 3600}, {Tokens: 900, TTLSeconds: 300}}}
	input, w5, w1, read := p.Split(1000, 0, 90)
	require.Equal(t, []int{100, 400, 500, 0}, []int{input, w5, w1, read})
	input, w5, w1, read = p.Split(1000, 600, 30)
	require.Equal(t, []int{400, 300, 0, 300}, []int{input, w5, w1, read})
	for total := 1; total < 100; total++ {
		for matched := 0; matched <= 900; matched += 100 {
			for percent := 0; percent <= 100; percent += 10 {
				a, b, c, d := p.Split(total, matched, percent)
				require.Equal(t, total, a+b+c+d)
				require.GreaterOrEqual(t, a, 1)
				require.GreaterOrEqual(t, min(b, c, d), 0)
			}
		}
	}
}
