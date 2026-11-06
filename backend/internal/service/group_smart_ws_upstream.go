package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sync/atomic"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/smartws"
	"github.com/gin-gonic/gin"
)

// The adapter is release-owned and has no dependency on shared plugin install state.
// It uses the same fully prepared account URL, credentials, headers and proxy as HTTP.
type groupSmartWSAttemptKey struct{}

var errSmartWSAlreadyDispatched = errors.New("smart WS request already dispatched")

type groupSmartWSAttempt struct {
	sent     atomic.Bool
	apiKeyID int64
}

func beginGroupSmartWSAttempt(ctx context.Context, c *gin.Context) (context.Context, *groupSmartWSAttempt) {
	if existing, ok := ctx.Value(groupSmartWSAttemptKey{}).(*groupSmartWSAttempt); ok {
		return ctx, existing
	}
	attempt := &groupSmartWSAttempt{apiKeyID: getAPIKeyIDFromContext(c)}
	return context.WithValue(ctx, groupSmartWSAttemptKey{}, attempt), attempt
}
func finishGroupSmartWSAttempt(attempt *groupSmartWSAttempt, err error) error {
	if err == nil || attempt == nil || !attempt.sent.Load() {
		return err
	}
	// Core's normal early-stream failover policy must not replay a WS generation.
	sentErr := &PluginTransportError{Code: "SMART_WS_REQUEST_SENT",
		Message: "WS upstream attempt ended after dispatch; automatic replay is disabled", RequestSent: true}
	return sentErr
}

var groupSmartWSUpstreamPool, groupSmartWSTuningError = newGroupSmartWSUpstreamPool()

func newGroupSmartWSUpstreamPool() (*smartws.Pool, string) {
	tuning, err := smartws.TuningFromEnvironment(os.LookupEnv)
	if err != nil {
		return smartws.NewPoolWithDialer(dialGroupSmartWS), err.Error()
	}
	pool, err := smartws.NewPoolWithTuning(dialGroupSmartWS, tuning)
	if err != nil {
		return smartws.NewPoolWithDialer(dialGroupSmartWS), err.Error()
	}
	return pool, ""
}

func GroupSmartWSRuntimeStats() map[string]any {
	stats := groupSmartWSUpstreamPool.Stats()
	stats["tuning_error"] = groupSmartWSTuningError
	stats["eligibility"] = groupSmartWSEligibilitySnapshot()
	return stats
}

// CloseGroupSmartWSPool is called after the HTTP server has drained requests.
func CloseGroupSmartWSPool() { groupSmartWSUpstreamPool.Close() }

func groupSmartWSIdentity(ctx context.Context) (int64, string) {
	group := gatewayTokenRequestBillingGroupFromContext(ctx)
	if group == nil {
		group, _ = ctx.Value(ctxkey.Group).(*Group)
	}
	if !IsGroupContextValid(group) || group.Platform != PlatformOpenAI {
		return 0, ""
	}
	userID, _ := ctx.Value(ctxkey.UserID).(int64)
	if userID <= 0 {
		return 0, ""
	}
	attempt, _ := ctx.Value(groupSmartWSAttemptKey{}).(*groupSmartWSAttempt)
	if attempt == nil || attempt.apiKeyID <= 0 {
		return 0, ""
	}
	return group.ID, fmt.Sprintf("group:%d/user:%d/key:%d", group.ID, userID, attempt.apiKeyID)
}

func (s *OpenAIGatewayService) tryGroupSmartWS(request *http.Request, proxyURL string, account *Account) (*http.Response, bool, error) {
	if s == nil || s.settingService == nil || request == nil || account == nil ||
		account.Platform != PlatformOpenAI || account.Type != AccountTypeOAuth {
		groupSmartWSEligibility.unsupported.Add(1)
		return nil, false, nil
	}
	if account.IsOpenAIWSForceHTTPEnabled() {
		groupSmartWSEligibility.accountForceHTTP.Add(1)
		return nil, false, nil
	}
	if account.Concurrency <= 0 {
		groupSmartWSEligibility.accountConcurrency.Add(1)
		return nil, false, nil
	}
	if s.cfg == nil || !s.cfg.Gateway.OpenAIWS.Enabled || !s.cfg.Gateway.OpenAIWS.OAuthEnabled || !s.cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 || s.cfg.Gateway.OpenAIWS.ForceHTTP {
		groupSmartWSEligibility.globalDisabled.Add(1)
		return nil, false, nil
	}
	// Explicit account modes take precedence over legacy enabled flags.
	switch account.ResolveOpenAIResponsesWebSocketV2Mode(OpenAIWSIngressModeOff) {
	case OpenAIWSIngressModeCtxPool, OpenAIWSIngressModePassthrough, OpenAIWSIngressModeShared, OpenAIWSIngressModeDedicated:
	default:
		groupSmartWSEligibility.accountMode.Add(1)
		return nil, false, nil
	}
	groupID, scope := groupSmartWSIdentity(request.Context())
	if groupID <= 0 {
		groupSmartWSEligibility.missingIdentity.Add(1)
		return nil, false, nil
	}
	if !s.settingService.GroupSmartWSEnabled(request.Context(), groupID) {
		groupSmartWSEligibility.groupDisabled.Add(1)
		return nil, false, nil
	}
	groupSmartWSEligibility.poolAttempt.Add(1)
	request = request.WithContext(smartws.WithAccountID(request.Context(), account.ID))
	resp, handled, err := groupSmartWSUpstreamPool.Try(request, proxyURL, fmt.Sprintf("%s/account:%d", scope, account.ID))
	attempt, _ := request.Context().Value(groupSmartWSAttemptKey{}).(*groupSmartWSAttempt)
	var sent *smartws.SentError
	if attempt != nil && ((resp != nil && resp.Header.Get("X-Sub2-Upstream-Transport") == "websocket") || errors.As(err, &sent)) {
		attempt.sent.Store(true)
	}
	if errors.As(err, &sent) {
		return resp, true, &PluginTransportError{
			Code:        "SMART_WS_DISPATCH_UNCERTAIN",
			Message:     "WS upstream request may have been sent; automatic replay is disabled",
			RequestSent: true,
		}
	}
	return resp, handled, err
}

// These fixed-cardinality counters describe adapter invocations, not unique
// requests. Native WS traffic does not invoke this HTTP adapter.
var groupSmartWSEligibility struct {
	unsupported, accountForceHTTP, accountConcurrency atomic.Uint64
	globalDisabled, accountMode, missingIdentity      atomic.Uint64
	groupDisabled, poolAttempt                        atomic.Uint64
}

func groupSmartWSEligibilitySnapshot() map[string]uint64 {
	return map[string]uint64{
		"skip_unsupported":         groupSmartWSEligibility.unsupported.Load(),
		"skip_account_force_http":  groupSmartWSEligibility.accountForceHTTP.Load(),
		"skip_account_concurrency": groupSmartWSEligibility.accountConcurrency.Load(),
		"skip_global_disabled":     groupSmartWSEligibility.globalDisabled.Load(),
		"skip_account_mode":        groupSmartWSEligibility.accountMode.Load(),
		"skip_missing_identity":    groupSmartWSEligibility.missingIdentity.Load(),
		"skip_group_disabled":      groupSmartWSEligibility.groupDisabled.Load(),
		"pool_attempt":             groupSmartWSEligibility.poolAttempt.Load(),
	}
}
