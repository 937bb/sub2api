package service

import (
	"strings"

	"github.com/tidwall/gjson"
)

// Codex AdditionalContextToolOutput carries dynamically named context (Goal,
// browser pages, selections, etc.) in the agent namespace without a tool call.
// Recognize its wire shape, not individual names, and preserve the original item.
// A nonempty call_id always identifies an ordinary paired tool output.
func isCodexAgentContextOutput(item map[string]any) bool {
	if item["type"] != "function_call_output" || item["namespace"] != "agent" {
		return false
	}
	name, ok := item["name"].(string)
	if !ok || strings.TrimSpace(name) == "" {
		return false
	}
	if _, ok := item["output"].(string); !ok {
		return false
	}
	callID := item["call_id"]
	if callID == nil {
		return true
	}
	id, ok := callID.(string)
	return ok && strings.TrimSpace(id) == ""
}

func isCodexAgentContextOutputJSON(item gjson.Result) bool {
	if item.Get("type").String() != "function_call_output" || item.Get("namespace").String() != "agent" || item.Get("name").Type != gjson.String || strings.TrimSpace(item.Get("name").String()) == "" || item.Get("output").Type != gjson.String {
		return false
	}
	callID := item.Get("call_id")
	return callID.Type == gjson.Null || (callID.Type == gjson.String && strings.TrimSpace(callID.String()) == "")
}
