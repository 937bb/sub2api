package service

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/net/http/httpguts"
)

const (
	openAICodexParentThreadIDHeader = "x-codex-parent-thread-id"
	openAICodexSubagentHeader       = "x-openai-subagent"

	openAICodexDelegationHeaderMaxBytes = 256
)

// copyOpenAICodexDelegationHeaders preserves the current Codex Desktop thread
// lineage markers on the ChatGPT subscription path. These headers are private
// Codex context, so ordinary API-key traffic and clients that were not
// identified as Codex must not be allowed to synthesize them upstream.
func copyOpenAICodexDelegationHeaders(
	c *gin.Context,
	account *Account,
	isCodexClient bool,
	dst http.Header,
) {
	if c == nil || c.Request == nil || account == nil || !account.IsOpenAIOAuth() || !isCodexClient || dst == nil {
		return
	}

	for _, name := range [...]string{
		openAICodexParentThreadIDHeader,
		openAICodexSubagentHeader,
	} {
		value := strings.TrimSpace(c.Request.Header.Get(name))
		if !isValidOpenAICodexDelegationHeaderValue(value) {
			continue
		}
		dst.Set(name, value)
	}
}

func isValidOpenAICodexDelegationHeaderValue(value string) bool {
	if value == "" || len(value) > openAICodexDelegationHeaderMaxBytes || !httpguts.ValidHeaderFieldValue(value) {
		return false
	}
	for i := 0; i < len(value); i++ {
		if value[i] < 0x20 || value[i] > 0x7e {
			return false
		}
	}
	return true
}
