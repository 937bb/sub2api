package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func requireQuotaBypassSuffix(t *testing.T, body []byte) {
	t.Helper()
	input := gjson.GetBytes(body, "input").Array()
	if len(input) < 2 {
		t.Fatalf("input length = %d, want at least 2", len(input))
	}
	customCall := input[len(input)-2]
	customOutput := input[len(input)-1]
	if got := customCall.Get("type").String(); got != "custom_tool_call" {
		t.Fatalf("penultimate input type = %q, want custom_tool_call", got)
	}
	if got := customOutput.Get("type").String(); got != "custom_tool_call_output" {
		t.Fatalf("last input type = %q, want custom_tool_call_output", got)
	}
	if customCall.Get("call_id").String() != customOutput.Get("call_id").String() {
		t.Fatal("synthetic custom tool call IDs do not match")
	}
	if got := customCall.Get("name").String(); got != quotaBypassCustomToolName {
		t.Fatalf("synthetic custom tool name = %q, want %q", got, quotaBypassCustomToolName)
	}
	if got := customCall.Get("input").String(); got != quotaBypassCustomToolInput {
		t.Fatalf("synthetic custom tool input = %q, want %q", got, quotaBypassCustomToolInput)
	}
	if customCall.Get("id").Exists() {
		t.Fatal("synthetic custom tool call must not include an item id")
	}
	if got := customOutput.Get("output.0.type").String(); got != "input_text" {
		t.Fatalf("synthetic custom tool output type = %q, want input_text", got)
	}
	if got := customOutput.Get("output.0.text").String(); got != quotaBypassCustomToolOutput {
		t.Fatalf("synthetic custom tool output = %q, want %q", got, quotaBypassCustomToolOutput)
	}
}

func requireQuotaBypassPairs(t *testing.T, body []byte, originalItems, pairs int) {
	t.Helper()
	input := gjson.GetBytes(body, "input").Array()
	require.Len(t, input, originalItems+pairs*2)
	seenCallIDs := make(map[string]struct{}, pairs)
	for i := 0; i < pairs; i++ {
		call := input[originalItems+i*2]
		output := input[originalItems+i*2+1]
		require.Equal(t, "custom_tool_call", call.Get("type").String())
		require.Equal(t, "custom_tool_call_output", output.Get("type").String())
		callID := call.Get("call_id").String()
		require.NotEmpty(t, callID)
		require.Equal(t, callID, output.Get("call_id").String())
		_, duplicate := seenCallIDs[callID]
		require.False(t, duplicate, "each injected pair must use a unique call_id")
		seenCallIDs[callID] = struct{}{}
	}
}

func TestConfigureOpenAIQuotaBypass429Retry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	account := &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	svc := &OpenAIGatewayService{}
	regular := svc.configureOpenAIQuotaBypass429Retry(c, account, &UpstreamFailoverError{
		StatusCode: http.StatusTooManyRequests,
	}, true)
	require.False(t, regular.RetryableOnSameAccount)
	require.Zero(t, regular.SameAccountRetryLimit)
	require.False(t, regular.ClearRateLimitBeforeRetry)

	markOpenAIQuotaBypassApplied(c, 1)
	bypass := svc.configureOpenAIQuotaBypass429Retry(c, account, &UpstreamFailoverError{
		StatusCode: http.StatusTooManyRequests,
	}, true)
	require.True(t, bypass.RetryableOnSameAccount)
	require.Equal(t, openAIQuotaBypassSameAccountRetries, bypass.SameAccountRetryLimit)
	require.Equal(t, openAIQuotaBypassSameAccountRetries, bypass.ResolveSameAccountRetryLimit(0))
	require.False(t, bypass.ClearRateLimitBeforeRetry)
	require.True(t, bypass.RateLimitObservedBefore.IsZero())

	concurrent := svc.configureOpenAIQuotaBypass429Retry(c, account, &UpstreamFailoverError{
		StatusCode: http.StatusTooManyRequests,
	}, true)
	require.False(t, concurrent.RetryableOnSameAccount, "only one request may own the account-wide probe")
	require.Zero(t, concurrent.SameAccountRetryLimit)
}

