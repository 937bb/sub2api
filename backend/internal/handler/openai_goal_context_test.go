package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestValidateFunctionCallOutputRequestGoalContext(t *testing.T) {
	const goal = `{"type":"function_call_output","namespace":"agent","name":"goal_objective","output":"Continue","call_id":null}`
	for _, tt := range []struct {
		name, input string
		allowed     bool
	}{
		{"goal", goal, true},
		{"browser_page", `{"type":"function_call_output","namespace":"agent","name":"codex_apps_open_page","output":"page context"}`, true},
		{"browser_selection", `{"type":"function_call_output","namespace":"agent","name":"codex_apps_selected_page_part_1_of_1","output":"selected context"}`, true},
		{"unnamed_agent_output", `{"type":"function_call_output","namespace":"agent","output":"bad"}`, false},
		{"paired_browser_output", `{"type":"function_call_output","namespace":"agent","name":"codex_apps_open_page","call_id":"call_browser","output":"result"}`, false},
		{"goal_and_missing_id", goal + `,{"type":"function_call_output","output":"bad"}`, false},
		{"goal_and_unreferenced", goal + `,{"type":"function_call_output","call_id":"call_a","output":"result"}`, false},
		{"goal_and_referenced", goal + `,{"type":"item_reference","id":"call_a"},{"type":"function_call_output","call_id":"call_a","output":"result"}`, true},
		{"dynamic_agent_context", `{"type":"function_call_output","namespace":"agent","name":"other_tool","output":"bad","call_id":null}`, true},
		{"goal_wrong_namespace", `{"type":"function_call_output","namespace":"functions","name":"goal_objective","output":"bad","call_id":null}`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/responses", nil)
			h := &OpenAIGatewayHandler{}
			body := []byte(`{"input":[` + tt.input + `]}`)
			require.Equal(t, tt.allowed, h.validateFunctionCallOutputRequest(c, body, zap.NewNop()))
			if tt.allowed {
				require.False(t, c.Writer.Written())
			} else {
				require.Equal(t, http.StatusBadRequest, w.Code)
			}
		})
	}
}
