package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/stretchr/testify/require"
)

func TestDecideResponsesProbeSupport(t *testing.T) {
	fnCall := []byte(`{"output":[{"type":"reasoning"},{"type":"function_call","name":"probe_ping"}]}`)
	reasoningOnly := []byte(`{"output":[{"type":"reasoning"}]}`)

	cases := []struct {
		name   string
		status int
		body   []byte
		want   bool
	}{
		// Endpoint clearly absent on third-party OpenAI-compatible upstreams.
		{"404 endpoint absent", 404, fnCall, false},
		{"405 method not allowed", 405, fnCall, false},
		// 2xx: tool capability is judged by presence of a function_call output item.
		{"200 with function_call", 200, fnCall, true},
		// Volcengine Ark coding/v3 × kimi-k2.6: reasoning only, no function_call.
		{"200 reasoning only", 200, reasoningOnly, false},
		{"200 invalid json", 200, []byte("not-json"), false},
		{"200 no output field", 200, []byte(`{"status":"completed"}`), false},
		// Non-2xx (other than 404/405): endpoint exists, capability undecidable -> conservative true.
		{"400 conservative true", 400, reasoningOnly, true},
		{"401 conservative true", 401, nil, true},
		{"500 conservative true", 500, nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, decideResponsesProbeSupport(tc.status, tc.body))
		})
	}
}

func TestResponsesProbeBodyHasFunctionCall(t *testing.T) {
	require.True(t, responsesProbeBodyHasFunctionCall([]byte(`{"output":[{"type":"function_call"}]}`)))
	require.True(t, responsesProbeBodyHasFunctionCall([]byte(`{"output":[{"type":"reasoning"},{"type":"function_call"}]}`)))
	require.False(t, responsesProbeBodyHasFunctionCall([]byte(`{"output":[{"type":"reasoning"}]}`)))
	require.False(t, responsesProbeBodyHasFunctionCall([]byte(`{"output":[]}`)))
	require.False(t, responsesProbeBodyHasFunctionCall([]byte(`{}`)))
	require.False(t, responsesProbeBodyHasFunctionCall([]byte(`garbage`)))
}

func TestSelectResponsesProbeModel(t *testing.T) {
	// No model_mapping -> fall back to DefaultTestModel (OpenAI official APIKey).
	model, scoped := selectResponsesProbeModelWithScope(&Account{})
	require.Equal(t, openai.DefaultTestModel, model)
	require.False(t, scoped)
	require.Equal(t, openai.DefaultTestModel, selectResponsesProbeModel(&Account{}))

	// model_mapping values are upstream models; pick first by sort for reproducibility.
	acct := &Account{Credentials: map[string]any{
		"model_mapping": map[string]any{
			"client-b": "zeta-model",
			"client-a": "alpha-model",
		},
	}}
	model, scoped = selectResponsesProbeModelWithScope(acct)
	require.Equal(t, "alpha-model", model)
	require.True(t, scoped)
	require.Equal(t, "alpha-model", selectResponsesProbeModel(acct))

	// Wildcard / blank upstream values are skipped.
	acctWild := &Account{Credentials: map[string]any{
		"model_mapping": map[string]any{
			"a": "*",
			"b": "  ",
			"c": "real-model",
		},
	}}
	model, scoped = selectResponsesProbeModelWithScope(acctWild)
	require.Equal(t, "real-model", model)
	require.True(t, scoped)

	// Only wildcard mappings -> DefaultTestModel.
	acctAllWild := &Account{Credentials: map[string]any{
		"model_mapping": map[string]any{"a": "gpt-*"},
	}}
	model, scoped = selectResponsesProbeModelWithScope(acctAllWild)
	require.Equal(t, openai.DefaultTestModel, model)
	require.False(t, scoped)
}

func TestBuildResponsesProbeExtraUpdatesDefaultProbeClearsStaleModelMap(t *testing.T) {
	extra := map[string]any{
		openai_compat.ExtraKeyResponsesSupportedByModel: map[string]any{"old-probe-model": false},
	}

	updates := buildResponsesProbeExtraUpdates(extra, openai.DefaultTestModel, false, false)

	require.Equal(t, false, updates[openai_compat.ExtraKeyResponsesSupported])
	require.Contains(t, updates, openai_compat.ExtraKeyResponsesSupportedByModel)
	require.Nil(t, updates[openai_compat.ExtraKeyResponsesSupportedByModel])
}

func TestBuildResponsesProbeExtraUpdatesModelScopedIncludesMergedMap(t *testing.T) {
	extra := map[string]any{
		openai_compat.ExtraKeyResponsesSupportedByModel: map[string]any{"model-a": true},
	}

	updates := buildResponsesProbeExtraUpdatesWithNestedMap(extra, "model-b", true, false, false)
	require.Equal(t, map[string]any{openai_compat.ExtraKeyResponsesSupported: false}, updates)

	updates = buildResponsesProbeExtraUpdates(extra, "model-b", true, false)
	require.Equal(t, false, updates[openai_compat.ExtraKeyResponsesSupported])
	require.Equal(t, map[string]any{"model-a": true, "model-b": false}, updates[openai_compat.ExtraKeyResponsesSupportedByModel])
}

func TestMergeResponsesSupportByModelPreservesSiblings(t *testing.T) {
	extra := map[string]any{
		openai_compat.ExtraKeyResponsesSupportedByModel: map[string]any{
			"model-a": true,
			"model-b": false,
		},
	}

	merged := mergeResponsesSupportByModel(extra, "model-c", true)

	require.Equal(t, map[string]any{
		"model-a": true,
		"model-b": false,
		"model-c": true,
	}, merged)
}

func TestMergeResponsesSupportByModelOverwritesProbeModel(t *testing.T) {
	extra := map[string]any{
		openai_compat.ExtraKeyResponsesSupportedByModel: map[string]bool{
			"model-a": true,
		},
	}

	merged := mergeResponsesSupportByModel(extra, "model-a", false)

	require.Equal(t, map[string]any{"model-a": false}, merged)
}