func TestConfigureOpenAIQuotaBypass429RetrySkipsTerminalQuotaSignals(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name    string
		body    string
		headers http.Header
	}{
		{name: "usage limit", body: `{"error":{"type":"usage_limit_reached","message":"The usage limit has been reached","resets_in_seconds":3600}}`},
		{name: "explicit retry after", body: `{"error":{"type":"rate_limit_error","code":"rate_limit_exceeded","message":"slow down"}}`, headers: http.Header{"Retry-After": []string{"5"}}},
		{name: "codex reset window", body: `{"error":{"type":"rate_limit_error","code":"rate_limit_exceeded"}}`, headers: http.Header{
			"X-Codex-Primary-Used-Percent":        []string{"100"},
			"X-Codex-Primary-Reset-After-Seconds": []string{"18000"},
		}},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			markOpenAIQuotaBypassApplied(c, 1)
			account := &Account{ID: int64(100 + i), Platform: PlatformOpenAI, Type: AccountTypeOAuth}
			failoverErr := (&OpenAIGatewayService{}).configureOpenAIQuotaBypass429Retry(c, account, &UpstreamFailoverError{
				StatusCode:      http.StatusTooManyRequests,
				ResponseBody:    []byte(tt.body),
				ResponseHeaders: tt.headers,
			}, true)
			require.False(t, failoverErr.RetryableOnSameAccount)
			require.Zero(t, failoverErr.SameAccountRetryLimit)
			require.False(t, failoverErr.ClearRateLimitBeforeRetry)
		})
	}
}

func TestOpenAIQuotaBypass429ProbeIsAccountWide(t *testing.T) {
	svc := &OpenAIGatewayService{}
	now := time.Now()
	var acquired atomic.Int64
	var wg sync.WaitGroup
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if svc.tryAcquireOpenAIQuotaBypass429Probe(501, now) {
				acquired.Add(1)
			}
		}()
	}
	wg.Wait()
	require.Equal(t, int64(1), acquired.Load())
	require.True(t, svc.tryAcquireOpenAIQuotaBypass429Probe(502, now), "different accounts have independent probes")
	require.True(t, svc.tryAcquireOpenAIQuotaBypass429Probe(501, now.Add(openAIQuotaBypass429ProbeLease)), "expired probe can be reacquired")
}

