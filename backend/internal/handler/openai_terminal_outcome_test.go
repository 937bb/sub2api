package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type terminalOutcomeSettingsRepo struct {
	service.SettingRepository
	ignoreCanceled bool
}

func (r *terminalOutcomeSettingsRepo) GetValue(context.Context, string) (string, error) {
	return "{}", nil
}

func (r *terminalOutcomeSettingsRepo) GetMultiple(context.Context, []string) (map[string]string, error) {
	return map[string]string{service.SettingKeyOpsAdvancedSettings: fmt.Sprintf(`{"ignore_context_canceled":%t}`, r.ignoreCanceled)}, nil
}

func TestOpsTerminalCancellationAfterHiddenRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, ignore := range []bool{false, true} {
		t.Run(fmt.Sprint(ignore), func(t *testing.T) {
			setupOpsErrorLogTestQueue(t, 4)
			ops := service.NewOpsService(nil, &terminalOutcomeSettingsRepo{ignoreCanceled: ignore}, nil, nil, nil, nil, nil, nil, nil, nil, nil)
			router := gin.New()
			router.Use(OpsErrorLoggerMiddleware(ops))
			router.POST("/v1/responses", func(c *gin.Context) {
				c.Set(service.OpsUpstreamErrorsKey, []*service.OpsUpstreamErrorEvent{{UpstreamStatusCode: 502, Message: "hidden retry failure"}})
				c.Header("Content-Type", "text/event-stream")
				c.Writer.WriteHeaderNow()
				service.MarkOpenAIForwardTerminalFailure(c, &service.OpenAIForwardResult{OpenAIWSMode: true, ClientDisconnect: true}, context.Canceled)
			})
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/responses", nil))
			require.Equal(t, 200, rec.Code)
			if ignore {
				require.Zero(t, OpsErrorLogQueueLength())
				return
			}
			require.EqualValues(t, 1, OpsErrorLogQueueLength())
			entry := (<-opsErrorLogQueue).entry
			require.Equal(t, 499, entry.StatusCode)
			require.Equal(t, "client", entry.ErrorOwner)
			require.Equal(t, "client_request", entry.ErrorSource)
			require.Contains(t, entry.ErrorMessage, "context canceled")
			require.NotContains(t, entry.ErrorMessage, "Recovered")
		})
	}
}

func TestOpsWire499UsesTerminalMarkerWithoutReplacingExplicitError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, ignore := range []bool{false, true} {
		for _, explicit := range []bool{false, true} {
			t.Run(fmt.Sprintf("ignore_%v_explicit_%v", ignore, explicit), func(t *testing.T) {
				setupOpsErrorLogTestQueue(t, 4)
				ops := service.NewOpsService(nil, &terminalOutcomeSettingsRepo{ignoreCanceled: ignore}, nil, nil, nil, nil, nil, nil, nil, nil, nil)
				router := gin.New()
				router.Use(OpsErrorLoggerMiddleware(ops))
				router.POST("/v1/responses", func(c *gin.Context) {
					c.Set(service.OpsUpstreamErrorsKey, []*service.OpsUpstreamErrorEvent{{UpstreamStatusCode: 502, Message: "hidden retry failure"}})
					ctx, cancel := context.WithCancel(c.Request.Context())
					cancel()
					c.Request = c.Request.WithContext(ctx)
					service.MarkOpenAIForwardTerminalFailure(c, &service.OpenAIForwardResult{OpenAIWSMode: true, ClientDisconnect: true}, context.Canceled)
					if explicit {
						c.Data(499, "application/json", []byte(`{"error":{"type":"upstream_error","message":"explicit upstream failure"}}`))
					} else {
						c.Status(499)
						c.Writer.WriteHeaderNow()
					}
				})
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/responses", nil))
				require.Equal(t, 499, rec.Code)
				if ignore && !explicit {
					require.Zero(t, OpsErrorLogQueueLength())
					return
				}
				require.EqualValues(t, 1, OpsErrorLogQueueLength())
				entry := (<-opsErrorLogQueue).entry
				require.Equal(t, 499, entry.StatusCode)
				if explicit {
					require.Equal(t, "explicit upstream failure", entry.ErrorMessage)
					require.Equal(t, "provider", entry.ErrorOwner)
				} else {
					require.Contains(t, entry.ErrorMessage, "context canceled")
					require.Equal(t, "client", entry.ErrorOwner)
				}
			})
		}
	}
}

func TestSelectionChannelRestrictionIsLocalModelError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	fallback := noAccountErrorClassification{Status: 503, ErrType: "api_error", Message: "Service temporarily unavailable"}
	cls := classifySelectionFailureErrorFromGin(c, fmt.Errorf("selection: %w", service.ErrOpenAIChannelModelRestricted), fallback)
	require.Equal(t, 404, cls.Status)
	require.Equal(t, "model_not_found", cls.ErrType)
	require.Contains(t, cls.Message, "channel model restrictions")
	require.True(t, service.HasOpsClientBusinessLimited(c))
	phase, limited, owner, source := classifyOpsErrorLog(c, cls.ErrType, cls.Message, "model_not_found", cls.Status)
	require.Equal(t, "routing", phase)
	require.True(t, limited)
	require.Equal(t, "platform", owner)
	require.Equal(t, "gateway", source)
	prior := noAccountErrorClassification{Status: 404, ErrType: "model_not_found", Message: "specific model mismatch", ModelNotFound: true}
	require.Equal(t, prior, classifySelectionFailureError(service.ErrOpenAIChannelModelRestricted, prior))
	require.Equal(t, fallback, classifySelectionFailureError(service.ErrNoAvailableAccounts, fallback))
}
