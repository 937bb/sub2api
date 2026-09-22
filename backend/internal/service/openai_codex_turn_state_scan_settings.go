package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const SettingKeyOpenAICodexTurnStateScanSettings = "openai_codex_turn_state_scan_settings"

const defaultOpenAICodexTurnStateDynamicProxyURL = "https://api.cliproxy.io/white/api?region=Rand&num=1&format=n&type=txt"

const (
	openAICodexTurnStateMinLength = 64
	openAICodexTurnStateMaxLength = 4096

	OpenAICodexTurnStateScanRouteAuto         = "auto"
	OpenAICodexTurnStateScanRouteManagedProxy = "managed_proxy"
	OpenAICodexTurnStateScanRouteIPv6         = "ipv6"
	OpenAICodexTurnStateScanRouteDynamicProxy = "dynamic_proxy"
)

// OpenAICodexTurnStateScanSettings configures state acquisition and the routing
// guard for newly imported OAuth credentials. TargetLengths is an ordered
// allowlist; values outside the resolved plan/model policy are never reusable.
type OpenAICodexTurnStateScanSettings struct {
	TargetLengths             []int                            `json:"target_lengths"`
	Rules                     []OpenAICodexTurnStateLengthRule `json:"rules"`
	PlanScanEnabled           map[string]bool                  `json:"plan_scan_enabled"`
	RequireStateBeforeRouting *bool                            `json:"require_state_before_routing"`
	RequireRouteBinding       *bool                            `json:"require_route_binding"`
	ParallelProbes            int                              `json:"parallel_probes"`
	ScanRouteMode             string                           `json:"scan_route_mode"`
	DynamicProxyEnabled       bool                             `json:"dynamic_proxy_enabled"`
	DynamicProxyURL           string                           `json:"dynamic_proxy_url"`
}

// OpenAICodexTurnStateLengthRule scopes ordered preferences to a plan/model.
type OpenAICodexTurnStateLengthRule struct {
	PlanType      string `json:"plan_type"`
	Model         string `json:"model"`
	TargetLengths []int  `json:"target_lengths"`
}

func defaultOpenAICodexTurnStateLengthRules() []OpenAICodexTurnStateLengthRule {
	return []OpenAICodexTurnStateLengthRule{
		{PlanType: "pro", Model: "*", TargetLengths: []int{332, 292}},
		{PlanType: "team", Model: "*", TargetLengths: []int{332}},
	}
}

func defaultOpenAICodexTurnStatePlanScanEnabled() map[string]bool {
	return map[string]bool{
		"pro":        true,
		"team":       true,
		"plus":       true,
		"free":       true,
		"enterprise": true,
	}
}

// NormalizeOpenAICodexStatePlanType uses credential values, never account names.
func NormalizeOpenAICodexStatePlanType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "pro", "pro5x", "pro20x", "pro_5x", "pro_20x", "pro-5x", "pro-20x", "prolite", "pro_lite", "pro-lite", "chatgpt_pro", "chatgptpro":
		return "pro"
	case "team", "business", "chatgpt_team", "self_serve_business", "self_serve_business_usage_based", "self_serve_business_prolite", "selfservebusinessprolite":
		return "team"
	default:
		return value
	}
}

func OpenAICodexStatePlanType(account *Account) string {
	if account != nil {
		for _, key := range []string{"plan_type", "chatgpt_plan_type", "subscription_plan"} {
			if value := strings.TrimSpace(account.GetCredential(key)); value != "" {
				return NormalizeOpenAICodexStatePlanType(value)
			}
		}
		// Imported Team credentials can omit the plan while retaining an
		// explicit workspace verification. Never infer the tier from its name.
		verified, _ := account.Extra["team_oauth_verified"].(bool)
		workspace, _ := account.Extra["team_oauth_verified_workspace_id"].(string)
		workspace = strings.TrimSpace(workspace)
		if verified && workspace != "" && workspace == strings.TrimSpace(account.GetCredential("chatgpt_account_id")) {
			return "team"
		}
	}
	return ""
}

// TargetLengthsFor returns a read-only view, with exact plan/model rules first.
func (s *OpenAICodexTurnStateScanSettings) TargetLengthsFor(plan, model string) []int {
	if s == nil {
		s = defaultOpenAICodexTurnStateScanSettings()
	}
	plan = NormalizeOpenAICodexStatePlanType(plan)
	model = normalizeOpenAICodexTurnStateModel(model)
	best := -1
	lengths := s.TargetLengths
	for _, rule := range s.Rules {
		if rule.PlanType != "*" && rule.PlanType != plan || rule.Model != "*" && rule.Model != model {
			continue
		}
		rank := 0
		if rule.PlanType != "*" {
			rank += 2
		}
		if rule.Model != "*" {
			rank++
		}
		if rank > best {
			best, lengths = rank, rule.TargetLengths
		}
	}
	return lengths
}

