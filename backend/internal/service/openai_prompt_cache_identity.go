package service

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const openAIAutoPromptCacheKeyPrefix = "sub2api-pc-v1-"

// ensureOpenAIOAuthPromptCacheKey gives stateless OpenAI-compatible clients a
// stable cache-routing identity. Official clients already send
// prompt_cache_key; those values always win. The fallback is derived only from
// the stable conversation prefix and is hashed with the local API-key tenant,
// so request content is never exposed in the upstream routing key and two
// tenants cannot collide on the same prompt.
func ensureOpenAIOAuthPromptCacheKey(c *gin.Context, account *Account, body []byte) ([]byte, string, bool, error) {
	if account == nil || !account.IsOpenAIOAuthLike() || len(body) == 0 || isOpenAIResponsesCompactPath(c) {
		return body, "", false, nil
	}
	if existing := gjson.GetBytes(body, "prompt_cache_key"); existing.Type == gjson.String {
		if value := strings.TrimSpace(existing.String()); value != "" {
			return body, value, false, nil
		}
	}

	apiKeyID := getAPIKeyIDFromContext(c)
	if apiKeyID <= 0 {
		return body, "", false, nil
	}
	seed := deriveOpenAIAnchoredContentSessionSeed(body)
	if seed == "" {
		return body, "", false, nil
	}

	digest := sha256.Sum256([]byte(fmt.Sprintf("sub2api:openai:prompt-cache:v1:tenant:%d:%s", apiKeyID, seed)))
	cacheKey := fmt.Sprintf("%s%x", openAIAutoPromptCacheKeyPrefix, digest[:16])
	updated, err := sjson.SetBytes(body, "prompt_cache_key", cacheKey)
	if err != nil {
		return body, "", false, fmt.Errorf("inject OpenAI prompt cache key: %w", err)
	}
	return updated, cacheKey, true, nil
}