func TestPrepareOpenAIQuotaBypassSameAccountRetryClearsRuntimeBlock(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := &Account{ID: 43, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	svc.BlockAccountScheduling(account, time.Now().Add(time.Hour), "429")
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
	generation, ok := svc.openaiAccountRuntimeBlockGeneration.Load(account.ID)
	require.True(t, ok)

	ok = svc.PrepareOpenAIQuotaBypassSameAccountRetry(context.Background(), account.ID, &UpstreamFailoverError{
		ClearRateLimitBeforeRetry: true,
		RuntimeBlockGeneration:    generation.(uint64),
	})
	require.True(t, ok)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
}

func TestPrepareOpenAIQuotaBypassSameAccountRetryPreservesNewerRuntimeBlock(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := &Account{ID: 44, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	svc.BlockAccountScheduling(account, time.Now().Add(time.Minute), "429")
	staleGeneration, ok := svc.openaiAccountRuntimeBlockGeneration.Load(account.ID)
	require.True(t, ok)
	svc.BlockAccountScheduling(account, time.Now().Add(time.Hour), "newer_429")

	ok = svc.PrepareOpenAIQuotaBypassSameAccountRetry(context.Background(), account.ID, &UpstreamFailoverError{
		ClearRateLimitBeforeRetry: true,
		RuntimeBlockGeneration:    staleGeneration.(uint64),
	})
	require.False(t, ok)
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
}

func TestInjectFunctionCallOutputSuffix_UsesCodexCustomToolProtocol(t *testing.T) {
	base := []byte(`{"model":"gpt-5.4","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}],"tools":[{"type":"function","name":"shell"},{"type":"function","name":"apply_patch"}]}`)

	first, ok := InjectFunctionCallOutputSuffix(base)
	if !ok {
		t.Fatal("injection did not apply")
	}
	requireQuotaBypassSuffix(t, first)

	items := gjson.GetBytes(first, "input").Array()
	call := items[len(items)-2]

	require.Equal(t, quotaBypassCustomToolName, call.Get("name").String())
	require.Equal(t, quotaBypassCustomToolInput, call.Get("input").String())
	require.False(t, call.Get("id").Exists())
	require.False(t, call.Get("arguments").Exists())

	for _, forbidden := range []string{"fc_syn_00", "call_syn_00", "_sys", "[continue]"} {
		if bytes.Contains(first, []byte(forbidden)) {
			t.Fatalf("payload still contains the synthetic marker %q", forbidden)
		}
	}

	// Two injections of the same request must not reuse identifiers.
	second, ok := InjectFunctionCallOutputSuffix(base)
	if !ok {
		t.Fatal("second injection did not apply")
	}
	firstID := call.Get("call_id").String()
	secondItems := gjson.GetBytes(second, "input").Array()
	secondCall := secondItems[len(secondItems)-2]
	secondID := secondCall.Get("call_id").String()
	if firstID == "" || firstID == secondID {
		t.Fatalf("call_id must be unique per request, got %q twice", firstID)
	}
	require.Equal(t, quotaBypassCustomToolName, secondCall.Get("name").String())
}

func TestInjectFunctionCallOutputSuffixN_AppendsConfiguredPairs(t *testing.T) {
	base := []byte(`{"model":"gpt-5.4","input":[{"type":"message","role":"user","content":"hi"}]}`)

	injected, ok := InjectFunctionCallOutputSuffixN(base, 3)
	require.True(t, ok)
	requireQuotaBypassPairs(t, injected, 1, 3)
}

func TestInjectFunctionCallOutputSuffixN_ClampsPairCount(t *testing.T) {
	base := []byte(`{"model":"gpt-5.4","input":[{"type":"message","role":"user","content":"hi"}]}`)

	minimum, ok := InjectFunctionCallOutputSuffixN(base, 0)
	require.True(t, ok)
	requireQuotaBypassPairs(t, minimum, 1, 1)

	maximum, ok := InjectFunctionCallOutputSuffixN(base, quotaBypassMaxInjectPairs+1)
	require.True(t, ok)
	requireQuotaBypassPairs(t, maximum, 1, quotaBypassMaxInjectPairs)
}

func TestResolveOpenAIQuotaBypassInjectPairs(t *testing.T) {
	require.Equal(t, 1, ResolveOpenAIQuotaBypassInjectPairs(nil))
	require.Equal(t, 1, ResolveOpenAIQuotaBypassInjectPairs(&config.Config{}))
}

func TestInjectOpenAIQuotaBypassForRequest_AlwaysInjectsOnePair(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	body := []byte(`{"model":"gpt-5.4","input":[{"type":"message","role":"user","content":"hi"}]}`)

	injected, ok := InjectOpenAIQuotaBypassForRequest(c, body, quotaBypassMaxInjectPairs)
	require.True(t, ok)
	requireQuotaBypassPairs(t, injected, 1, 1)
	applied, pairs := OpenAIQuotaBypassUsageSnapshot(c)
	require.True(t, applied)
	require.Equal(t, 1, pairs)
}

func TestIsQuotaBypassEligible(t *testing.T) {
	bypassGroup := &Group{ID: 1, QuotaBypassEnabled: true}

	tests := []struct {
		name    string
		account *Account
		group   *Group
		want    bool
	}{
		{
			name: "request group enabled",
			account: &Account{
				Platform:    PlatformOpenAI,
				Type:        AccountTypeOAuth,
				Credentials: map[string]any{"plan_type": "plus"},
			},
			group: bypassGroup,
			want:  true,
		},
		{
			name: "plus account override enabled",
			account: &Account{
				Platform:    PlatformOpenAI,
				Type:        AccountTypeOAuth,
				Credentials: map[string]any{"plan_type": "plus"},
				Extra:       map[string]any{"quota_bypass_enabled": true},
			},
			want: true,
		},
		{
			name: "plus account attached group enabled without request group",
			account: &Account{
				Platform:    PlatformOpenAI,
				Type:        AccountTypeOAuth,
				Credentials: map[string]any{"plan_type": "plus"},
				AccountGroups: []AccountGroup{
					{GroupID: 1, Group: bypassGroup},
				},
			},
			want: true,
		},
		{
			name:    "setup token account is eligible",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeSetupToken},
			group:   bypassGroup,
			want:    true,
		},
		{
			name: "account switch off still inherits enabled group",
			account: &Account{
				Platform: PlatformOpenAI,
				Type:     AccountTypeOAuth,
				Extra:    map[string]any{"quota_bypass_enabled": false},
				Groups:   []*Group{bypassGroup},
			},
			group: bypassGroup,
			want:  true,
		},
		{
			name: "team plan alone does not enable bypass",
			account: &Account{
				Platform:    PlatformOpenAI,
				Type:        AccountTypeOAuth,
				Credentials: map[string]any{"plan_type": "team"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsQuotaBypassEligible(tt.account, tt.group); got != tt.want {
				t.Fatalf("IsQuotaBypassEligible() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAttachedGroupQuotaBypassInjectsDirectRequest(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		AccountGroups: []AccountGroup{
			{GroupID: 42, Group: &Group{ID: 42, QuotaBypassEnabled: true}},
		},
	}
	body := []byte(`{"model":"gpt-5.1","input":[{"type":"message","role":"user","content":"hello"}]}`)

	requireBypass := IsQuotaBypassEligible(account, nil)
	if !requireBypass {
		t.Fatal("attached bypass group must enable direct request injection")
	}
	injected, ok := InjectFunctionCallOutputSuffix(body)
	if !ok {
		t.Fatal("direct request was not injected")
	}
	input := gjson.GetBytes(injected, "input").Array()
	if len(input) != 3 {
		t.Fatalf("injected input length = %d, want 3", len(input))
	}
	if got := input[2].Get("type").String(); got != "custom_tool_call_output" {
		t.Fatalf("last input type = %q, want custom_tool_call_output", got)
	}
}

func TestApplyOpenAIQuotaBypassForRequest_UsesHandlerGroupDecision(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{"quota_bypass_enabled": false},
	}
	body := []byte(`{"model":"gpt-5.1","input":[{"type":"message","role":"user","content":"hello"}]}`)

	SetOpenAIQuotaBypassEnabled(c, true)
	injected := applyOpenAIQuotaBypassForRequest(c, account, body, 1)

	requireQuotaBypassSuffix(t, injected)
	applied, pairs := OpenAIQuotaBypassUsageSnapshot(c)
	require.True(t, applied)
	require.Equal(t, 1, pairs)
	require.Equal(t, "applied", c.Writer.Header().Get(openAIQuotaBypassResponseHeader))
	require.Equal(t, "1", c.Writer.Header().Get(openAIQuotaBypassPairsResponseHeader))

	SetOpenAIQuotaBypassEnabled(c, false)
	applied, pairs = OpenAIQuotaBypassUsageSnapshot(c)
	require.False(t, applied)
	require.Zero(t, pairs)
	require.Empty(t, c.Writer.Header().Get(openAIQuotaBypassResponseHeader))
	require.Empty(t, c.Writer.Header().Get(openAIQuotaBypassPairsResponseHeader))
	notInjected := applyOpenAIQuotaBypassForRequest(c, account, body, 1)
	require.Equal(t, body, notInjected)
}

func TestApplyOpenAIQuotaBypassForRequest_SkipsCompactionTrigger(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{"quota_bypass_enabled": true},
	}
	body := []byte(`{"model":"gpt-5.6-sol","input":[{"type":"message","role":"user","content":"compact"},{"type":"compaction_trigger"}]}`)

	forwarded := applyOpenAIQuotaBypassForRequest(c, account, body, 1)

	require.Equal(t, body, forwarded)
	applied, pairs := OpenAIQuotaBypassUsageSnapshot(c)
	require.False(t, applied)
	require.Zero(t, pairs)
	require.Empty(t, recorder.Header().Get(openAIQuotaBypassResponseHeader))
	require.Empty(t, recorder.Header().Get(openAIQuotaBypassPairsResponseHeader))
}

func TestApplyOpenAIQuotaBypassForRequest_StaleNegativeContextDoesNotHideAccountEligibility(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{"quota_bypass_enabled": false},
		AccountGroups: []AccountGroup{{
			GroupID: 42,
			Group:   &Group{ID: 42, QuotaBypassEnabled: true},
		}},
	}
	body := []byte(`{"model":"gpt-5.1","input":[{"type":"message","role":"user","content":"hello"}]}`)

	SetOpenAIQuotaBypassEnabled(c, false)
	injected := applyOpenAIQuotaBypassForRequest(c, account, body, 1)

	requireQuotaBypassSuffix(t, injected)
}

func TestOpenAIGatewayService_ForwardInjectsQuotaBypassForStringInput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"model":"gpt-5.6-sol","stream":false,"input":"hello"}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"type":"invalid_request_error","message":"stop after capture"}}`)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID:          501,
		Name:        "quota-bypass-string-input",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-account",
		},
		Extra: map[string]any{"quota_bypass_enabled": true},
	}

	result, err := svc.Forward(context.Background(), c, account, body)
	require.Error(t, err)
	require.Nil(t, result)
	require.NotNil(t, upstream.lastReq)
	requireQuotaBypassPairs(t, upstream.lastBody, 1, 1)
	require.Equal(t, "hello", gjson.GetBytes(upstream.lastBody, "input.0.content.0.text").String())
}

func TestInjectFunctionCallOutputSuffix_RealToolOutputAlreadyBypassesQuotaStage(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-sol","input":[{"type":"message","role":"user","content":"run tests"},{"type":"function_call","id":"fc_real","call_id":"call_real","name":"exec_command","arguments":"{}"},{"type":"function_call_output","call_id":"call_real","output":"ok"}]}`)

	injected, ok := InjectFunctionCallOutputSuffix(body)
	if ok {
		t.Fatal("real function_call_output must not receive a redundant synthetic pair")
	}
	if string(injected) != string(body) {
		t.Fatal("real tool turn was changed")
	}
}

