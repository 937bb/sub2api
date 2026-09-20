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

// OpenAICodexTurnStateScanSettings configures state acquisition only. TargetLengths
// is an ordered local preference, not an assertion about upstream token semantics.
type OpenAICodexTurnStateScanSettings struct {
	TargetLengths       []int  `json:"target_lengths"`
	ParallelProbes      int    `json:"parallel_probes"`
	DynamicProxyEnabled bool   `json:"dynamic_proxy_enabled"`
	DynamicProxyURL     string `json:"dynamic_proxy_url"`
}

func defaultOpenAICodexTurnStateScanSettings() *OpenAICodexTurnStateScanSettings {
	return &OpenAICodexTurnStateScanSettings{
		TargetLengths:   []int{332, 292},
		ParallelProbes:  5,
		DynamicProxyURL: defaultOpenAICodexTurnStateDynamicProxyURL,
	}
}

func (s *OpenAICodexTurnStateScanSettings) clone() *OpenAICodexTurnStateScanSettings {
	if s == nil {
		return defaultOpenAICodexTurnStateScanSettings()
	}
	copySettings := *s
	copySettings.TargetLengths = slices.Clone(s.TargetLengths)
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
		return defaultOpenAICodexTurnStateScanSettings().acceptsLength(length)
	}
	return s.lengthRank(length) < len(s.TargetLengths)
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
	seen := make(map[int]struct{}, len(next.TargetLengths))
	for _, length := range next.TargetLengths {
		if length < 64 || length > 4096 {
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
		s.openAIGatewayService.getOpenAICodexTurnStatePool().setTargetLengths(settings.TargetLengths)
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
