package service

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	ompClientUserAgentPrefix         = "omp/"
	ompResponsesPromptCacheKeyPrefix = "compat_omp_"
)

// deriveOMPResponsesPromptCacheKey supplies the stable cache-routing identity
// that OMP's native Responses requests omit. Explicit client cache keys always
// win; this fallback is deliberately limited to OMP so unrelated Responses
// clients keep their existing wire behavior.
func deriveOMPResponsesPromptCacheKey(c *gin.Context, body []byte, upstreamModel string) string {
	if !isOMPClient(c) || len(body) == 0 || !shouldAutoInjectPromptCacheKeyForCompat(upstreamModel) {
		return ""
	}
	if strings.TrimSpace(gjson.GetBytes(body, "prompt_cache_key").String()) != "" {
		return ""
	}

	seed := explicitOpenAIHeaderSessionID(c)
	if seed == "" {
		sessionID, threadID := codexScopedBodyConversation(body)
		seed = strings.TrimSpace(sessionID)
		if seed == "" {
			seed = strings.TrimSpace(threadID)
		}
	}
	if seed == "" {
		seed = deriveOpenAIAnchoredContentSessionSeed(body)
	}
	if seed == "" {
		return ""
	}

	model := strings.ToLower(strings.TrimSpace(canonicalizeOpenAIModelAliasSpelling(upstreamModel)))
	return ompResponsesPromptCacheKeyPrefix + hashSensitiveValueForLog(fmt.Sprintf("model=%s|seed=%s", model, seed))
}

func isOMPClient(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(c.GetHeader("User-Agent"))), ompClientUserAgentPrefix)
}
