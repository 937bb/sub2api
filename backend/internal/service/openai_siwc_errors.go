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

func siwcErrorStatus(body []byte) int {
	for _, path := range []string{"response.error.code", "error.code", "code"} {
		switch gjson.GetBytes(body, path).String() {
		case siwcUsageLimitCode:
			return http.StatusTooManyRequests
		case "subscription_sharing_usage_unavailable":
			return http.StatusServiceUnavailable
		case "subscription_sharing_unsupported_capability":
			return http.StatusBadRequest
		case "subscription_sharing_route_not_supported":
			return http.StatusForbidden
		}
	}
	return 0
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
