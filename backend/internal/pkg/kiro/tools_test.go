package kiro

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func convertToolRequest(t *testing.T, request map[string]any) (map[string]any, *Request) {
	t.Helper()
	request["max_tokens"] = 1024
	raw, err := json.Marshal(request)
	require.NoError(t, err)
	body, req, err := Convert(raw, "claude-sonnet-4.6", "session", nil)
	require.NoError(t, err)
	var out map[string]any
	require.NoError(t, json.Unmarshal(body, &out))
	return out["conversationState"].(map[string]any), req
}

func TestToolsParallelResultsImagesAndRetryHistory(t *testing.T) {
	long := "mcp__" + strings.Repeat("server_", 12) + "read_image"
	image := map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": "aGVsbG8="}}
	state, req := convertToolRequest(t, map[string]any{
		"tools": []any{map[string]any{"name": long, "input_schema": map[string]any{"type": "object", "properties": nil, "required": nil}}},
		"messages": []any{
			map[string]any{"role": "user", "content": "Read both files."},
			map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "tool_use", "id": "one", "name": long, "input": map[string]any{"path": "a.png"}}, map[string]any{"type": "tool_use", "id": "two", "name": long, "input": map[string]any{"path": "b.png"}}}},
			map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": "two", "is_error": true, "content": "permission denied"}}},
			map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": "one", "content": []any{map[string]any{"type": "text", "text": "image A"}, image}}}},
			map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "tool_use", "id": "retry", "name": long, "input": map[string]any{"path": "b.png"}}}},
			map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": "retry", "content": "retried successfully"}}},
		},
	})
	alias := upstreamToolName(long)
	require.Equal(t, long, req.ToolNames[alias])
	require.LessOrEqual(t, len(alias), 63)
	history := state["history"].([]any)
	require.Len(t, history, 4)
	assistant := history[1].(map[string]any)["assistantResponseMessage"].(map[string]any)
	uses := assistant["toolUses"].([]any)
	require.Len(t, uses, 2)
	require.Equal(t, alias, uses[0].(map[string]any)["name"])
	user := history[2].(map[string]any)["userInputMessage"].(map[string]any)
	require.Len(t, user["images"].([]any), 1)
	results := user["userInputMessageContext"].(map[string]any)["toolResults"].([]any)
	require.Equal(t, "two", results[0].(map[string]any)["toolUseId"])
	require.Equal(t, "error", results[0].(map[string]any)["status"])
	require.Equal(t, "one", results[1].(map[string]any)["toolUseId"])
	current := state["currentMessage"].(map[string]any)["userInputMessage"].(map[string]any)
	require.Len(t, current["userInputMessageContext"].(map[string]any)["tools"].([]any), 1)
}

func TestToolNameAliasesAndCollisions(t *testing.T) {
	a := strings.Repeat("x", 80) + "a"
	b := strings.Repeat("x", 80) + "b"
	require.NotEqual(t, upstreamToolName(a), upstreamToolName(b))
	name := strings.Repeat("测", 50)
	require.True(t, utf8.ValidString(upstreamToolName(name)))
	require.LessOrEqual(t, len(upstreamToolName(name)), 63)
	req := Request{Tools: []Tool{{Name: a}, {Name: upstreamToolName(a)}}}
	require.ErrorContains(t, prepareTools(&req), "collision")
	req = Request{Tools: []Tool{{Name: "read"}, {Name: "read"}}}
	require.ErrorContains(t, prepareTools(&req), "unique")
}

func TestHistoricalToolsAndNoneDoNotEnableNewCalls(t *testing.T) {
	for _, choice := range []string{"auto", "none"} {
		t.Run(choice, func(t *testing.T) {
			_, req := convertToolRequest(t, map[string]any{"tool_choice": map[string]any{"type": choice}, "tools": []any{map[string]any{"name": "current"}}, "messages": []any{
				map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "tool_use", "id": "id", "name": "old", "input": map[string]any{}}}},
				map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": "id", "content": "done"}}},
			}})
			require.NotContains(t, req.ToolNames, "old")
			require.Contains(t, req.Tools[len(req.Tools)-1].Description, "Historical")
			if choice == "none" {
				require.Empty(t, req.ToolNames)
				require.Len(t, req.Tools, 1)
			}
		})
	}
}

func TestInvalidToolHistoryRejectedWithoutDroppingContent(t *testing.T) {
	cases := []string{
		`[{"role":"user","content":[{"type":"tool_result","tool_use_id":"missing","content":"x"}]}]`,
		`[{"role":"assistant","content":[{"type":"tool_use","id":"x","name":"read","input":{}}]},{"role":"user","content":"continue"}]`,
		`[{"role":"assistant","content":[{"type":"tool_use","id":"x","name":"read","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"x","content":"a"},{"type":"tool_result","tool_use_id":"x","content":"b"}]}]`,
		`[{"role":"assistant","content":[{"type":"tool_use","id":"x","name":"read","input":[]}]},{"role":"user","content":"continue"}]`,
	}
	for _, history := range cases {
		_, _, err := Convert([]byte(`{"max_tokens":1024,"messages":`+history+`}`), "model", "session", nil)
		require.Error(t, err)
	}
}

func TestToolSchemaPreservesConstraints(t *testing.T) {
	raw := json.RawMessage(`{"type":"object","properties":{"x":{"anyOf":[{"type":"string"},{"type":"null"}]}},"required":["x"],"additionalProperties":false,"$defs":{"a":{"type":"string"}}}`)
	got, err := toolSchema(raw)
	require.NoError(t, err)
	require.JSONEq(t, string(raw), string(got))
	for _, raw := range []string{`[]`, `{"type":"array"}`, `{"required":"x"}`, `{"properties":[]}`} {
		_, err := toolSchema([]byte(raw))
		require.Error(t, err)
	}
}