func TestInjectFunctionCallOutputSuffix_RealCustomToolOutputAlreadyBypassesQuotaStage(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-sol","input":[{"type":"message","role":"user","content":"run tests"},{"type":"custom_tool_call","call_id":"call_real","name":"exec","input":"echo ok"},{"type":"custom_tool_call_output","call_id":"call_real","output":[{"type":"input_text","text":"ok"}]}]}`)

	injected, ok := InjectFunctionCallOutputSuffix(body)
	require.False(t, ok)
	require.Equal(t, body, injected)
}

func TestApplyOpenAIQuotaBypassForRequest_RealToolOutputIsNativeBypass(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	account := &Account{ID: 45, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{"quota_bypass_enabled": true}}
	body := []byte(`{"model":"gpt-5.6-sol","input":[{"type":"function_call","call_id":"call_real"},{"type":"function_call_output","call_id":"call_real","output":"ok"}]}`)

	forwarded := applyOpenAIQuotaBypassForRequest(c, account, body, 1)

	require.Equal(t, body, forwarded)
	applied, pairs := OpenAIQuotaBypassUsageSnapshot(c)
	require.True(t, applied)
	require.Zero(t, pairs)
	failoverErr := (&OpenAIGatewayService{}).configureOpenAIQuotaBypass429Retry(c, account, &UpstreamFailoverError{StatusCode: http.StatusTooManyRequests}, true)
	require.True(t, failoverErr.RetryableOnSameAccount)
}

