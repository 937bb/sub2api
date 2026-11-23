package service

import (
	"strings"

	"github.com/tidwall/gjson"
)

// Codex injects the Goal objective as additional context after compaction or
// continuation. It uses the function_call_output envelope without a tool call.
// Recognize only this known shape; preserve the original item for forwarding.
func isCodexGoalContextOutput(item map[string]any) bool {
	if item["type"] != "function_call_output" || item["namespace"] != "agent" || item["name"] != "goal_objective" {
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

func isCodexGoalContextOutputJSON(item gjson.Result) bool {
	if item.Get("type").String() != "function_call_output" || item.Get("namespace").String() != "agent" || item.Get("name").String() != "goal_objective" || item.Get("output").Type != gjson.String {
		return false
	}
	callID := item.Get("call_id")
	return callID.Type == gjson.Null || (callID.Type == gjson.String && strings.TrimSpace(callID.String()) == "")
}
