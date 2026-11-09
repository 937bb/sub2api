package handler

import (
	"context"
	"encoding/json"
	"net/http"
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

// Exercise the real service and both adapters, so an error cannot silently
// become an allow decision or a policy violation between layers.
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

func TestContentModerationUnavailableAdaptersStopToolContinuation(t *testing.T) {
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
		require.Equal(t, securityaudit.DecisionUnavailable, decision.Kind)
		require.Equal(t, http.StatusServiceUnavailable, decision.HTTPStatus)
		require.Equal(t, "content_moderation_unavailable", decision.ErrorCode)
		require.False(t, decision.AllowNextStage)
		require.False(t, decision.Legacy.Flagged)
		_, cached := c.Get(securityAuditCompletedContextKey)
		require.False(t, cached)
	}
}

func TestContentModerationUnavailableWebSocketReturnsRetryableError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, coordinated := range []bool{false, true} {
		svc := newUnavailableModerationService(t)
		h := &OpenAIGatewayHandler{
			gatewayService:           &service.OpenAIGatewayService{},
			billingCacheService:      &service.BillingCacheService{},
			apiKeyService:            &service.APIKeyService{},
			contentModerationService: svc,
			concurrencyHelper:        NewConcurrencyHelper(service.NewConcurrencyService(&concurrencyCacheMock{}), SSEPingFormatNone, time.Second),
		}
		if coordinated {
			h.securityAuditCoordinator = securityaudit.NewCoordinator(securityaudit.NewLegacyModerationAdapter(svc), nil)
		}
		server := newOpenAIWSHandlerTestServer(t, h, middleware.AuthSubject{UserID: 1, Concurrency: 1})
		func() {
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			conn, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/openai/v1/responses", nil)
			require.NoError(t, err)
			defer conn.CloseNow()
			require.NoError(t, conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-test","input":"hello"}`)))
			_, payload, err := conn.Read(ctx)
			require.NoError(t, err)
			require.Contains(t, string(payload), "content_moderation_unavailable")
			require.Contains(t, string(payload), "server_error")
			require.NotContains(t, string(payload), "content_policy_violation")
			_, _, err = conn.Read(ctx)
			require.Equal(t, coderws.StatusTryAgainLater, coderws.CloseStatus(err))
		}()
	}
}
