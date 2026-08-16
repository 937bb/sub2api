package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	openAIQuotaBypassEnabledContextKey     = "openai_quota_bypass_enabled"
	openAIQuotaBypassAppliedContextKey     = "openai_quota_bypass_applied"
	openAIQuotaBypassInjectPairsContextKey = "openai_quota_bypass_inject_pairs"
	openAIQuotaBypassResponseHeader        = "X-Sub2API-Quota-Bypass"
	openAIQuotaBypassPairsResponseHeader   = "X-Sub2API-Quota-Bypass-Pairs"
	openAIQuotaBypassSameAccountRetries    = 3
)

// IsQuotaBypassEligible reports whether an account qualifies for Codex quota
// bypass injection through an account override, the request group, or any group
// attached to the scheduled account. Eligibility is intentionally independent
// of plan_type, so Plus, Team, Pro and other OpenAI OAuth plans behave alike.
// Eligibility is additive: an account-level true value or any enabled group is
// sufficient. A stored false value means only that the account-level switch is
// off; it must not disable bypass inherited from a request or attached group.
func IsQuotaBypassEligible(account *Account, group *Group) bool {
	if account == nil || account.Platform != PlatformOpenAI || !account.IsOAuth() {
		return false
	}
	if account.Extra != nil {
		if v, ok := account.Extra["quota_bypass_enabled"].(bool); ok && v {
			return true
		}
	}
	if group != nil && group.QuotaBypassEnabled {
		return true
	}
	for _, g := range account.Groups {
		if g != nil && g.QuotaBypassEnabled {
			return true
		}
	}
	for _, ag := range account.AccountGroups {
		if ag.Group != nil && ag.Group.QuotaBypassEnabled {
			return true
		}
	}
	return false
}

// IsAccountQuotaBypassEligible checks whether an account is bypass-eligible
// via its own Extra flag or any of its attached Groups.
func IsAccountQuotaBypassEligible(account *Account) bool {
	return IsQuotaBypassEligible(account, nil)
}

// IsAccountQuotaBypassConcentrated reports whether an account explicitly opts
// into fill-first scheduling through an attached marker group. Both switches
// must be enabled on the same group so a dedicated bypass group can mark
// accounts used by other public request groups without turning those public
// groups into bypass groups themselves. The account-level bypass override still
// enables injection, but concentration remains controlled by a group switch.
func IsAccountQuotaBypassConcentrated(account *Account) bool {
	if account == nil || account.Platform != PlatformOpenAI || !account.IsOAuth() {
		return false
	}
	for _, group := range account.Groups {
		if group != nil && group.QuotaBypassEnabled && group.QuotaBypassConcentratedSchedulingEnabled {
			return true
		}
	}
	for _, accountGroup := range account.AccountGroups {
		group := accountGroup.Group
		if group != nil && group.QuotaBypassEnabled && group.QuotaBypassConcentratedSchedulingEnabled {
			return true
		}
	}
	return false
}

// IsAccountQuotaBypassConcentratedForRequestGroup preserves the legacy marker
// semantics while allowing the current request group to opt out explicitly.
// An attached quota-bypass group other than the request group is an account
// marker, so it remains concentrated even when that dedicated group's new
// concentration toggle was never backfilled. The request group itself still
// needs both switches and is handled by the scheduler request flags.
func IsAccountQuotaBypassConcentratedForRequestGroup(account *Account, requestGroupID *int64) bool {
	if account == nil || account.Platform != PlatformOpenAI || !account.IsOAuth() {
		return false
	}
	if IsAccountQuotaBypassConcentrated(account) {
		return true
	}
	for _, group := range account.Groups {
		if isAttachedQuotaBypassMarkerForRequestGroup(group, groupIDForQuotaBypassMarker(group), requestGroupID) {
			return true
		}
	}
	for _, accountGroup := range account.AccountGroups {
		if isAttachedQuotaBypassMarkerForRequestGroup(accountGroup.Group, accountGroup.GroupID, requestGroupID) {
			return true
		}
	}
	return false
}

func groupIDForQuotaBypassMarker(group *Group) int64 {
	if group == nil {
		return 0
	}
	return group.ID
}

func isAttachedQuotaBypassMarkerForRequestGroup(group *Group, groupID int64, requestGroupID *int64) bool {
	if group == nil || !group.QuotaBypassEnabled {
		return false
	}
	if group.QuotaBypassConcentratedSchedulingEnabled || requestGroupID == nil {
		return true
	}
	return groupID > 0 && *requestGroupID != groupID
}

