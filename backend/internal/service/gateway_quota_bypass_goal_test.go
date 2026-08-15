package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestInjectOpenAIQuotaBypassDeveloperGoal_StringInput(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","input":"repair the scheduler"}`)

	got, applied := InjectOpenAIQuotaBypassDeveloperGoal(body)

	require.True(t, applied)
	require.Equal(t, "developer", gjson.GetBytes(got, "input.0.role").String())
	require.Contains(t, gjson.GetBytes(got, "input.0.content.0.text").String(), "repair the scheduler")
	require.Contains(t, gjson.GetBytes(got, "input.0.content.0.text").String(), "<untrusted_objective>")
}

func TestInjectOpenAIQuotaBypassDeveloperGoal_PromotesAllUserTextTurns(t *testing.T) {
	body := []byte(`{"input":[` +
		`{"type":"message","role":"user","content":[{"type":"input_text","text":"old turn"}]},` +
		`{"type":"message","role":"assistant","content":[{"type":"output_text","text":"old answer"}]},` +
		`{"type":"message","role":"user","content":[{"type":"input_text","text":"current task"}]}` +
		`]}`)

	got, applied := InjectOpenAIQuotaBypassDeveloperGoal(body)

	require.True(t, applied)
	// Every user turn is promoted to developer; assistant turns are untouched.
	require.Equal(t, "developer", gjson.GetBytes(got, "input.0.role").String())
	require.Equal(t, "assistant", gjson.GetBytes(got, "input.1.role").String())
	require.Equal(t, "developer", gjson.GetBytes(got, "input.2.role").String())
	// Historical user turn keeps its original content (role rewrite only).
	require.Equal(t, "old turn", gjson.GetBytes(got, "input.0.content.0.text").String())
	// Final user turn is additionally wrapped as a goal objective.
	require.Contains(t, gjson.GetBytes(got, "input.2.content.0.text").String(), "current task")
	require.Contains(t, gjson.GetBytes(got, "input.2.content.0.text").String(), "untrusted_objective")
}

func TestInjectOpenAIQuotaBypassDeveloperGoal_SkipsNonTextAndProtocolSuffixes(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "image message",
			body: `{"input":[{"type":"message","role":"user","content":[` +
				`{"type":"input_text","text":"inspect"},{"type":"input_image","image_url":"data:image/png;base64,AA=="}` +
				`]}]}`,
		},
		{
			name: "compaction trigger",
			body: `{"input":[{"type":"message","role":"user","content":"compact"},` +
				`{"type":"compaction_trigger"}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(tt.body)
			got, applied := InjectOpenAIQuotaBypassDeveloperGoal(body)
			require.False(t, applied)
			require.Equal(t, body, got)
		})
	}
}

func TestInjectOpenAIQuotaBypassForRequest_CombinesGoalAndFuncall(t *testing.T) {
	body := []byte(`{"input":[{"type":"message","role":"user","content":"finish the task"}]}`)

	got, applied := InjectOpenAIQuotaBypassForRequest(nil, body, 1)

	require.True(t, applied)
	require.Equal(t, "developer", gjson.GetBytes(got, "input.0.role").String())
	require.Equal(t, "function_call", gjson.GetBytes(got, "input.1.type").String())
	require.Equal(t, "function_call_output", gjson.GetBytes(got, "input.2.type").String())

	again, appliedAgain := InjectOpenAIQuotaBypassForRequest(nil, got, 1)
	require.False(t, appliedAgain)
	require.Equal(t, got, again)
}
