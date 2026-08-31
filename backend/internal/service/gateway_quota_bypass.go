package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
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
	openAIQuotaBypassSameAccountRetries    = 1
	openAIQuotaBypass429ProbeLease         = 5 * time.Second
)

type openAIQuotaBypassRequestContextKey struct{}

type OpenAIQuotaBypassRequestMode uint8

const (
	OpenAIQuotaBypassRequestUnavailable OpenAIQuotaBypassRequestMode = iota
	OpenAIQuotaBypassRequestInjectable
	OpenAIQuotaBypassRequestNativeToolOutput
)

// ClassifyOpenAIQuotaBypassRequest is the single request-shape decision shared
// by scheduling and forwarding. A genuine tool output already reaches the same
// upstream quota stage without an extra synthetic pair. Compaction requests
// cannot be modified because compaction_trigger must remain the final item.
func ClassifyOpenAIQuotaBypassRequest(body []byte) OpenAIQuotaBypassRequestMode {
	if !gjson.ValidBytes(body) || HasCompactionTriggerInInput(body) {
		return OpenAIQuotaBypassRequestUnavailable
	}
	// Responses WebSocket control and conversation-item frames are not create
	// requests and must remain byte-for-byte unchanged. HTTP request bodies do
	// not carry a top-level event type, so an empty type remains eligible.
	if eventType := strings.TrimSpace(gjson.GetBytes(body, "type").String()); eventType != "" && eventType != "response.create" {
		return OpenAIQuotaBypassRequestUnavailable
	}
	input := gjson.GetBytes(body, "input")
	if !input.Exists() || input.Type == gjson.Null {
		return OpenAIQuotaBypassRequestInjectable
	}
	if input.Type == gjson.String {
		return OpenAIQuotaBypassRequestInjectable
	}
	if !input.IsArray() {
		return OpenAIQuotaBypassRequestUnavailable
	}
	items := input.Array()
	if len(items) > 0 && isOpenAIQuotaBypassToolOutputType(items[len(items)-1].Get("type").String()) {
		return OpenAIQuotaBypassRequestNativeToolOutput
	}
	return OpenAIQuotaBypassRequestInjectable
}

// WithOpenAIQuotaBypassRequestBody carries request capability into account
// selection, which runs before quota-bypass mutation. Missing context remains
// fail-open for adapters that construct Responses input after account selection.
func WithOpenAIQuotaBypassRequestBody(ctx context.Context, body []byte) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	mode := ClassifyOpenAIQuotaBypassRequest(body)
	eventType := strings.TrimSpace(gjson.GetBytes(body, "type").String())
	// A non-create WS frame cannot be injected, but it also does not determine
	// the shape of the later response.create on the same connection. Keep
	// bypass-aware account selection enabled until the generating frame arrives.
	unavailable := mode == OpenAIQuotaBypassRequestUnavailable && (eventType == "" || eventType == "response.create")
	return context.WithValue(ctx, openAIQuotaBypassRequestContextKey{}, unavailable)
}

func openAIQuotaBypassRequestUnavailable(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	unavailable, _ := ctx.Value(openAIQuotaBypassRequestContextKey{}).(bool)
	return unavailable
}

