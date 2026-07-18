package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const emulatedWebSearchHistoryBody = `{"model":"claude-sonnet-4-6","messages":[` +
	`{"role":"user","content":[{"type":"text","text":"search"}]},` +
	`{"role":"assistant","content":[` +
	`{"type":"server_tool_use","id":"srvtoolu_ws_0123456789abcdef","name":"web_search","input":{"query":"weather"}},` +
	`{"type":"web_search_tool_result","tool_use_id":"srvtoolu_ws_0123456789abcdef","content":[]},` +
	`{"type":"text","text":"search summary"}]}]}`

const genuineWebSearchHistoryBody = `{"model":"claude-sonnet-4-6","messages":[` +
	`{"role":"assistant","content":[` +
	`{"type":"server_tool_use","id":"srvtoolu_01ABCDEF","name":"web_search","input":{"query":"weather"}},` +
	`{"type":"web_search_tool_result","tool_use_id":"srvtoolu_01ABCDEF","content":[]},` +
	`{"type":"text","text":"genuine summary"}]}]}`

func webSearchHistoryContentTypes(body []byte) []string {
	var types []string
	for _, message := range gjson.GetBytes(body, "messages").Array() {
		for _, block := range message.Get("content").Array() {
			types = append(types, block.Get("type").String())
		}
	}
	return types
}

func TestFilterWebSearchHistoryBlocks_StripsOnlyEmulatedBlocksForAnthropicStrict(t *testing.T) {
	out := FilterWebSearchHistoryBlocks([]byte(emulatedWebSearchHistoryBody), "claude-sonnet-4-6")
	require.Equal(t, []string{"text", "text"}, webSearchHistoryContentTypes(out))
	require.Contains(t, string(out), "search summary")
	require.NotContains(t, string(out), webSearchToolUseIDPrefix)

	genuine := []byte(genuineWebSearchHistoryBody)
	require.Equal(t, genuine, FilterWebSearchHistoryBlocks(genuine, "claude-sonnet-4-6"))
}

func TestFilterWebSearchHistoryBlocks_StripsAllSearchBlocksForPassbackRequired(t *testing.T) {
	out := FilterWebSearchHistoryBlocks([]byte(genuineWebSearchHistoryBody), "glm-4.7")
	require.Equal(t, []string{"text"}, webSearchHistoryContentTypes(out))
	require.Contains(t, string(out), "genuine summary")
}

func TestFilterWebSearchHistoryBlocks_PreservesMessageShape(t *testing.T) {
	body := []byte(`{"model":"glm-4.7","messages":[{"role":"assistant","content":[{"type":"server_tool_use","id":"srvtoolu_01X","name":"web_search","input":{}},{"type":"web_search_tool_result","tool_use_id":"srvtoolu_01X","content":[]}]}]}`)
	out := FilterWebSearchHistoryBlocks(body, "glm-4.7")
	require.True(t, gjson.ValidBytes(out))
	require.Equal(t, "assistant", gjson.GetBytes(out, "messages.0.role").String())
	require.Equal(t, "text", gjson.GetBytes(out, "messages.0.content.0.type").String())
	require.Equal(t, "(assistant content removed)", gjson.GetBytes(out, "messages.0.content.0.text").String())
}

func TestFilterWebSearchHistoryBlocks_NoRelevantBlocksReturnsOriginalBody(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":"explain server_tool_use"}]}`)
	out := FilterWebSearchHistoryBlocks(body, "glm-4.7")
	require.Equal(t, body, out)
}
