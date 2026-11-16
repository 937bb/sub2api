package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CodexTurnCosts serves the auxiliary turn-cost lookup used by Codex Desktop
// when it runs against an API-key provider. sub2api cannot safely associate a
// client turn ID with one upstream OAuth account, so it returns the native empty
// envelope locally instead of scheduling an account or forwarding the lookup.
func (h *OpenAIGatewayHandler) CodexTurnCosts(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"turns": []any{}})
}