// SetOpenAIQuotaBypassEnabled carries the request-group decision across
// protocol conversion paths where only the selected account is otherwise
// available. The handler refreshes it after every failover selection.
func SetOpenAIQuotaBypassEnabled(c *gin.Context, enabled bool) {
	if c == nil {
		return
	}
	c.Set(openAIQuotaBypassEnabledContextKey, enabled)
	// A failover selection starts a new attempt. Do not let an injection made
	// for the previous account leak into the final account's usage snapshot.
	c.Set(openAIQuotaBypassAppliedContextKey, false)
	c.Set(openAIQuotaBypassInjectPairsContextKey, 0)
	c.Writer.Header().Del(openAIQuotaBypassResponseHeader)
	c.Writer.Header().Del(openAIQuotaBypassPairsResponseHeader)
}

func markOpenAIQuotaBypassApplied(c *gin.Context, pairs int) {
	if c == nil {
		return
	}
	c.Set(openAIQuotaBypassAppliedContextKey, true)
	pairs = clampOpenAIQuotaBypassInjectPairs(pairs)
	c.Set(openAIQuotaBypassInjectPairsContextKey, pairs)
	c.Header(openAIQuotaBypassResponseHeader, "applied")
	c.Header(openAIQuotaBypassPairsResponseHeader, strconv.Itoa(pairs))
}

// OpenAIQuotaBypassUsageSnapshot returns the immutable request metadata that
// should be copied into a usage log before billing is dispatched asynchronously.
func OpenAIQuotaBypassUsageSnapshot(c *gin.Context) (bool, int) {
	if c == nil {
		return false, 0
	}
	appliedValue, exists := c.Get(openAIQuotaBypassAppliedContextKey)
	applied, ok := appliedValue.(bool)
	if !exists || !ok || !applied {
		return false, 0
	}
	pairsValue, _ := c.Get(openAIQuotaBypassInjectPairsContextKey)
	pairs, _ := pairsValue.(int)
	return true, clampOpenAIQuotaBypassInjectPairs(pairs)
}

// InjectOpenAIQuotaBypassForRequest appends the synthetic tool turn.
func InjectOpenAIQuotaBypassForRequest(c *gin.Context, body []byte, _ int) ([]byte, bool) {
	injected, applied := InjectFunctionCallOutputSuffix(body)
	if applied {
		markOpenAIQuotaBypassApplied(c, 1)
	}
	return injected, applied
}

func isOpenAIQuotaBypassEnabledForRequest(c *gin.Context, account *Account) bool {
	// The selected request group may enable injection even when the scheduler's
	// account snapshot does not carry full group metadata.
	if c != nil {
		if value, exists := c.Get(openAIQuotaBypassEnabledContextKey); exists {
			if enabled, ok := value.(bool); ok {
				if enabled {
					return true
				}
			}
		}
	}
	return IsAccountQuotaBypassEligible(account)
}

func applyOpenAIQuotaBypassForRequest(c *gin.Context, account *Account, body []byte, pairs int) []byte {
	if !isOpenAIQuotaBypassEnabledForRequest(c, account) {
		return body
	}
	// Injection is the complete quota-bypass behavior. Never propagate this
	// decision into response handling: upstream 401/403/429 and all other
	// failures must follow the same account-state path as an ordinary request.
	if injected, ok := InjectOpenAIQuotaBypassForRequest(c, body, pairs); ok {
		return injected
	}
	return body
}

func applyOpenAIWSQuotaBypass(payload []byte, hooks *OpenAIWSIngressHooks) []byte {
	if hooks == nil || !hooks.QuotaBypassEnabled {
		return payload
	}
	injected, applied := InjectFunctionCallOutputSuffix(payload)
	if applied {
		if hooks.OnQuotaBypassApplied != nil {
			hooks.OnQuotaBypassApplied()
		}
		return injected
	}
	return payload
}