func TestInjectFunctionCallOutputSuffix_PreservesBypassForInputlessResponseCreate(t *testing.T) {
	for _, body := range [][]byte{
		[]byte(`{"type":"response.create","model":"gpt-5.6-sol","stream":true}`),
		[]byte(`{"type":"response.create","model":"gpt-5.6-sol","input":null}`),
		[]byte(`{"type":"response.create","model":"gpt-5.6-sol","input":[]}`),
	} {
		injected, ok := InjectFunctionCallOutputSuffix(body)

		require.True(t, ok)
		requireQuotaBypassPairs(t, injected, 0, 1)
	}
}

func TestClassifyOpenAIQuotaBypassRequest(t *testing.T) {
	tests := []struct {
		name string
		body string
		want OpenAIQuotaBypassRequestMode
	}{
		{name: "text", body: `{"input":"hello"}`, want: OpenAIQuotaBypassRequestInjectable},
		{name: "missing input websocket turn", body: `{"type":"response.create"}`, want: OpenAIQuotaBypassRequestInjectable},
		{name: "null input websocket turn", body: `{"type":"response.create","input":null}`, want: OpenAIQuotaBypassRequestInjectable},
		{name: "empty input websocket turn", body: `{"type":"response.create","input":[]}`, want: OpenAIQuotaBypassRequestInjectable},
		{name: "conversation item frame", body: `{"type":"conversation.item.create","item":{"type":"message"}}`, want: OpenAIQuotaBypassRequestUnavailable},
		{name: "session update frame", body: `{"type":"session.update","session":{}}`, want: OpenAIQuotaBypassRequestUnavailable},
		{name: "native tool output", body: `{"input":[{"type":"function_call_output","call_id":"call_real"}]}`, want: OpenAIQuotaBypassRequestNativeToolOutput},
		{name: "native custom tool output", body: `{"input":[{"type":"custom_tool_call_output","call_id":"call_real"}]}`, want: OpenAIQuotaBypassRequestNativeToolOutput},
		{name: "compaction", body: `{"input":[{"type":"compaction_trigger"}]}`, want: OpenAIQuotaBypassRequestUnavailable},
		{name: "invalid input object", body: `{"input":{"type":"message"}}`, want: OpenAIQuotaBypassRequestUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, ClassifyOpenAIQuotaBypassRequest([]byte(tt.body)))
		})
	}
}

