package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

type OpenAIOAuthStartupConfigValidation struct{}

var openAIOAuthForbiddenExtraKeys = map[string]string{
	"openai_oauth_passthrough":                      "delete legacy OAuth passthrough key; if OAuth WS is needed set openai_oauth_ws_mode=managed_session|off",
	"openai_passthrough":                            "delete APIKey-only passthrough key before saving as OAuth; if OAuth WS is needed set openai_oauth_ws_mode=managed_session|off",
	"openai_oauth_responses_websockets_v2_mode":     "migrate to openai_oauth_ws_mode=managed_session|off and delete legacy mode key",
	"openai_oauth_responses_websockets_v2_enabled":  "migrate to openai_oauth_ws_mode=managed_session|off and delete legacy enabled key",
	"responses_websockets_v2_enabled":               "migrate generic legacy WS key to openai_oauth_ws_mode=managed_session|off before saving as OAuth",
	"openai_ws_enabled":                             "migrate generic legacy WS key to openai_oauth_ws_mode=managed_session|off before saving as OAuth",
	"openai_apikey_responses_websockets_v2_enabled": "delete APIKey-only WS key before saving as OAuth",
	"openai_apikey_responses_websockets_v2_mode":    "delete APIKey-only WS key before saving as OAuth",
}

// ProvideOpenAIOAuthStartupConfigValidation runs the migration follow-up guard
// during application startup, after repository migrations and config validation.
func ProvideOpenAIOAuthStartupConfigValidation(accountRepo AccountRepository, cfg *config.Config) (*OpenAIOAuthStartupConfigValidation, error) {
	accounts, err := accountRepo.ListByPlatformForValidation(context.Background(), PlatformOpenAI)
	if err != nil {
		return nil, fmt.Errorf("list OpenAI accounts for OAuth config validation: %w", err)
	}
	if err := ValidateOpenAIOAuthAccountsStartupConfig(accounts, cfg); err != nil {
		return nil, err
	}
	return &OpenAIOAuthStartupConfigValidation{}, nil
}

// ValidateOpenAIOAuthAccountsStartupConfig rejects legacy OAuth passthrough/WS
// config after migrations have run. It reports only key classes/actions, never
// user-provided values from account Extra.
func ValidateOpenAIOAuthAccountsStartupConfig(accounts []Account, cfg *config.Config) error {
	var violations []string
	for i := range accounts {
		violations = append(violations, validateOpenAIOAuthAccountConfig(&accounts[i], "startup")...)
	}

	if cfg != nil && normalizeOpenAIWSIngressDefaultMode(cfg.Gateway.OpenAIWS.IngressModeDefault) == OpenAIWSIngressModePassthrough {
		probe := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
		if mode := probe.ResolveOpenAIResponsesWebSocketV2Mode(cfg.Gateway.OpenAIWS.IngressModeDefault); mode == OpenAIWSIngressModePassthrough {
			violations = append(violations, "global key=gateway.openai_ws.ingress_mode_default action=fix_resolver_isolation recommendation=OAuth must resolve passthrough default to off unless openai_oauth_ws_mode=managed_session")
		}
	}

	if len(violations) > 0 {
		return newOpenAIOAuthConfigValidationError(violations)
	}
	return nil
}

func validateOpenAIOAuthAccountWriteConfig(account *Account) error {
	violations := validateOpenAIOAuthAccountConfig(account, "write")
	if len(violations) > 0 {
		return newOpenAIOAuthConfigValidationError(violations)
	}
	return nil
}

func validateOpenAIOAuthAccountConfig(account *Account, scope string) []string {
	if account == nil || !account.IsOpenAIOAuthLike() || account.Extra == nil {
		return nil
	}

	keys := make([]string, 0, len(openAIOAuthForbiddenExtraKeys))
	for key := range openAIOAuthForbiddenExtraKeys {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	violations := make([]string, 0)
	for _, key := range keys {
		if _, ok := account.Extra[key]; ok {
			violations = append(violations, formatOpenAIOAuthConfigViolation(account.ID, scope, key, openAIOAuthForbiddenExtraKeys[key]))
		}
	}

	if raw, ok := account.Extra["openai_oauth_ws_mode"]; ok {
		mode, ok := raw.(string)
		if !ok || normalizeOpenAIOAuthWSMode(mode) == "" {
			violations = append(violations, formatOpenAIOAuthConfigViolation(account.ID, scope, "openai_oauth_ws_mode", "set openai_oauth_ws_mode=managed_session|off"))
		}
	}

	return violations
}

func newOpenAIOAuthConfigValidationError(violations []string) error {
	message := "OpenAI OAuth config validation failed: " + strings.Join(violations, "; ")
	return infraerrors.BadRequest("OPENAI_OAUTH_CONFIG_INVALID", message)
}

func formatOpenAIOAuthConfigViolation(accountID int64, scope, key, recommendation string) string {
	return fmt.Sprintf("scope=%s account_id=%d key=%s action=reject recommendation=%s", scope, accountID, key, recommendation)
}

func mergeAccountExtraForValidation(existing, updates map[string]any) map[string]any {
	return mergeAccountExtraWithDeletesForValidation(existing, updates, nil)
}

// mergeAccountExtraWithDeletesForValidation mirrors the temporary bulk write order:
// delete legacy extra keys first, then merge updates. Remove with ExtraDeleteKeys.
func mergeAccountExtraWithDeletesForValidation(existing, updates map[string]any, deleteKeys []string) map[string]any {
	if len(existing) == 0 && len(updates) == 0 {
		return nil
	}
	merged := make(map[string]any, len(existing)+len(updates))
	for key, value := range existing {
		merged[key] = value
	}
	for _, key := range NormalizeExtraDeleteKeys(deleteKeys) {
		delete(merged, key)
	}
	for key, value := range updates {
		merged[key] = value
	}
	return merged
}

func NormalizeExtraDeleteKeys(keys []string) []string {
	if len(keys) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(keys))
	normalized := make([]string, 0, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, key)
	}
	return normalized
}

func NormalizeOpenAIOAuthExtraDeleteKeys(keys []string) ([]string, error) {
	normalized := NormalizeExtraDeleteKeys(keys)
	for _, key := range normalized {
		if _, ok := openAIOAuthForbiddenExtraKeys[key]; !ok {
			return nil, infraerrors.BadRequest("OPENAI_OAUTH_EXTRA_DELETE_KEYS_INVALID", "extra_delete_keys only supports legacy OpenAI OAuth passthrough/WS cleanup keys")
		}
	}
	return normalized, nil
}
