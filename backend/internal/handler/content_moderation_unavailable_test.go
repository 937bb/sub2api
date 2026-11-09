package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Exercise the real service and both adapters: audit outages must allow
// the next stage without becoming content violations.
func newUnavailableModerationService(t *testing.T) *service.ContentModerationService {
	t.Helper()
	cfg, err := json.Marshal(&service.ContentModerationConfig{
		Enabled: true, Mode: service.ContentModerationModePreBlock,
		AllGroups: true, SampleRate: 100, KeywordBlockingMode: service.ContentModerationKeywordModeAPIOnly,
	})
	require.NoError(t, err)
	settings := &contentModerationHandlerSettingRepo{values: map[string]string{
		service.SettingKeyRiskControlEnabled:      "true",
		service.SettingKeyContentModerationConfig: string(cfg),
	}}
	return service.NewContentModerationService(settings, &contentModerationHandlerTestRepo{}, nil, nil, nil, nil, nil, nil)
}

func TestContentModerationUnavailableAdaptersAllowToolContinuation(t *testing.T) {
	for _, coordinated := range []bool{false, true} {
		svc := newUnavailableModerationService(t)
		var coordinator *securityaudit.Coordinator
		if coordinated {
			coordinator = securityaudit.NewCoordinator(securityaudit.NewLegacyModerationAdapter(svc), nil)
		}
		c, _ := securityAuditErrorTestContext(t)
		decision := runSecurityAudit(c, nil, coordinator, svc, nil, middleware.AuthSubject{UserID: 1}, service.ContentModerationProtocolOpenAIResponses, "gpt-test",
			[]byte(`{"input":[{"type":"function_call_output","call_id":"call_1","output":"code review results"}]}`), "subsequent_turn")
		require.NotNil(t, decision)
		require.Equal(t, securityaudit.DecisionAllow, decision.Kind)
		require.Equal(t, http.StatusOK, decision.HTTPStatus)
		require.Empty(t, decision.ErrorCode)
		require.True(t, decision.AllowNextStage)
		require.False(t, decision.Legacy.Flagged)
		_, cached := c.Get(securityAuditCompletedContextKey)
		require.False(t, cached)
	}
}

func TestContentModerationUnavailableWebSocketKeepsTurnsOpen(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, coordinated := range []bool{false, true} {
		svc := newUnavailableModerationService(t)
		var coordinator *securityaudit.Coordinator
		if coordinated {
			coordinator = securityaudit.NewCoordinator(securityaudit.NewLegacyModerationAdapter(svc), nil)
		}
		router := gin.New()
		router.GET("/responses", func(c *gin.Context) {
			conn, err := coderws.Accept(c.Writer, c.Request, nil)
			if err != nil {
				t.Error(err)
				return
			}
			defer conn.CloseNow()
			for _, stage := range []string{"first_turn", "subsequent_turn"} {
				_, body, err := conn.Read(c.Request.Context())
				if err != nil {
					t.Error(err)
					return
				}
				decision := runSecurityAudit(c, nil, coordinator, svc, nil, middleware.AuthSubject{UserID: 1}, service.ContentModerationProtocolOpenAIResponses, "gpt-test", body, stage)
				if decision == nil || !decision.AllowNextStage {
					t.Error("audit outage stopped WS turn")
					return
				}
				if err := conn.Write(c.Request.Context(), coderws.MessageText, []byte(`{"continued":true}`)); err != nil {
					t.Error(err)
					return
				}
			}
			_ = conn.Close(coderws.StatusNormalClosure, "done")
		})
		server := httptest.NewServer(router)
		func() {
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			conn, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/responses", nil)
			require.NoError(t, err)
			defer conn.CloseNow()
			for _, body := range []string{`{"input":"hello"}`, `{"input":[{"type":"function_call_output","output":"normal tool result"}]}`} {
				require.NoError(t, conn.Write(ctx, coderws.MessageText, []byte(body)))
				_, payload, err := conn.Read(ctx)
				require.NoError(t, err)
				require.JSONEq(t, `{"continued":true}`, string(payload))
			}
			_, _, err = conn.Read(ctx)
			require.Equal(t, coderws.StatusNormalClosure, coderws.CloseStatus(err))
		}()
	}
}
