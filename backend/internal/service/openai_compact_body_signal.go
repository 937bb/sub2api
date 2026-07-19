package service

import (
	"context"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

type openAICompactBodySignalContextKey struct{}

func HasOpenAICompactionTriggerInInput(body []byte) bool {
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

func isBareOpenAIResponsesPath(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return false
	}
	normalizedPath := strings.TrimRight(strings.TrimSpace(c.Request.URL.Path), "/")
	return strings.HasSuffix(normalizedPath, "/responses")
}

// PromoteOpenAICompactBodySignal marks an official Codex body signal before
// scheduling. Account type remains a terminal adapter decision after selection.
func PromoteOpenAICompactBodySignal(c *gin.Context, body []byte, forceCodexCLI bool) bool {
	if c == nil || c.Request == nil || !isBareOpenAIResponsesPath(c) || !HasOpenAICompactionTriggerInInput(body) {
		return false
	}
	if !forceCodexCLI && !openai.IsCodexOfficialClientByHeadersStrict(c.GetHeader("User-Agent"), c.GetHeader("originator")) {
		return false
	}
	c.Request.URL.Path = strings.TrimRight(c.Request.URL.Path, "/") + "/compact"
	c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), openAICompactBodySignalContextKey{}, true))
	if stream := gjson.GetBytes(body, "stream"); stream.Type == gjson.True {
		c.Set(openAICompactClientStreamKey, true)
	}
	return true
}

func isOpenAICompactBodySignalRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	promoted, _ := c.Request.Context().Value(openAICompactBodySignalContextKey{}).(bool)
	return promoted
}

func IsOpenAICompactBodySignalRequest(c *gin.Context) bool {
	return isOpenAICompactBodySignalRequest(c)
}
