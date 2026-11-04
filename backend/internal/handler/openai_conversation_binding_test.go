package handler

import (
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
)

func TestConversationBindingErrorReturnsRetryableCode(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	h := &OpenAIGatewayHandler{}
	err := fmt.Errorf("forward: %w", &service.OpenAIConversationError{Status: 503, Code: "conversation_account_unavailable", Message: "Retry the original account later."})
	require.True(t, h.handleConversationBindingError(c, err, false, false))
	require.Equal(t, 503, rec.Code)
	require.Equal(t, "5", rec.Header().Get("Retry-After"))
	require.Contains(t, rec.Body.String(), "conversation_account_unavailable")
}