func TestWithOpenAIQuotaBypassRequestBody_KeepsSchedulingForWSControlFrame(t *testing.T) {
	controlCtx := WithOpenAIQuotaBypassRequestBody(context.Background(), []byte(`{"type":"conversation.item.create","item":{"type":"message"}}`))
	require.False(t, openAIQuotaBypassRequestUnavailable(controlCtx))

	compactionCtx := WithOpenAIQuotaBypassRequestBody(context.Background(), []byte(`{"type":"response.create","input":[{"type":"compaction_trigger"}]}`))
	require.True(t, openAIQuotaBypassRequestUnavailable(compactionCtx))
}

func TestInjectFunctionCallOutputSuffix_IsIdempotentForSyntheticPair(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-sol","input":[{"type":"message","role":"user","content":"hello"}]}`)

	injected, ok := InjectFunctionCallOutputSuffix(body)
	if !ok {
		t.Fatal("first injection failed")
	}
	repeated, ok := InjectFunctionCallOutputSuffix(injected)
	if ok {
		t.Fatal("synthetic quota bypass suffix must not be appended twice")
	}
	if string(repeated) != string(injected) {
		t.Fatal("idempotent injection changed the request body")
	}
	requireQuotaBypassSuffix(t, repeated)
}

func TestInjectFunctionCallOutputSuffix_SkipsCompactionTrigger(t *testing.T) {
	tests := []struct {
		name string
		body []byte
	}{
		{
			name: "valid final trigger",
			body: []byte(`{"model":"gpt-5.6-sol","input":[{"type":"message","role":"user","content":"compact"},{"type":"compaction_trigger"}]}`),
		},
		{
			name: "malformed non-final trigger remains untouched",
			body: []byte(`{"model":"gpt-5.6-sol","input":[{"type":"compaction_trigger"},{"type":"message","role":"user","content":"compact"}]}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			injected, ok := InjectFunctionCallOutputSuffix(tt.body)
			require.False(t, ok)
			require.Equal(t, tt.body, injected)
		})
	}
}

func TestApplyOpenAIWSQuotaBypass_SkipsCompactionTrigger(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-sol","input":[{"type":"message","role":"user","content":"compact"},{"type":"compaction_trigger"}]}`)
	applied := 0
	hooks := &OpenAIWSIngressHooks{
		QuotaBypassEnabled:   true,
		OnQuotaBypassApplied: func() { applied++ },
	}

	forwarded := applyOpenAIWSQuotaBypass(body, hooks)
	require.Equal(t, body, forwarded)
	require.Zero(t, applied)
}

func TestApplyOpenAIWSQuotaBypass_InjectsConversationBackedTurn(t *testing.T) {
	body := []byte(`{"type":"response.create","model":"gpt-5.6-sol"}`)
	applied := 0
	pairs := 0
	hooks := &OpenAIWSIngressHooks{
		QuotaBypassEnabled:   true,
		OnQuotaBypassApplied: func() { applied++ },
		OnQuotaBypassAppliedWithPairs: func(value int) {
			pairs = value
		},
	}

	forwarded := applyOpenAIWSQuotaBypass(body, hooks)

	require.NotEqual(t, body, forwarded)
	require.Equal(t, 1, applied)
	require.Equal(t, 1, pairs)
	requireQuotaBypassPairs(t, forwarded, 0, 1)
}

func TestApplyOpenAIWSQuotaBypass_SkipsNonResponseFrames(t *testing.T) {
	body := []byte(`{"type":"conversation.item.create","item":{"type":"message","role":"user","content":[{"type":"input_text","text":"run tests"}]}}`)
	hooks := &OpenAIWSIngressHooks{QuotaBypassEnabled: true}

	forwarded := applyOpenAIWSQuotaBypass(body, hooks)

	require.Equal(t, body, forwarded)
}

func TestApplyOpenAIWSQuotaBypass_RealToolOutputReportsNativeBypass(t *testing.T) {
	body := []byte(`{"type":"response.create","input":[{"type":"function_call_output","call_id":"call_real","output":"ok"}]}`)
	applied := 0
	pairs := -1
	hooks := &OpenAIWSIngressHooks{
		QuotaBypassEnabled: true,
		OnQuotaBypassApplied: func() {
			applied++
		},
		OnQuotaBypassAppliedWithPairs: func(value int) {
			pairs = value
		},
	}

	forwarded := applyOpenAIWSQuotaBypass(body, hooks)

	require.Equal(t, body, forwarded)
	require.Equal(t, 1, applied)
	require.Zero(t, pairs)
	require.True(t, openAIQuotaBypassEffectivePayload(forwarded))
}

func TestInjectFunctionCallOutputSuffix_ConvertsStringInput(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-sol","input":"hello"}`)

	injected, ok := InjectFunctionCallOutputSuffix(body)
	if !ok {
		t.Fatal("string input was not converted and injected")
	}
	input := gjson.GetBytes(injected, "input").Array()
	if len(input) != 3 {
		t.Fatalf("injected input length = %d, want 3", len(input))
	}
	if got := input[0].Get("type").String(); got != "message" {
		t.Fatalf("converted input type = %q, want message", got)
	}
	if got := input[0].Get("content.0.text").String(); got != "hello" {
		t.Fatalf("converted input text = %q, want hello", got)
	}
	requireQuotaBypassSuffix(t, injected)
}

func TestIsAccountQuotaBypassEligible(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		want    bool
	}{
		{"nil account", nil, false},
		{"wrong platform", &Account{Platform: "anthropic", Type: AccountTypeOAuth}, false},
		{"wrong type", &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, false},
		{
			"extra flag true",
			&Account{
				Platform: PlatformOpenAI,
				Type:     AccountTypeOAuth,
				Extra:    map[string]any{"quota_bypass_enabled": true},
			},
			true,
		},
		{
			"extra flag false",
			&Account{
				Platform: PlatformOpenAI,
				Type:     AccountTypeOAuth,
				Extra:    map[string]any{"quota_bypass_enabled": false},
			},
			false,
		},
		{
			"personal access token without group metadata",
			&Account{
				Platform: PlatformOpenAI,
				Type:     AccountTypeOAuth,
				Credentials: map[string]any{
					"auth_mode": "personalAccessToken",
				},
			},
			true,
		},
		{
			"legacy personal access token import marker",
			&Account{
				Platform: PlatformOpenAI,
				Type:     AccountTypeOAuth,
				Extra: map[string]any{
					"import_source": "codex_personal_access_token",
				},
			},
			true,
		},
		{
			"via Groups field (DB path)",
			&Account{
				Platform: PlatformOpenAI,
				Type:     AccountTypeOAuth,
				Groups:   []*Group{{ID: 1, QuotaBypassEnabled: true}},
			},
			true,
		},
		{
			"via AccountGroups.Group (cache path)",
			&Account{
				Platform: PlatformOpenAI,
				Type:     AccountTypeOAuth,
				AccountGroups: []AccountGroup{
					{GroupID: 1, Group: &Group{ID: 1, QuotaBypassEnabled: true}},
				},
			},
			true,
		},
		{
			"AccountGroups without Group pointer (old cache)",
			&Account{
				Platform:      PlatformOpenAI,
				Type:          AccountTypeOAuth,
				AccountGroups: []AccountGroup{{GroupID: 1}},
			},
			false,
		},
		{
			"AccountGroups with bypass disabled",
			&Account{
				Platform: PlatformOpenAI,
				Type:     AccountTypeOAuth,
				AccountGroups: []AccountGroup{
					{GroupID: 1, Group: &Group{ID: 1, QuotaBypassEnabled: false}},
				},
			},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsAccountQuotaBypassEligible(tt.account)
			if got != tt.want {
				t.Errorf("IsAccountQuotaBypassEligible() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAttachedQuotaBypassGroupSeparatesInjectionAndConcentration(t *testing.T) {
	markerGroup := &Group{ID: 77, QuotaBypassEnabled: true}
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		AccountGroups: []AccountGroup{{
			GroupID: markerGroup.ID,
			Group:   markerGroup,
		}},
	}

	require.True(t, IsAccountQuotaBypassEligible(account), "quota bypass remains active while concentration is disabled")
	require.False(t, IsAccountQuotaBypassConcentrated(account), "disabled concentration must use ordinary load balancing")

	markerGroup.QuotaBypassConcentratedSchedulingEnabled = true
	require.True(t, IsAccountQuotaBypassEligible(account))
	require.True(t, IsAccountQuotaBypassConcentrated(account), "enabled concentration must use fill-first scheduling")
}
