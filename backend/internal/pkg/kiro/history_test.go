package kiro

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestModelThinkingRetentionMatchesRSFamilies(t *testing.T) {
	for _, tc := range []struct {
		model string
		keep  bool
	}{
		{"claude-opus-4.6", true}, {"claude-opus-4.5", true}, {"claude-opus-4.1", false},
		{"claude-sonnet-4.6", true}, {"claude-sonnet-4.5", false}, {"claude-haiku-5.5", true},
		{"claude-3-7-sonnet", false}, {"gpt-5.6-sol", false},
	} {
		require.Equal(t, tc.keep, modelKeepsPriorThinking(tc.model), tc.model)
	}
}

func TestAssistantHistoryThinkingBoundary(t *testing.T) {
	blocks := []Block{{Type: "thinking", Thinking: "plan"}, {Type: "text", Text: "answer"}, {Type: "redacted_thinking", Data: "opaque"}}
	require.Equal(t, "<thinking>plan</thinking>\n\nanswer", assistantHistoryText(blocks, true))
	require.Equal(t, "answer", assistantHistoryText(blocks, false))
	messages := []Message{
		{Role: "user", Content: json.RawMessage(`"first"`)},
		{Role: "assistant", Content: mustJSON(blocks)},
		{Role: "user", Content: json.RawMessage(`[ {"type":"tool_result","tool_use_id":"x","content":"ok"} ]`)},
		{Role: "assistant", Content: mustJSON(blocks)},
		{Role: "user", Content: json.RawMessage(`"new instruction"`)},
	}
	require.Equal(t, 4, thinkingHistoryBoundary(messages, "claude-opus-4.1"))
	require.Equal(t, 0, thinkingHistoryBoundary(messages, "claude-opus-4.6"))
}

func TestThinkingModelAliasPreservesExplicitControls(t *testing.T) {
	raw := []byte(`{"model":"claude-opus-4-6-thinking","max_tokens":1024,"messages":[{"role":"user","content":"hi"}]}`)
	updated, err := ApplyThinkingAlias(raw, "claude-opus-4-6-thinking")
	require.NoError(t, err)
	require.Contains(t, string(updated), `"budget_tokens":20000`)
	withExplicit := []byte(`{"model":"claude-opus-4-6-thinking","thinking":{"type":"adaptive"},"max_tokens":1024,"messages":[{"role":"user","content":"hi"}]}`)
	updated, err = ApplyThinkingAlias(withExplicit, "claude-opus-4-6-thinking")
	require.NoError(t, err)
	require.JSONEq(t, string(withExplicit), string(updated))
	plain, err := ApplyThinkingAlias([]byte(`{"model":"my-thinking-model","messages":[]}`), "my-thinking-model")
	require.NoError(t, err)
	require.NotContains(t, string(plain), "budget_tokens")
}

func mustJSON(value any) json.RawMessage {
	data, _ := json.Marshal(value)
	return data
}