// IsPlanScanEnabled controls acquisition only. It never disables normal
// account routing or reuse of a state that was acquired earlier.
func (s *OpenAICodexTurnStateScanSettings) IsPlanScanEnabled(plan string) bool {
	if s == nil {
		return true
	}
	plan = NormalizeOpenAICodexStatePlanType(plan)
	if enabled, ok := s.PlanScanEnabled[plan]; ok {
		return enabled
	}
	// Unknown or newly introduced plan types remain enabled for backwards
	// compatibility. Administrators can explicitly disable known plans.
	return true
}

// IsStateRequiredBeforeRouting defaults to true so settings saved before the
// routing guard existed retain the safer behavior when loaded or updated.
func (s *OpenAICodexTurnStateScanSettings) IsStateRequiredBeforeRouting() bool {
	return s == nil || s.RequireStateBeforeRouting == nil || *s.RequireStateBeforeRouting
}

// IsRouteBindingRequired reports whether a reusable scanner ticket must retain
// the same IPv6 or explicitly bound proxy used to acquire it.
func (s *OpenAICodexTurnStateScanSettings) IsRouteBindingRequired() bool {
	return s != nil && s.RequireRouteBinding != nil && *s.RequireRouteBinding
}

func (s *OpenAICodexTurnStateScanSettings) normalizedScanRouteMode() string {
	if s == nil {
		return OpenAICodexTurnStateScanRouteAuto
	}
	mode := strings.ToLower(strings.TrimSpace(s.ScanRouteMode))
	if mode == "" {
		return OpenAICodexTurnStateScanRouteAuto
	}
	return mode
}

func (s *OpenAICodexTurnStateScanSettings) forAccountModel(account *Account, model string) *OpenAICodexTurnStateScanSettings {
	resolved := s.clone()
	resolved.TargetLengths = slices.Clone(s.TargetLengthsFor(OpenAICodexStatePlanType(account), model))
	resolved.Rules = nil
	return resolved
}

func defaultOpenAICodexTurnStateScanSettings() *OpenAICodexTurnStateScanSettings {
	requireStateBeforeRouting := true
	requireRouteBinding := false
	return &OpenAICodexTurnStateScanSettings{
		TargetLengths:             []int{332, 292},
		Rules:                     defaultOpenAICodexTurnStateLengthRules(),
		PlanScanEnabled:           defaultOpenAICodexTurnStatePlanScanEnabled(),
		RequireStateBeforeRouting: &requireStateBeforeRouting,
		RequireRouteBinding:       &requireRouteBinding,
		ParallelProbes:            5,
		ScanRouteMode:             OpenAICodexTurnStateScanRouteAuto,
		DynamicProxyURL:           defaultOpenAICodexTurnStateDynamicProxyURL,
	}
}

func (s *OpenAICodexTurnStateScanSettings) clone() *OpenAICodexTurnStateScanSettings {
	if s == nil {
		return defaultOpenAICodexTurnStateScanSettings()
	}
	copySettings := *s
	if s.RequireStateBeforeRouting != nil {
		requireStateBeforeRouting := *s.RequireStateBeforeRouting
		copySettings.RequireStateBeforeRouting = &requireStateBeforeRouting
	}
	if s.RequireRouteBinding != nil {
		requireRouteBinding := *s.RequireRouteBinding
		copySettings.RequireRouteBinding = &requireRouteBinding
	}
	copySettings.TargetLengths = slices.Clone(s.TargetLengths)
	copySettings.Rules = slices.Clone(s.Rules)
	copySettings.PlanScanEnabled = make(map[string]bool, len(s.PlanScanEnabled))
	for plan, enabled := range s.PlanScanEnabled {
		copySettings.PlanScanEnabled[plan] = enabled
	}
	for i := range copySettings.Rules {
		copySettings.Rules[i].TargetLengths = slices.Clone(s.Rules[i].TargetLengths)
	}
	return &copySettings
}

func (s *OpenAICodexTurnStateScanSettings) lengthRank(length int) int {
	if s == nil {
		return defaultOpenAICodexTurnStateScanSettings().lengthRank(length)
	}
	if index := slices.Index(s.TargetLengths, length); index >= 0 {
		return index
	}
	return len(s.TargetLengths)
}

func (s *OpenAICodexTurnStateScanSettings) acceptsLength(length int) bool {
	if s == nil {
		s = defaultOpenAICodexTurnStateScanSettings()
	}
	return slices.Contains(s.TargetLengths, length)
}

