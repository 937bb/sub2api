package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/tidwall/gjson"
)

const siwcUsageLimitCode = "subscription_sharing_usage_limit_exceeded"

// This is a local retry cooldown, not an inferred provider quota reset.
const siwcUsageLimitCooldown = time.Minute

const siwcUnsupportedRequestReason GatewayFailureReason = "siwc_request_capability_unsupported"

type siwcCapabilityError struct{ message string }

type siwcRequestCompatibilityKey struct{}

// Classify once before scheduling, without storing request content in context.
// Both scheduler modes must exclude incompatible grants before TopK selection.
func WithSIWCRequestCompatibility(ctx context.Context, body []byte) context.Context {
	blocked := false
	payload := gjson.ParseBytes(body)
	for _, name := range []string{"previous_response_id", "conversation"} {
		value := payload.Get(name)
		blocked = blocked || (value.Exists() && value.Type != gjson.Null && value.String() != "")
	}
	payload.Get("input").ForEach(func(_, item gjson.Result) bool {
		blocked = blocked || siwcUnsupportedInputType(item.Get("type").String())
		return !blocked
	})
	payload.Get("tools").ForEach(func(_, tool gjson.Result) bool {
		switch tool.Get("type").String() {
		case "function", "custom", "tool_search", "web_search", "web_search_preview":
		case "namespace":
			tool.Get("tools").ForEach(func(_, child gjson.Result) bool {
				kind := child.Get("type").String()
				blocked = blocked || (kind != "function" && kind != "custom")
				return !blocked
			})
		default:
			blocked = true
		}
		return !blocked
	})
	return context.WithValue(ctx, siwcRequestCompatibilityKey{}, blocked)
}

func siwcUnsupportedInputType(kind string) bool {
	switch kind {
	case "compaction_trigger", "image_generation_call", "file_search_call", "code_interpreter_call", "mcp_call", "mcp_list_tools", "mcp_approval_request", "mcp_approval_response":
		return true
	}
	return false
}

func siwcRequestIncompatible(ctx context.Context, account *Account) bool {
	if !account.IsOpenAISiwc() || ctx == nil {
		return false
	}
	blocked, _ := ctx.Value(siwcRequestCompatibilityKey{}).(bool)
	return blocked
}

func (e *siwcCapabilityError) Error() string { return e.message }

func siwcCapabilityFailover(message string) *UpstreamFailoverError {
	return &UpstreamFailoverError{
		StatusCode: http.StatusBadRequest, ClientStatusCode: http.StatusBadRequest,
		Stage: GatewayFailureStageInference, Scope: GatewayFailureScopeRequest,
		Reason: siwcUnsupportedRequestReason, ClientMessage: message,
	}
}

func siwcErrorStatus(body []byte) int {
	for _, path := range []string{"response.error.code", "error.code", "code"} {
		switch gjson.GetBytes(body, path).String() {
		case siwcUsageLimitCode:
			return http.StatusTooManyRequests
		case "subscription_sharing_usage_unavailable", "subscription_sharing_user_unavailable":
			return http.StatusServiceUnavailable
		case "subscription_sharing_unsupported_capability":
			return http.StatusBadRequest
		case "subscription_sharing_route_not_supported", "subscription_sharing_user_not_eligible", "chatpass_v2_scope_not_authorized", "chatpass_v2_invalid_authorization_context":
			return http.StatusForbidden
		case "subscription_sharing_invalid_user":
			return http.StatusUnauthorized
		}
	}
	return 0
}

func siwcCredentialFailover() *UpstreamFailoverError {
	return &UpstreamFailoverError{
		StatusCode: http.StatusServiceUnavailable, ClientStatusCode: http.StatusServiceUnavailable,
		Stage: GatewayFailureStageAccountAuth, Scope: GatewayFailureScopeAccount,
		Reason:        "siwc_credential_unavailable",
		ClientMessage: "No usable SIWC authorization is currently available",
	}
}

func siwcErrorShouldFailover(body []byte) bool {
	switch siwcErrorStatus(body) {
	case http.StatusUnauthorized, http.StatusTooManyRequests, http.StatusServiceUnavailable:
		return true
	}
	for _, path := range []string{"response.error.code", "error.code", "code"} {
		if gjson.GetBytes(body, path).String() == "subscription_sharing_user_not_eligible" {
			return true
		}
	}
	return false
}

func (s *RateLimitService) handleSIWCUsageLimit(ctx context.Context, account *Account) {
	now := time.Now()
	until := now.Add(siwcUsageLimitCooldown)
	state := &TempUnschedState{
		UntilUnix: until.Unix(), TriggeredAtUnix: now.Unix(),
		StatusCode: http.StatusTooManyRequests, RuleIndex: -1,
		MatchedKeyword: siwcUsageLimitCode,
		ErrorMessage:   "SIWC shared usage limit reached; local retry cooldown only. Check ChatGPT Settings > Usage and app limits. Provider reset time is unknown.",
	}
	reason, err := json.Marshal(state)
	if err != nil {
		return
	}
	s.notifyAccountSchedulingBlocked(account, until, siwcUsageLimitCode)
	if s.accountRepo != nil {
		if err := s.accountRepo.SetTempUnschedulable(ctx, account.ID, until, string(reason)); err != nil {
			slog.Warn("siwc_usage_cooldown_persist_failed", "account_id", account.ID, "error", err)
		}
	}
	if s.tempUnschedCache != nil {
		if err := s.tempUnschedCache.SetTempUnsched(ctx, account.ID, state); err != nil {
			slog.Warn("siwc_usage_cooldown_cache_failed", "account_id", account.ID, "error", err)
		}
	}
}