// configureOpenAIQuotaBypass429Retry converts the manual "recover state and
// retry" workflow into a bounded request-local retry. The final failed attempt
// remains rate-limited so normal account failover still applies.
func (s *OpenAIGatewayService) configureOpenAIQuotaBypass429Retry(
	c *gin.Context,
	account *Account,
	failoverErr *UpstreamFailoverError,
	clearRateLimitBeforeRetry bool,
) *UpstreamFailoverError {
	if failoverErr == nil || failoverErr.StatusCode != http.StatusTooManyRequests ||
		account == nil || account.Platform != PlatformOpenAI || !account.IsOAuth() {
		return failoverErr
	}
	applied, _ := OpenAIQuotaBypassUsageSnapshot(c)
	return s.configureOpenAIQuotaBypass429RetryEnabled(account, failoverErr, applied, clearRateLimitBeforeRetry)
}

func (s *OpenAIGatewayService) configureOpenAIQuotaBypass429RetryEnabled(
	account *Account,
	failoverErr *UpstreamFailoverError,
	enabled bool,
	clearRateLimitBeforeRetry bool,
) *UpstreamFailoverError {
	if !enabled || failoverErr == nil || failoverErr.StatusCode != http.StatusTooManyRequests ||
		account == nil || account.Platform != PlatformOpenAI || !account.IsOAuth() {
		return failoverErr
	}
	failoverErr.RetryableOnSameAccount = true
	failoverErr.SameAccountRetryLimit = openAIQuotaBypassSameAccountRetries
	failoverErr.ClearRateLimitBeforeRetry = clearRateLimitBeforeRetry
	if clearRateLimitBeforeRetry {
		failoverErr.RateLimitObservedBefore = time.Now().UTC()
		if s != nil {
			if rawGeneration, ok := s.openaiAccountRuntimeBlockGeneration.Load(account.ID); ok {
				failoverErr.RuntimeBlockGeneration, _ = rawGeneration.(uint64)
			}
		}
	}
	return failoverErr
}

// PrepareOpenAIQuotaBypassSameAccountRetry clears only the 429 state written by
// the failed attempt, after the handler has confirmed another attempt will run.
func (s *OpenAIGatewayService) PrepareOpenAIQuotaBypassSameAccountRetry(
	ctx context.Context,
	accountID int64,
	failoverErr *UpstreamFailoverError,
) bool {
	if failoverErr == nil || !failoverErr.ClearRateLimitBeforeRetry {
		return true
	}
	if s == nil || accountID <= 0 {
		return false
	}
	mu := s.openAIAccountRuntimeBlockLock(accountID)
	mu.Lock()
	defer mu.Unlock()
	if failoverErr.RuntimeBlockGeneration > 0 {
		currentGeneration, ok := s.openaiAccountRuntimeBlockGeneration.Load(accountID)
		if !ok || currentGeneration != failoverErr.RuntimeBlockGeneration {
			return false
		}
	}
	if s.rateLimitService != nil {
		cleared, err := s.rateLimitService.clearRateLimitForQuotaBypassRetry(ctx, accountID, failoverErr.RateLimitObservedBefore)
		if err != nil {
			slog.Warn("openai_quota_bypass_retry_state_clear_failed", "account_id", accountID, "error", err)
			return false
		}
		if !cleared {
			return false
		}
	}
	s.openaiAccountRuntimeBlockUntil.Delete(accountID)
	s.openaiAccountRuntimeBlockGeneration.Store(accountID, s.openaiAccountRuntimeBlockSequence.Add(1))
	return true
}

// ResolveOpenAIQuotaBypassInjectPairs keeps the usage metadata explicit while
// production injection stays fixed at one matched tool round. Full-window
// stress tests showed that additional rounds do not increase usable quota and
// only add input tokens.
func ResolveOpenAIQuotaBypassInjectPairs(_ *config.Config) int {
	return 1
}

func clampOpenAIQuotaBypassInjectPairs(pairs int) int {
	if pairs < 1 {
		return 1
	}
	if pairs > quotaBypassMaxInjectPairs {
		return quotaBypassMaxInjectPairs
	}
	return pairs
}

// InjectFunctionCallOutputSuffix appends one synthetic function_call +
// function_call_output pair to the Responses API "input" array, which causes
// the upstream to skip its first-stage quota check.
func InjectFunctionCallOutputSuffix(body []byte) ([]byte, bool) {
	return InjectFunctionCallOutputSuffixN(body, 1)
}