func (s *OpenAICodexTurnStateScanSettings) acceptsLengthFor(plan, model string, length int) bool {
	if s == nil {
		s = defaultOpenAICodexTurnStateScanSettings()
	}
	return slices.Contains(s.TargetLengthsFor(plan, model), length)
}

func (s *OpenAICodexTurnStateScanSettings) primaryLength() int {
	if s == nil || len(s.TargetLengths) == 0 {
		return openAICodexTurnStateLength332
	}
	return s.TargetLengths[0]
}

func validateOpenAICodexTurnStateScanSettings(settings *OpenAICodexTurnStateScanSettings) (*OpenAICodexTurnStateScanSettings, error) {
	if settings == nil || len(settings.TargetLengths) < 1 || len(settings.TargetLengths) > 16 {
		return nil, infraerrors.BadRequest("CODEX_STATE_SCAN_SETTINGS_INVALID", "target_lengths must contain 1 to 16 ordered lengths")
	}
	next := settings.clone()
	if next.RequireStateBeforeRouting == nil {
		requireStateBeforeRouting := true
		next.RequireStateBeforeRouting = &requireStateBeforeRouting
	}
	if next.RequireRouteBinding == nil {
		requireRouteBinding := false
		next.RequireRouteBinding = &requireRouteBinding
	}
	if next.PlanScanEnabled == nil {
		next.PlanScanEnabled = defaultOpenAICodexTurnStatePlanScanEnabled()
	} else {
		normalizedPlanScanEnabled := make(map[string]bool, len(next.PlanScanEnabled)+5)
		for plan, enabled := range next.PlanScanEnabled {
			plan = NormalizeOpenAICodexStatePlanType(plan)
			switch plan {
			case "pro", "team", "plus", "free", "enterprise":
			default:
				return nil, infraerrors.BadRequest("CODEX_STATE_SCAN_SETTINGS_INVALID", "plan_scan_enabled contains an unsupported plan")
			}
			if _, exists := normalizedPlanScanEnabled[plan]; exists {
				return nil, infraerrors.BadRequest("CODEX_STATE_SCAN_SETTINGS_INVALID", "plan_scan_enabled contains duplicate normalized plans")
			}
			normalizedPlanScanEnabled[plan] = enabled
		}
		for plan, enabled := range defaultOpenAICodexTurnStatePlanScanEnabled() {
			if _, exists := normalizedPlanScanEnabled[plan]; !exists {
				normalizedPlanScanEnabled[plan] = enabled
			}
		}
		next.PlanScanEnabled = normalizedPlanScanEnabled
	}
	if next.Rules == nil {
		next.Rules = defaultOpenAICodexTurnStateLengthRules()
	}
	if len(next.Rules) > 128 {
		return nil, infraerrors.BadRequest("CODEX_STATE_SCAN_SETTINGS_INVALID", "rules must contain at most 128 entries")
	}
	ruleKeys := make(map[string]struct{}, len(next.Rules))
	for i := range next.Rules {
		rule := &next.Rules[i]
		rule.PlanType = NormalizeOpenAICodexStatePlanType(rule.PlanType)
		rule.Model = normalizeOpenAICodexTurnStateModel(rule.Model)
		switch rule.PlanType {
		case "*", "pro", "team", "plus", "free", "enterprise":
		default:
			return nil, infraerrors.BadRequest("CODEX_STATE_SCAN_SETTINGS_INVALID", "rule plan_type must be *, pro, team, plus, free, or enterprise")
		}
		if rule.Model == "" || len(rule.Model) > 200 || strings.ContainsAny(rule.Model, "\x00\r\n\t ") || (strings.Contains(rule.Model, "*") && rule.Model != "*") {
			return nil, infraerrors.BadRequest("CODEX_STATE_SCAN_SETTINGS_INVALID", "rule model must be an upstream model name or *")
		}
		key := rule.PlanType + "\x00" + rule.Model
		if _, exists := ruleKeys[key]; exists {
			return nil, infraerrors.BadRequest("CODEX_STATE_SCAN_SETTINGS_INVALID", "duplicate plan/model rules are not allowed")
		}
		ruleKeys[key] = struct{}{}
		if err := validateOpenAICodexTurnStateRuleLengths(rule.TargetLengths); err != nil {
			return nil, err
		}
	}
	seen := make(map[int]struct{}, len(next.TargetLengths))
	for _, length := range next.TargetLengths {
		if length < openAICodexTurnStateMinLength || length > openAICodexTurnStateMaxLength {
			return nil, infraerrors.BadRequest("CODEX_STATE_SCAN_SETTINGS_INVALID", "target lengths must be between 64 and 4096")
		}
		if _, exists := seen[length]; exists {
			return nil, infraerrors.BadRequest("CODEX_STATE_SCAN_SETTINGS_INVALID", "target lengths must not contain duplicates")
		}
		seen[length] = struct{}{}
	}
	if next.ParallelProbes < 1 || next.ParallelProbes > 5 {
		return nil, infraerrors.BadRequest("CODEX_STATE_SCAN_SETTINGS_INVALID", "parallel_probes must be between 1 and 5")
	}
	next.ScanRouteMode = next.normalizedScanRouteMode()
	switch next.ScanRouteMode {
	case OpenAICodexTurnStateScanRouteAuto,
		OpenAICodexTurnStateScanRouteManagedProxy,
		OpenAICodexTurnStateScanRouteIPv6,
		OpenAICodexTurnStateScanRouteDynamicProxy:
	default:
		return nil, infraerrors.BadRequest("CODEX_STATE_SCAN_SETTINGS_INVALID", "scan_route_mode must be auto, managed_proxy, ipv6, or dynamic_proxy")
	}
	next.DynamicProxyURL = strings.TrimSpace(next.DynamicProxyURL)
	if next.DynamicProxyURL == "" && !next.DynamicProxyEnabled {
		next.DynamicProxyURL = defaultOpenAICodexTurnStateDynamicProxyURL
	}
	parsed, err := url.Parse(next.DynamicProxyURL)
	if err != nil || len(next.DynamicProxyURL) > 2048 || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || strings.Contains(next.DynamicProxyURL, "#") || parsed.Opaque != "" {
		return nil, infraerrors.BadRequest("CODEX_STATE_SCAN_SETTINGS_INVALID", "dynamic_proxy_url must be an HTTPS URL without user information or a fragment")
	}
	return next, nil
}

