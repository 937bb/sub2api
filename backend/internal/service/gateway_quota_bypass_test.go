package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
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

func TestIsQuotaBypassEligible(t *testing.T) {
	bypassGroup := &Group{ID: 1, QuotaBypassEnabled: true}

	tests := []struct {
		name    string
		account *Account
		group   *Group
		want    bool
	}{
		{
			name:    "request group enabled",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth},
			group:   bypassGroup,
			want:    true,
		},
		{
			name: "attached group enabled without request group",
			account: &Account{
				Platform: PlatformOpenAI,
				Type:     AccountTypeOAuth,
				AccountGroups: []AccountGroup{
					{GroupID: 1, Group: bypassGroup},
				},
			},
			want: true,
		},
		{
			name: "explicit account disable overrides groups",
			account: &Account{
				Platform: PlatformOpenAI,
				Type:     AccountTypeOAuth,
				Extra:    map[string]any{"quota_bypass_enabled": false},
				Groups:   []*Group{bypassGroup},
			},
			group: bypassGroup,
			want:  false,
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
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	body := []byte(`{"model":"gpt-5.1","input":[{"type":"message","role":"user","content":"hello"}]}`)

	SetOpenAIQuotaBypassEnabled(c, true)
	injected := applyOpenAIQuotaBypassForRequest(c, account, body)

	requireQuotaBypassSuffix(t, injected)
}

func TestSetOpenAIQuotaBypassEnabled_PropagatesToRequestContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	SetOpenAIQuotaBypassEnabled(c, true)

	enabled, exists := openAIQuotaBypassEnabledFromContext(c.Request.Context())
	if !exists || !enabled {
		t.Fatalf("request context quota bypass = (%v, %v), want (true, true)", enabled, exists)
	}
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
