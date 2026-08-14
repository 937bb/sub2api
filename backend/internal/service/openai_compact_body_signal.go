package service

import (
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const openAICompactRequestContextKey = "openai_compact_request"

func MarkOpenAICompactRequest(c *gin.Context) {
	if c != nil {
		c.Set(openAICompactRequestContextKey, true)
	}
}

func IsOpenAIResponsesCompactRequest(c *gin.Context) bool {
	return isOpenAIResponsesCompactPath(c)
}

// HasCompactionTriggerInInput detects an input item with
// type="compaction_trigger". The handler combines this body signal with the
// request path, stream flag, and Codex beta feature header to distinguish the
// native remote compaction v2 wire from the legacy /responses/compact bridge.
func HasCompactionTriggerInInput(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return false
	}
	found := false
	input.ForEach(func(_, item gjson.Result) bool {
		if item.Get("type").String() == "compaction_trigger" {
			found = true
			return false
		}
		return true
	})
	return found
}