// IsQuotaBypassEligible reports whether an account qualifies for Codex quota
// bypass injection through an account override, the request group, or any group
// attached to the scheduled account. Personal access token accounts are
// implicitly eligible because they are imported as OAuth accounts and do not
// carry the optional group metadata used by the web importer. API key accounts
// remain excluded. Eligibility is intentionally independent of plan_type, so
// Plus, Team, Pro and other OpenAI OAuth plans behave alike.
// Eligibility is additive: an account-level true value or any enabled group is
// sufficient. A stored false value means only that the account-level switch is
// off; it must not disable bypass inherited from a request or attached group.
func IsQuotaBypassEligible(account *Account, group *Group) bool {
	if account == nil || account.Platform != PlatformOpenAI || !account.IsOAuth() {
		return false
	}
	if isOpenAIPersonalAccessToken(account) {
		return true
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

// isOpenAIPersonalAccessToken recognizes the credential markers emitted by
// the Codex PAT importer. Older imports used the camel-case auth_mode value,
// while newer imports also persist the normalized openai_auth_mode and source
// marker. Keep all forms here so old accounts are upgraded without a rewrite.
func isOpenAIPersonalAccessToken(account *Account) bool {
	if account == nil || account.Platform != PlatformOpenAI || !account.IsOAuth() {
		return false
	}
	if account.IsOpenAIPersonalAccessToken() {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(account.getExtraString("import_source")), "codex_personal_access_token")
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
	if pairs > 0 {
		pairs = clampOpenAIQuotaBypassInjectPairs(pairs)
	} else {
		pairs = 0
	}
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
	if pairs <= 0 {
		return true, 0
	}
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
	if c != nil && c.Request != nil && openAIQuotaBypassRequestUnavailable(c.Request.Context()) {
		return body
	}
	switch ClassifyOpenAIQuotaBypassRequest(body) {
	case OpenAIQuotaBypassRequestUnavailable:
		return body
	case OpenAIQuotaBypassRequestNativeToolOutput:
		// No mutation is needed, but this attempt is quota-bypass capable and
		// must use the same bounded 429 handling and usage classification.
		if applied, _ := OpenAIQuotaBypassUsageSnapshot(c); !applied {
			markOpenAIQuotaBypassApplied(c, 0)
		}
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
	switch ClassifyOpenAIQuotaBypassRequest(payload) {
	case OpenAIQuotaBypassRequestUnavailable:
		return payload
	case OpenAIQuotaBypassRequestNativeToolOutput:
		if hooks.OnQuotaBypassApplied != nil {
			hooks.OnQuotaBypassApplied()
		}
		if hooks.OnQuotaBypassAppliedWithPairs != nil {
			hooks.OnQuotaBypassAppliedWithPairs(0)
		}
		return payload
	}
	injected, applied := InjectFunctionCallOutputSuffix(payload)
	if applied {
		if hooks.OnQuotaBypassApplied != nil {
			hooks.OnQuotaBypassApplied()
		}
		if hooks.OnQuotaBypassAppliedWithPairs != nil {
			hooks.OnQuotaBypassAppliedWithPairs(1)
		}
		return injected
	}
	return payload
}

func openAIQuotaBypassEffectivePayload(payload []byte) bool {
	return ClassifyOpenAIQuotaBypassRequest(payload) == OpenAIQuotaBypassRequestNativeToolOutput
}

// configureOpenAIQuotaBypass429Retry gives only unknown/transient 429s one
// account-wide half-open probe. Quota-exhausted responses are never retried on
// the same account, and the shared rate-limit state is never cleared here.
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
	_ bool,
) *UpstreamFailoverError {
	if !enabled || failoverErr == nil || failoverErr.StatusCode != http.StatusTooManyRequests ||
		account == nil || account.Platform != PlatformOpenAI || !account.IsOAuth() {
		return failoverErr
	}
	// A quota/reset signal is deterministic for this account until its stated
	// boundary. Retrying it in-place only repeats the same 429 and amplifies load.
	if isOpenAIQuotaBypassTerminal429(failoverErr) {
		failoverErr.RetryableOnSameAccount = false
		failoverErr.SameAccountRetryLimit = 0
		failoverErr.ClearRateLimitBeforeRetry = false
		return failoverErr
	}

	// Unknown/transient 429s get one account-wide half-open probe. Concurrent
	// requests observe the lease and skip the probe, preventing every request
	// from clearing shared state and retrying the same cooling account.
	if s == nil || !s.tryAcquireOpenAIQuotaBypass429Probe(account.ID, time.Now()) {
		failoverErr.RetryableOnSameAccount = false
		failoverErr.SameAccountRetryLimit = 0
		failoverErr.ClearRateLimitBeforeRetry = false
		return failoverErr
	}
	failoverErr.RetryableOnSameAccount = true
	failoverErr.SameAccountRetryLimit = openAIQuotaBypassSameAccountRetries
	// Keep the account's persisted/runtime 429 block in place while the current
	// request performs its direct retry. Reopening scheduling here caused the
	// account to flap back into the pool and created a thundering herd.
	failoverErr.ClearRateLimitBeforeRetry = false
	return failoverErr
}

func isOpenAIQuotaBypassTerminal429(failoverErr *UpstreamFailoverError) bool {
	if failoverErr == nil || failoverErr.StatusCode != http.StatusTooManyRequests {
		return false
	}
	body := failoverErr.ResponseBody
	for _, path := range []string{
		"error.type", "error.code", "response.error.type", "response.error.code",
	} {
		switch strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, path).String())) {
		case "usage_limit_reached", "quota_exhausted":
			return true
		}
	}
	if parseOpenAIRateLimitResetTime(body) != nil || calculateOpenAI429ResetTime(failoverErr.ResponseHeaders) != nil {
		return true
	}
	if failoverErr.ResponseHeaders != nil && strings.TrimSpace(failoverErr.ResponseHeaders.Get("Retry-After")) != "" {
		return true
	}
	message := strings.ToLower(strings.TrimSpace(extractUpstreamErrorMessage(body)))
	for _, marker := range []string{"usage limit", "quota exhausted", "limit has been reached"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func (s *OpenAIGatewayService) tryAcquireOpenAIQuotaBypass429Probe(accountID int64, now time.Time) bool {
	if s == nil || accountID <= 0 {
		return false
	}
	leaseUntil := now.Add(openAIQuotaBypass429ProbeLease)
	for {
		current, loaded := s.openaiQuotaBypass429ProbeUntil.LoadOrStore(accountID, leaseUntil)
		if !loaded {
			return true
		}
		currentUntil, ok := current.(time.Time)
		if ok && now.Before(currentUntil) {
			return false
		}
		if s.openaiQuotaBypass429ProbeUntil.CompareAndSwap(accountID, current, leaseUntil) {
			return true
		}
	}
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

// InjectFunctionCallOutputSuffix is the legacy public entry point for appending
// one synthetic Codex custom-tool history pair. The custom protocol is required
// by the ChatGPT Codex upstream quota stage; ordinary function history is still
// rejected with usage_limit_reached after the account's reported quota is full.
func InjectFunctionCallOutputSuffix(body []byte) ([]byte, bool) {
	return InjectFunctionCallOutputSuffixN(body, 1)
}

// InjectFunctionCallOutputSuffixN is retained for deterministic payload tests.
// Production always passes one; pairs is clamped to the test helper's bounds.
func InjectFunctionCallOutputSuffixN(body []byte, pairs int) ([]byte, bool) {
	// Remote compaction has a strict terminal-item contract: compaction_trigger
	// must remain the final input item. Synthetic tool turns also depend on their
	// custom_tool_call_output being the suffix, so the two protocols cannot safely
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
	if !inputArr.Exists() || inputArr.Type == gjson.Null {
		var err error
		body, err = sjson.SetRawBytes(body, "input", []byte("[]"))
		if err != nil {
			return body, false
		}
		inputArr = gjson.GetBytes(body, "input")
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
	// A real function or custom-tool output already passes the same upstream
	// quota stage, so adding another synthetic call would only perturb a valid
	// continuation and can interfere with the client's next tool invocation.
	if len(items) > 0 && isOpenAIQuotaBypassToolOutputType(items[len(items)-1].Get("type").String()) {
		return body, false
	}
	idx := len(items)
	for i := 0; i < pairs; i++ {
		callID := newQuotaBypassCallID()

		call, err := json.Marshal(map[string]any{
			"type":    "custom_tool_call",
			"call_id": callID,
			"name":    quotaBypassCustomToolName,
			"input":   quotaBypassCustomToolInput,
		})
		if err != nil {
			return body, false
		}
		output, err := json.Marshal(map[string]any{
			"type":    "custom_tool_call_output",
			"call_id": callID,
			"output": []map[string]string{{
				"type": "input_text",
				"text": quotaBypassCustomToolOutput,
			}},
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

// quotaBypassMaxInjectPairs bounds the diagnostic multi-pair helper. Production
// injection is fixed at one pair by ResolveOpenAIQuotaBypassInjectPairs.
const quotaBypassMaxInjectPairs = 16

const (
	quotaBypassCustomToolName   = "exec"
	quotaBypassCustomToolInput  = `const r = await tools.exec_command({"cmd":"true","yield_time_ms":1000,"max_output_tokens":1000}); text(r.output);`
	quotaBypassCustomToolOutput = "Script completed\nWall time 0.0 seconds\nOutput:\n"
)

func isOpenAIQuotaBypassToolOutputType(itemType string) bool {
	switch strings.TrimSpace(itemType) {
	case "function_call_output", "custom_tool_call_output":
		return true
	default:
		return false
	}
}

// quotaBypassIDFallbackCounter seeds the degraded ID path so concurrent
// requests cannot collide on an identical UnixNano.
var quotaBypassIDFallbackCounter uint64

// newQuotaBypassCallID returns a per-request identifier in the format accepted
// by both HTTP and WebSocket Codex Responses transports.
func newQuotaBypassCallID() string {
	return "call_" + randomQuotaBypassHex(16)
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