func TestToolControlsRejectUnsupportedSemantics(t *testing.T) {
	for _, raw := range []string{`{"type":"tool","name":"read"}`, `{"type":"any"}`, `{"type":"auto","disable_parallel_tool_use":true}`} {
		req := Request{ToolChoice: json.RawMessage(raw)}
		require.Error(t, prepareTools(&req))
	}
}

func toolEvent(id, name, input string, stop bool) []byte {
	payload, _ := json.Marshal(map[string]any{"toolUseId": id, "name": name, "input": input, "stop": stop})
	return frameForTest("toolUseEvent", string(payload))
}

func TestInterleavedToolStreamSerializesBlocksAndRestoresNames(t *testing.T) {
	long := strings.Repeat("mcp_server_", 8)
	alias := upstreamToolName(long)
	wire := bytes.Join([][]byte{
		frameForTest("assistantResponseEvent", `{"content":"Reading files."}`),
		toolEvent("one", alias, `{"path":"`, false),
		toolEvent("two", "read", `{"path":"b"}`, true),
		frameForTest("assistantResponseEvent", `{"content":"Both requested."}`),
		toolEvent("one", "", `a"}`, true),
		frameForTest("meteringEvent", `{"inputTokens":12,"outputTokens":8}`),
	}, nil)
	active := -1
	stopped := 0
	args := map[int]string{}
	names := []string{}
	out, err := Decode(bytes.NewReader(wire), "model", 1, func(kind string, data any) error {
		e := data.(map[string]any)
		switch kind {
		case "content_block_start":
			require.Equal(t, -1, active, "overlapping output blocks break Responses clients")
			active = e["index"].(int)
			if b, ok := e["content_block"].(Block); ok {
				names = append(names, b.Name)
			}
		case "content_block_delta":
			require.Equal(t, active, e["index"])
			delta := e["delta"].(map[string]any)
			if s, ok := delta["partial_json"].(string); ok {
				args[active] += s
			}
		case "content_block_stop":
			require.Equal(t, active, e["index"])
			active = -1
			stopped++
		}
		return nil
	}, map[string]string{alias: long, "read": "read"})
	require.NoError(t, err)
	require.Equal(t, -1, active)
	require.Equal(t, 4, stopped)
	require.Equal(t, []string{long, "read"}, names)
	require.JSONEq(t, `{"path":"a"}`, args[1])
	require.JSONEq(t, `{"path":"b"}`, args[2])
	require.Equal(t, "tool_use", out.StopReason)
	require.Equal(t, 12, out.Usage.InputTokens)
	require.JSONEq(t, args[1], string(out.Content[1].Input))
}

func TestToolStreamErrorsNeverComplete(t *testing.T) {
	cases := map[string][]byte{
		"truncated":            toolEvent("a", "read", `{"x":`, false),
		"non_object":           toolEvent("a", "read", `[]`, true),
		"renamed":              append(toolEvent("a", "read", `{`, false), toolEvent("a", "write", `}`, true)...),
		"duplicate_completion": append(toolEvent("a", "read", `{}`, true), toolEvent("a", "", ``, true)...),
		"unknown":              toolEvent("a", "unknown", `{}`, true),
		"two_unfinished":       append(toolEvent("a", "read", `{}`, false), toolEvent("b", "read", `{}`, false)...),
	}
	for name, wire := range cases {
		t.Run(name, func(t *testing.T) {
			complete := false
			_, err := Decode(bytes.NewReader(wire), "model", 1, func(kind string, _ any) error {
				if kind == "message_stop" {
					complete = true
				}
				return nil
			}, map[string]string{"read": "read"})
			require.Error(t, err)
			require.False(t, complete)
		})
	}
	_, err := Decode(bytes.NewReader(toolEvent("a", "read", `{}`, true)), "model", 1, nil, map[string]string{})
	require.ErrorContains(t, err, "disabled")
}

func TestInterleavedToolEmitterPropagatesClientDisconnect(t *testing.T) {
	wire := append(toolEvent("a", "read", `{`, false), toolEvent("b", "read", `{}`, true)...)
	wire = append(wire, toolEvent("a", "", `}`, true)...)
	sentinel := errors.New("client disconnected")
	_, err := Decode(bytes.NewReader(wire), "model", 1, func(kind string, data any) error {
		if kind == "content_block_start" && data.(map[string]any)["index"] == 1 {
			return sentinel
		}
		return nil
	})
	require.ErrorIs(t, err, sentinel)
}

func TestToolsNeverUseResponseCache(t *testing.T) {
	for _, raw := range []string{
		`{"tools":[{"name":"read"}],"messages":[{"role":"user","content":"go"}]}`,
		`{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"a","content":"result"}]}]}`,
	} {
		_, eligible := CacheKey([]byte(raw), "scope")
		require.False(t, eligible, fmt.Sprint(raw))
	}
}

func TestEarlierMeteringCannotCompleteLaterUnfinishedTools(t *testing.T) {
	wire := append(frameForTest("meteringEvent", `{"unit":"credit","usage":0.01}`), toolEvent("a", "read", `{}`, false)...)
	_, err := Decode(bytes.NewReader(wire), "model", 1, nil)
	require.ErrorContains(t, err, "before tool completion")
}
