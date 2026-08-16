package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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
	functionCall := input[len(input)-2]
	functionOutput := input[len(input)-1]
	if got := functionCall.Get("type").String(); got != "function_call" {
		t.Fatalf("penultimate input type = %q, want function_call", got)
	}
	if got := functionOutput.Get("type").String(); got != "function_call_output" {
		t.Fatalf("last input type = %q, want function_call_output", got)
	}
	if functionCall.Get("call_id").String() != functionOutput.Get("call_id").String() {
		t.Fatal("synthetic function call IDs do not match")
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
		require.Equal(t, "function_call", call.Get("type").String())
		require.Equal(t, "function_call_output", output.Get("type").String())
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
	require.True(t, bypass.ClearRateLimitBeforeRetry)
	require.False(t, bypass.RateLimitObservedBefore.IsZero())
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

// The injected turn must look like a real Codex shell call, not a constant.
// A fixed call_id shared by every request this proxy sends is a trivial
// upstream fingerprint, and "_sys"/"[continue]" advertise the turn as synthetic.
func TestInjectFunctionCallOutputSuffix_LooksLikeRealToolCall(t *testing.T) {
	base := []byte(`{"model":"gpt-5.4","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}]}`)

	first, ok := InjectFunctionCallOutputSuffix(base)
	if !ok {
		t.Fatal("injection did not apply")
	}
	requireQuotaBypassSuffix(t, first)

	items := gjson.GetBytes(first, "input").Array()
	call := items[len(items)-2]

	if got := call.Get("name").String(); got != "shell" {
		t.Fatalf("tool name = %q, want shell", got)
	}
	if !gjson.Valid(call.Get("arguments").String()) {
		t.Fatalf("arguments must be a JSON document, got %q", call.Get("arguments").String())
	}
	if cmd := gjson.Get(call.Get("arguments").String(), "command").Array(); len(cmd) == 0 {
		t.Fatal("shell arguments must carry a command argv")
	}

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
	secondID := secondItems[len(secondItems)-2].Get("call_id").String()
	if firstID == "" || firstID == secondID {
		t.Fatalf("call_id must be unique per request, got %q twice", firstID)
	}
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
	if got := input[2].Get("type").String(); got != "function_call_output" {
		t.Fatalf("last input type = %q, want function_call_output", got)
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
