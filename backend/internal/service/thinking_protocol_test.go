package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestResolveThinkingProtocol(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  ThinkingProtocol
	}{
		{name: "claude strict", model: "claude-3-5-sonnet-20241022", want: ThinkingProtocolAnthropicStrict},
		{name: "sonnet alias strict", model: "sonnet-4.5", want: ThinkingProtocolAnthropicStrict},
		{name: "deepseek passback", model: "deepseek-reasoner", want: ThinkingProtocolPassbackRequired},
		{name: "kimi passback", model: "kimi-k2", want: ThinkingProtocolPassbackRequired},
		{name: "moonshot passback", model: "moonshot-v1-128k", want: ThinkingProtocolPassbackRequired},
		{name: "glm passback", model: "glm-4.6", want: ThinkingProtocolPassbackRequired},
		{name: "minimax m passback", model: "minimax-m1", want: ThinkingProtocolPassbackRequired},
		{name: "qwen thinking passback", model: "qwen3-next-thinking", want: ThinkingProtocolPassbackRequired},
		{name: "qwen non thinking unknown", model: "qwen3-next", want: ThinkingProtocolUnknown},
		{name: "unknown", model: "gpt-5.5", want: ThinkingProtocolUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, ResolveThinkingProtocol(tt.model))
		})
	}
}

func TestThinkingProtocolFiltersAreModelAware(t *testing.T) {
	body := []byte(`{"thinking":{"type":"enabled"},"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"keep me","signature":""},{"type":"text","text":"answer"}]}]}`)

	strict := FilterThinkingBlocks(body, "claude-3-5-sonnet-20241022")
	require.NotEqual(t, string(body), string(strict))
	require.NotContains(t, string(strict), `"type":"thinking"`)

	require.Equal(t, body, FilterThinkingBlocks(body, "kimi-k2"), "passback-required providers need historical thinking blocks unchanged")
	require.Equal(t, body, FilterThinkingBlocks(body, "gpt-5.5"), "unknown providers should not be destructively filtered")
	require.Equal(t, body, FilterThinkingBlocksForRetry(body, "kimi-k2"), "retry filters are Anthropic-strict only")

	toolBody := []byte(`{"thinking":{"type":"enabled"},"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Search","input":{"q":"x"}},{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}]}`)
	require.Equal(t, toolBody, FilterSignatureSensitiveBlocksForRetry(toolBody, "kimi-k2"), "passback-required providers should keep tool/thinking protocol blocks unchanged")
	require.NotEqual(t, string(toolBody), string(FilterSignatureSensitiveBlocksForRetry(toolBody, "claude-3-5-sonnet-20241022")))
}

func TestNormalizeChineseLLMThinking(t *testing.T) {
	body := []byte(`{"thinking":{"type":"enabled"},"messages":[]}`)

	out, changed := NormalizeChineseLLMThinking(body, "minimax-m1")
	require.True(t, changed)
	require.Equal(t, "adaptive", gjson.GetBytes(out, "thinking.type").String())

	out, changed = NormalizeChineseLLMThinking(body, "kimi-k2")
	require.False(t, changed)
	require.Equal(t, body, out)

	adaptiveBody := []byte(`{"thinking":{"type":"adaptive"},"messages":[]}`)
	out, changed = NormalizeChineseLLMThinking(adaptiveBody, "minimax-m1")
	require.False(t, changed)
	require.Equal(t, adaptiveBody, out)
}

func TestNormalizeGLMOpenAIReasoningEffort(t *testing.T) {
	tests := []struct {
		name        string
		body        []byte
		model       string
		path        string
		want        string
		wantChanged bool
	}{
		{
			name:        "flat xhigh to max",
			body:        []byte(`{"reasoning_effort":"xhigh","messages":[]}`),
			model:       "glm-5.2",
			path:        "reasoning_effort",
			want:        "max",
			wantChanged: true,
		},
		{
			name:        "flat x-high to max",
			body:        []byte(`{"reasoning_effort":"x-high","messages":[]}`),
			model:       "glm-5.2",
			path:        "reasoning_effort",
			want:        "max",
			wantChanged: true,
		},
		{
			name:        "flat medium to high",
			body:        []byte(`{"reasoning_effort":"medium","messages":[]}`),
			model:       "glm-5.2",
			path:        "reasoning_effort",
			want:        "high",
			wantChanged: true,
		},
		{
			name:        "nested uppercase high to high",
			body:        []byte(`{"reasoning":{"effort":"HIGH"},"messages":[]}`),
			model:       "glm-5.2",
			path:        "reasoning.effort",
			want:        "high",
			wantChanged: true,
		},
		{
			name:        "non GLM unchanged",
			body:        []byte(`{"reasoning_effort":"xhigh","messages":[]}`),
			model:       "deepseek-reasoner",
			wantChanged: false,
		},
		{
			name:        "missing effort unchanged",
			body:        []byte(`{"messages":[]}`),
			model:       "glm-5.2",
			wantChanged: false,
		},
		{
			name:        "unknown effort unchanged",
			body:        []byte(`{"reasoning_effort":"extreme","messages":[]}`),
			model:       "glm-5.2",
			wantChanged: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, changed := NormalizeGLMOpenAIReasoningEffort(tt.body, tt.model)
			require.Equal(t, tt.wantChanged, changed)
			if !tt.wantChanged {
				require.Equal(t, tt.body, out)
				return
			}
			require.Equal(t, tt.want, gjson.GetBytes(out, tt.path).String())
		})
	}
}

func TestDefaultEffortForThinkingEnabled(t *testing.T) {
	tests := []struct {
		name    string
		model   string
		wantNil bool
	}{
		{name: "kimi", model: "kimi-k2"},
		{name: "moonshot", model: "moonshot-v1-128k"},
		{name: "glm", model: "glm-4.6"},
		{name: "minimax m", model: "minimax-m1"},
		{name: "qwen thinking", model: "qwen3-next-thinking"},
		{name: "deepseek excluded", model: "deepseek-reasoner", wantNil: true},
		{name: "claude strict", model: "claude-3-5-sonnet-20241022", wantNil: true},
		{name: "unknown", model: "gpt-5.5", wantNil: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DefaultEffortForThinkingEnabled(tt.model)
			if tt.wantNil {
				require.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			require.Equal(t, "high", *got)
		})
	}
}

func TestApplyThinkingEnabledFallback(t *testing.T) {
	explicit := "low"
	require.Same(t, &explicit, ApplyThinkingEnabledFallback(&explicit, []byte(`{"thinking":{"type":"enabled"}}`), "kimi-k2"))

	got := ApplyThinkingEnabledFallback(nil, []byte(`{"thinking":{"type":"enabled"}}`), "kimi-k2")
	require.NotNil(t, got)
	require.Equal(t, "high", *got)

	got = ApplyThinkingEnabledFallback(nil, []byte(`{"thinking":{"type":"adaptive"}}`), "minimax-m1")
	require.NotNil(t, got)
	require.Equal(t, "high", *got)

	require.Nil(t, ApplyThinkingEnabledFallback(nil, []byte(`{"thinking":{"type":"disabled"}}`), "kimi-k2"))
	require.Nil(t, ApplyThinkingEnabledFallback(nil, []byte(`{"thinking":{"type":"enabled"}}`), "deepseek-reasoner"))
	require.Nil(t, ApplyThinkingEnabledFallback(nil, []byte(`{"thinking":{"type":"enabled"}}`), "gpt-5.5"))
}
