package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCodexTurnCostsReturnsNativeEmptyEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/analytics/codex/turn-costs", strings.NewReader(`{"turn_ids":["turn-fixture"]}`))

	(&OpenAIGatewayHandler{}).CodexTurnCosts(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "application/json; charset=utf-8", recorder.Header().Get("Content-Type"))
	var response struct {
		Turns []any `json:"turns"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.NotNil(t, response.Turns)
	require.Empty(t, response.Turns)
}