func validateOpenAICodexTurnStateRuleLengths(lengths []int) error {
	if len(lengths) < 1 || len(lengths) > 16 {
		return infraerrors.BadRequest("CODEX_STATE_SCAN_SETTINGS_INVALID", "rule target_lengths must contain 1 to 16 ordered lengths")
	}
	seen := make(map[int]struct{}, len(lengths))
	for _, length := range lengths {
		if length < openAICodexTurnStateMinLength || length > openAICodexTurnStateMaxLength {
			return infraerrors.BadRequest("CODEX_STATE_SCAN_SETTINGS_INVALID", "target lengths must be between 64 and 4096")
		}
		if _, exists := seen[length]; exists {
			return infraerrors.BadRequest("CODEX_STATE_SCAN_SETTINGS_INVALID", "target lengths must not contain duplicates")
		}
		seen[length] = struct{}{}
	}
	return nil
}

// GetOpenAICodexTurnStateScanSettings returns a detached snapshot, so callers
// cannot mutate the settings concurrently used by request routing and scanning.
func (s *OpsService) GetOpenAICodexTurnStateScanSettings() *OpenAICodexTurnStateScanSettings {
	if s == nil {
		return defaultOpenAICodexTurnStateScanSettings()
	}
	return s.codexTurnStateScanSettings.Load().clone()
}

func (s *OpsService) applyOpenAICodexTurnStateScanSettings(settings *OpenAICodexTurnStateScanSettings) {
	settings = settings.clone()
	if s.openAIGatewayService != nil {
		s.openAIGatewayService.getOpenAICodexTurnStatePool().setScanSettings(settings)
	}
	if s.codexTurnStateScanner != nil {
		s.codexTurnStateScanner.settings.Store(settings)
	}
	s.codexTurnStateScanSettings.Store(settings)
}

func (s *OpsService) UpdateOpenAICodexTurnStateScanSettings(ctx context.Context, settings *OpenAICodexTurnStateScanSettings) (*OpenAICodexTurnStateScanSettings, error) {
	next, err := validateOpenAICodexTurnStateScanSettings(settings)
	if err != nil {
		return nil, err
	}
	if s == nil || s.settingRepo == nil {
		return nil, fmt.Errorf("codex state scan settings storage is unavailable")
	}
	data, err := json.Marshal(next)
	if err != nil {
		return nil, fmt.Errorf("encode Codex state scan settings: %w", err)
	}
	s.runtimeSettingsMu.Lock()
	defer s.runtimeSettingsMu.Unlock()
	if err := s.settingRepo.Set(ctx, SettingKeyOpenAICodexTurnStateScanSettings, string(data)); err != nil {
		return nil, fmt.Errorf("save Codex state scan settings: %w", err)
	}
	s.applyOpenAICodexTurnStateScanSettings(next)
	return next.clone(), nil
}