// InjectFunctionCallOutputSuffixN is retained for deterministic payload tests.
// Production always passes one; pairs is clamped to the test helper's bounds.
func InjectFunctionCallOutputSuffixN(body []byte, pairs int) ([]byte, bool) {
	// Remote compaction has a strict terminal-item contract: compaction_trigger
	// must remain the final input item. Synthetic tool turns also depend on their
	// function_call_output being the suffix, so the two protocols cannot safely
	// share one request. Keep compact payloads byte-for-byte unchanged.
	if HasCompactionTriggerInInput(body) {
		return body, false
	}
	if pairs < 1 {
		pairs = 1
	}
	if pairs > quotaBypassMaxInjectPairs {
		pairs = quotaBypassMaxInjectPairs
	}
	inputArr := gjson.GetBytes(body, "input")
	if !inputArr.Exists() {
		return body, false
	}
	if inputArr.Type == gjson.String {
		message, err := json.Marshal([]map[string]any{{
			"type": "message",
			"role": "user",
			"content": []map[string]any{{
				"type": "input_text",
				"text": inputArr.String(),
			}},
		}})
		if err != nil {
			return body, false
		}
		body, err = sjson.SetRawBytes(body, "input", message)
		if err != nil {
			return body, false
		}
		inputArr = gjson.GetBytes(body, "input")
	}
	if !inputArr.IsArray() {
		return body, false
	}
	items := inputArr.Array()
	// A real function output already passes the same upstream quota stage, so
	// adding another synthetic call would only perturb an otherwise valid turn.
	if len(items) > 0 && items[len(items)-1].Get("type").String() == "function_call_output" {
		return body, false
	}
	idx := len(items)
	for i := 0; i < pairs; i++ {
		callID, fcID := newQuotaBypassCallIDs()

		call, err := json.Marshal(map[string]any{
			"type":      "function_call",
			"id":        fcID,
			"call_id":   callID,
			"name":      quotaBypassToolName,
			"arguments": quotaBypassCallArguments,
		})
		if err != nil {
			return body, false
		}
		output, err := json.Marshal(map[string]any{
			"type":    "function_call_output",
			"call_id": callID,
			"output":  quotaBypassCallOutput,
		})
		if err != nil {
			return body, false
		}

		body, err = sjson.SetRawBytes(body, "input."+strconv.Itoa(idx), call)
		if err != nil {
			return body, false
		}
		body, err = sjson.SetRawBytes(body, "input."+strconv.Itoa(idx+1), output)
		if err != nil {
			return body, false
		}
		idx += 2
	}
	return body, true
}

// The injected turn has to be indistinguishable from a real Codex tool call.
// The previous payload used the literal constants fc_syn_00 / call_syn_00 /
// "_sys" / "[continue]" on every single request, which is both an obvious
// synthetic marker and a fixed fingerprint shared by every request this proxy
// ever sent. These mirror the shell tool that Codex actually drives.
// quotaBypassMaxInjectPairs bounds the diagnostic multi-pair helper. Production
// injection is fixed at one pair by ResolveOpenAIQuotaBypassInjectPairs.
const quotaBypassMaxInjectPairs = 16

const (
	quotaBypassToolName = "shell"
	// Matches the shell tool's real argument shape: an argv array plus the
	// workdir Codex always passes. `true` is a no-op that any shell accepts, so
	// the call stays coherent if it is ever replayed or inspected.
	quotaBypassCallArguments = `{"command":["bash","-lc","true"],"workdir":"."}`
	quotaBypassCallOutput    = ""
)

// quotaBypassIDFallbackCounter seeds the degraded ID path so concurrent
// requests cannot collide on an identical UnixNano.
var quotaBypassIDFallbackCounter uint64

// newQuotaBypassCallIDs returns a (call_id, function_call id) pair in the
// format the Responses API uses. They are per-request: reusing one constant
// makes every injected turn trivially greppable upstream.
func newQuotaBypassCallIDs() (string, string) {
	return "call_" + randomQuotaBypassHex(16), "fc_" + randomQuotaBypassHex(24)
}

func randomQuotaBypassHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		// Never panic on a request path: fall back to a counter-mixed xorshift
		// when the entropy source is unavailable. These IDs only need to be
		// unique, not unpredictable.
		seed := uint64(time.Now().UnixNano()) ^ atomic.AddUint64(&quotaBypassIDFallbackCounter, 1)
		for i := range buf {
			seed ^= seed << 13
			seed ^= seed >> 7
			seed ^= seed << 17
			buf[i] = byte(seed)
		}
	}
	return hex.EncodeToString(buf)
}
