package service

import "testing"

func TestResolveOpenAIForwardModel(t *testing.T) {
	account := &Account{Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5": "gpt-5.4"}}}
	if got := resolveOpenAIForwardModel(account, "gpt-5"); got != "gpt-5.4" {
		t.Fatalf("resolveOpenAIForwardModel(...) = %q, want %q", got, "gpt-5.4")
	}
	if got := resolveOpenAIForwardModel(nil, "claude-opus-4-6"); got != "claude-opus-4-6" {
		t.Fatalf("resolveOpenAIForwardModel(...) = %q, want original model", got)
	}
}

func TestResolveOpenAIMessagesForwardModel(t *testing.T) {
	tests := []struct {
		name      string
		account   *Account
		requested string
		dispatch  string
		want      string
	}{
		{name: "unknown family exact dispatch", account: &Account{Credentials: map[string]any{}}, requested: "claude-fable-5", dispatch: "gpt-5.6-sol", want: "gpt-5.6-sol"},
		{name: "trims dispatch", account: &Account{Credentials: map[string]any{}}, requested: "claude-fable-5", dispatch: "  gpt-5.6-sol  ", want: "gpt-5.6-sol"},
		{name: "empty dispatch uses requested", account: &Account{Credentials: map[string]any{}}, requested: "claude-fable-5", dispatch: " \t ", want: "claude-fable-5"},
		{name: "nil account uses dispatch", requested: "claude-fable-5", dispatch: "gpt-5.6-sol", want: "gpt-5.6-sol"},
		{name: "exact account mapping wins", account: &Account{Credentials: map[string]any{"model_mapping": map[string]any{"claude-fable-5": "gpt-5.5"}}}, requested: "claude-fable-5", dispatch: "gpt-5.6-sol", want: "gpt-5.5"},
		{name: "wildcard account mapping wins", account: &Account{Credentials: map[string]any{"model_mapping": map[string]any{"claude-*": "gpt-5.4"}}}, requested: "claude-fable-5", dispatch: "gpt-5.6-sol", want: "gpt-5.4"},
		{name: "passthrough account mapping wins", account: &Account{Credentials: map[string]any{"model_mapping": map[string]any{"claude-fable-5": "claude-fable-5"}}}, requested: "claude-fable-5", dispatch: "gpt-5.6-sol", want: "claude-fable-5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveOpenAIMessagesForwardModel(tt.account, tt.requested, tt.dispatch); got != tt.want {
				t.Fatalf("resolveOpenAIMessagesForwardModel(...) = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveOpenAICompactForwardModel(t *testing.T) {
	tests := []struct {
		name          string
		account       *Account
		model         string
		expectedModel string
	}{
		{
			name:          "nil account keeps original model",
			account:       nil,
			model:         "gpt-5.4",
			expectedModel: "gpt-5.4",
		},
		{
			name: "missing compact mapping keeps original model",
			account: &Account{
				Credentials: map[string]any{},
			},
			model:         "gpt-5.4",
			expectedModel: "gpt-5.4",
		},
		{
			name: "exact compact mapping overrides model",
			account: &Account{
				Credentials: map[string]any{
					"compact_model_mapping": map[string]any{
						"gpt-5.4": "gpt-5.4-openai-compact",
					},
				},
			},
			model:         "gpt-5.4",
			expectedModel: "gpt-5.4-openai-compact",
		},
		{
			name: "wildcard compact mapping overrides model",
			account: &Account{
				Credentials: map[string]any{
					"compact_model_mapping": map[string]any{
						"gpt-5.*": "gpt-5-openai-compact",
					},
				},
			},
			model:         "gpt-5.4",
			expectedModel: "gpt-5-openai-compact",
		},
		{
			name: "passthrough compact mapping remains unchanged",
			account: &Account{
				Credentials: map[string]any{
					"compact_model_mapping": map[string]any{
						"gpt-5.4": "gpt-5.4",
					},
				},
			},
			model:         "gpt-5.4",
			expectedModel: "gpt-5.4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveOpenAICompactForwardModel(tt.account, tt.model); got != tt.expectedModel {
				t.Fatalf("resolveOpenAICompactForwardModel(...) = %q, want %q", got, tt.expectedModel)
			}
		})
	}
}

func TestNormalizeCodexModel(t *testing.T) {
	cases := map[string]string{
		"gpt-5.3-codex-spark":       "gpt-5.3-codex-spark",
		"gpt-5.3-codex-spark-high":  "gpt-5.3-codex-spark",
		"gpt-5.3-codex-spark-xhigh": "gpt-5.3-codex-spark",
		"gpt-5.3":                   "gpt-5.3-codex",
		"gpt-image-2":               "gpt-image-2",
		"gpt-5.4-nano":              "gpt-5.4-nano",
		"gpt-5.4-nano-high":         "gpt-5.4-nano",
		"gpt6":                      "gpt6",
		"claude-opus-4-6":           "claude-opus-4-6",
	}

	for input, expected := range cases {
		if got := normalizeCodexModel(input); got != expected {
			t.Fatalf("normalizeCodexModel(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestNormalizeOpenAIModelForUpstream(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		model   string
		want    string
	}{
		{
			name:    "oauth routes bare GPT-5.6 alias to Sol",
			account: &Account{Type: AccountTypeOAuth},
			model:   "gpt-5.6",
			want:    "gpt-5.6-sol",
		},
		{
			name:    "oauth routes provider-prefixed GPT-5.6 alias to Sol",
			account: &Account{Type: AccountTypeOAuth},
			model:   "openai/gpt-5.6",
			want:    "gpt-5.6-sol",
		},
		{
			name:    "oauth preserves unknown non codex model",
			account: &Account{Type: AccountTypeOAuth},
			model:   "gemini-3-flash-preview",
			want:    "gemini-3-flash-preview",
		},
		{
			name:    "oauth preserves invalid gpt model",
			account: &Account{Type: AccountTypeOAuth},
			model:   "gpt6",
			want:    "gpt6",
		},
		{
			name:    "oauth normalizes known codex alias",
			account: &Account{Type: AccountTypeOAuth},
			model:   "gpt-5.4-high",
			want:    "gpt-5.4",
		},
		{
			name:    "setup-token normalizes known codex alias",
			account: &Account{Type: AccountTypeSetupToken},
			model:   "gpt-5.4-high",
			want:    "gpt-5.4",
		},
		{
			name:    "oauth preserves codex auto review model",
			account: &Account{Type: AccountTypeOAuth},
			model:   "codex-auto-review",
			want:    "codex-auto-review",
		},
		{
			name:    "apikey preserves official bare GPT-5.6 alias",
			account: &Account{Type: AccountTypeAPIKey},
			model:   "gpt-5.6",
			want:    "gpt-5.6",
		},
		{
			name:    "apikey preserves custom compatible model",
			account: &Account{Type: AccountTypeAPIKey},
			model:   "gemini-3-flash-preview",
			want:    "gemini-3-flash-preview",
		},
		{
			name:    "apikey preserves official non codex model",
			account: &Account{Type: AccountTypeAPIKey},
			model:   "gpt-4.1",
			want:    "gpt-4.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeOpenAIModelForUpstream(tt.account, tt.model); got != tt.want {
				t.Fatalf("normalizeOpenAIModelForUpstream(...) = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUsageBillingModelCandidatesPreserveCodexAutoReviewModel(t *testing.T) {
	candidates := usageBillingModelCandidates("codex-auto-review")

	expected := []string{"codex-auto-review"}
	if len(candidates) != len(expected) {
		t.Fatalf("usageBillingModelCandidates(codex-auto-review) = %#v, want %#v", candidates, expected)
	}
	for i := range expected {
		if candidates[i] != expected[i] {
			t.Fatalf("usageBillingModelCandidates(codex-auto-review) = %#v, want %#v", candidates, expected)
		}
	}
}
