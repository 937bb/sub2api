package service

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	ompClientUserAgentPrefix         = "omp/"
	ompResponsesPromptCacheKeyPrefix = "compat_omp_"
)

// applyOMPResponsesPromptCacheKey runs before request transformations and route
// splitting. Modern OMP sends its own key; only missing-key compatibility uses
// this fallback, with the same source payload on normal and passthrough paths.
func applyOMPResponsesPromptCacheKey(c *gin.Context, account *Account, body []byte) ([]byte, error) {
	if account == nil || !account.UsesOpenAICodexProtocol() || isOpenAIResponsesCompactPath(c) || !isOMPClient(c) {
		return body, nil
	}
	model := account.GetMappedModel(gjson.GetBytes(body, "model").String())
	if mapped, ok := openAIGroupMappedModel(c); ok {
		model = mapped
	}
	key := deriveOMPResponsesPromptCacheKey(c, body, model)
	if key == "" {
		return body, nil
	}
	next, err := sjson.SetBytes(body, "prompt_cache_key", key)
	if err != nil {
		return nil, fmt.Errorf("set OMP Responses cache key: %w", err)
	}
	return next, nil
}

// deriveOMPResponsesPromptCacheKey handles OMP requests whose explicit cache
// key was omitted. Existing keys always win, including those intentionally
// shared across separate conversations. Other clients keep their wire behavior.
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