func TestThinkingHistoryConvertAndPrefixStayAligned(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: json.RawMessage(`"inspect test data"`)},
		{Role: "assistant", Content: json.RawMessage(`[{"type":"text","text":"A"},{"type":"text","text":"B"},{"type":"thinking","thinking":"VISIBLE_OLD","signature":"OPAQUE_SIGNATURE"},{"type":"tool_use","id":"call1","name":"Read","input":{}},{"type":"redacted_thinking","data":"OPAQUE_DATA"}]`)},
		{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"call1","content":"synthetic result"}]`)},
	}
	for _, model := range []string{"claude-opus-4.6", "claude-sonnet-4.5"} {
		for _, newTurn := range []bool{false, true} {
			msgs := append([]Message(nil), messages...)
			if newTurn {
				msgs = append(msgs, Message{Role: "assistant", Content: json.RawMessage(`"done"`)}, Message{Role: "user", Content: json.RawMessage(`"next question"`)})
			}
			raw := mustJSON(map[string]any{"max_tokens": 1024, "messages": msgs})
			body, _, err := Convert(raw, model, "session", nil)
			require.NoError(t, err)
			require.NotContains(t, string(body), "OPAQUE_")
			require.Contains(t, string(body), "AB")
			keep := !newTurn || model == "claude-opus-4.6"
			if keep {
				require.Contains(t, string(body), "VISIBLE_OLD")
			} else {
				require.NotContains(t, string(body), "VISIBLE_OLD")
			}
			plan := PlanPrefix(raw, model, true, 300)
			require.NotNil(t, plan)
			changedOpaque := []byte(strings.ReplaceAll(strings.ReplaceAll(string(raw), "OPAQUE_SIGNATURE", "different signature"), "OPAQUE_DATA", "different ciphertext"))
			require.Equal(t, plan, PlanPrefix(changedOpaque, model, true, 300), "opaque values do not enter prefix identity or token estimates")
			changedThought := []byte(strings.ReplaceAll(string(raw), "VISIBLE_OLD", strings.Repeat("changed visible thought ", 50)))
			other := PlanPrefix(changedThought, model, true, 300)
			require.NotNil(t, other)
			if keep {
				require.NotEqual(t, plan.Writes, other.Writes)
				require.Greater(t, other.TotalTokens, plan.TotalTokens)
			} else {
				require.Equal(t, plan, other)
			}
		}
	}
}

func TestThinkingAliasCanonicalModelAndPrefix(t *testing.T) {
	for _, alias := range []string{"claude-opus-4-6-thinking", "claude-opus-4.6-thinking", "kiro/claude-opus-4-6-thinking[1m]", "claude-opus-4-6[1m]-thinking", "claude-opus-4-6-20260101-thinking"} {
		require.True(t, IsThinkingAlias(alias))
		require.Equal(t, "claude-opus-4.6", ModelID(alias))
		raw := mustJSON(map[string]any{"model": alias, "max_tokens": 2048, "messages": []Message{{Role: "user", Content: json.RawMessage(`"hi"`)}}})
		require.True(t, DecodeOptionsForRequest(raw).ThinkingEnabled)
		_, eligible := CacheKey(raw, "scope")
		require.False(t, eligible, "thinking aliases cannot use full-response cache")
		explicit := []byte(`{"model":"claude-opus-4.6","max_tokens":2048,"thinking":{"type":"enabled","budget_tokens":20000},"messages":[{"role":"user","content":"hi"}]}`)
		require.Equal(t, PlanPrefix(explicit, "claude-opus-4.6", true, 300), PlanPrefix(raw, alias, true, 300))
		plain := []byte(`{"messages":[{"role":"user","content":"hi"}]}`)
		require.NotEqual(t, PlanPrefix(plain, "claude-opus-4.6", true, 300).Writes, PlanPrefix(raw, alias, true, 300).Writes)
	}
	for _, explicit := range []string{`{"type":"disabled"}`, `{"type":"enabled","budget_tokens":4000,"display":"omitted"}`, `{"type":"adaptive","display":"updates"}`} {
		raw := []byte(`{"model":"claude-opus-4-6-thinking","thinking":` + explicit + `}`)
		updated, err := ApplyThinkingAlias(raw, "claude-opus-4-6-thinking")
		require.NoError(t, err)
		require.Equal(t, raw, updated)
	}
	for _, model := range []string{"my-thinking-model", "gpt-5.6-sol-thinking", "claude-opus-4.6-thinking-extra"} {
		require.False(t, IsThinkingAlias(model))
		require.Equal(t, model, ModelID(model))
	}
}

func TestThinkingHistoryBoundaryMixedUserContent(t *testing.T) {
	for _, tc := range []struct {
		content  string
		boundary int
	}{
		{`[{"type":"tool_result","tool_use_id":"x","content":"ok"},{"type":"text","text":"  "}]`, 0},
		{`[{"type":"tool_result","tool_use_id":"x","content":"ok"},{"type":"text","text":"new instruction"}]`, 2},
		{`[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"x"}}]`, 2},
	} {
		messages := []Message{{Role: "user", Content: json.RawMessage(`"start"`)}, {Role: "assistant", Content: json.RawMessage(`"answer"`)}, {Role: "user", Content: json.RawMessage(tc.content)}}
		require.Equal(t, tc.boundary, thinkingHistoryBoundary(messages, "claude-sonnet-4.5"))
	}
}
