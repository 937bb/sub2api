package service

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
)

type openAIRuntimeDefaults struct {
	oauthWSDefaultEnabled              bool
	codexFingerprintDefaultFullEnabled bool
	expiresAt                          int64
}

var openAIRuntimeDefaultsCache atomic.Pointer[openAIRuntimeDefaults]
var openAIRuntimeDefaultsSF singleflight.Group

const openAIRuntimeDefaultsCacheTTL = 60 * time.Second
const openAIRuntimeDefaultsErrorTTL = 5 * time.Second
const openAIRuntimeDefaultsDBTimeout = 5 * time.Second

func defaultOpenAIRuntimeDefaults(ttl time.Duration) *openAIRuntimeDefaults {
	return &openAIRuntimeDefaults{
		oauthWSDefaultEnabled:              true,
		codexFingerprintDefaultFullEnabled: true,
		expiresAt:                          time.Now().Add(ttl).UnixNano(),
	}
}

func publishOpenAIRuntimeDefaults(oauthWSDefaultEnabled, fingerprintDefaultFullEnabled bool) {
	openAIRuntimeDefaultsSF.Forget("openai_runtime_defaults")
	openAIRuntimeDefaultsCache.Store(&openAIRuntimeDefaults{
		oauthWSDefaultEnabled:              oauthWSDefaultEnabled,
		codexFingerprintDefaultFullEnabled: fingerprintDefaultFullEnabled,
		expiresAt:                          time.Now().Add(openAIRuntimeDefaultsCacheTTL).UnixNano(),
	})
}

// GetOpenAIRuntimeDefaults returns the inherited OpenAI subscription-account
// defaults. Missing settings and repository failures fail open to the historical
// behavior: WS enabled and Codex fingerprint mode full.
func (s *SettingService) GetOpenAIRuntimeDefaults(ctx context.Context) (oauthWSDefaultEnabled, fingerprintDefaultFullEnabled bool) {
	if cached := openAIRuntimeDefaultsCache.Load(); cached != nil && time.Now().UnixNano() < cached.expiresAt {
		return cached.oauthWSDefaultEnabled, cached.codexFingerprintDefaultFullEnabled
	}
	if s == nil || s.settingRepo == nil {
		return true, true
	}

	value, _, _ := openAIRuntimeDefaultsSF.Do("openai_runtime_defaults", func() (any, error) {
		if cached := openAIRuntimeDefaultsCache.Load(); cached != nil && time.Now().UnixNano() < cached.expiresAt {
			return cached, nil
		}
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), openAIRuntimeDefaultsDBTimeout)
		defer cancel()
		values, err := s.settingRepo.GetMultiple(dbCtx, []string{
			SettingKeyOpenAIOAuthWSDefaultEnabled,
			SettingKeyOpenAICodexFingerprintDefaultFullEnabled,
		})
		if err != nil {
			slog.Warn("failed to load OpenAI runtime defaults", "error", err)
			fallback := defaultOpenAIRuntimeDefaults(openAIRuntimeDefaultsErrorTTL)
			openAIRuntimeDefaultsCache.Store(fallback)
			return fallback, nil
		}

		resolved := defaultOpenAIRuntimeDefaults(openAIRuntimeDefaultsCacheTTL)
		if raw, ok := values[SettingKeyOpenAIOAuthWSDefaultEnabled]; ok && raw != "" {
			resolved.oauthWSDefaultEnabled = raw == "true"
		}
		if raw, ok := values[SettingKeyOpenAICodexFingerprintDefaultFullEnabled]; ok && raw != "" {
			resolved.codexFingerprintDefaultFullEnabled = raw == "true"
		}
		openAIRuntimeDefaultsCache.Store(resolved)
		return resolved, nil
	})
	resolved, ok := value.(*openAIRuntimeDefaults)
	if !ok || resolved == nil {
		return true, true
	}
	return resolved.oauthWSDefaultEnabled, resolved.codexFingerprintDefaultFullEnabled
}

func (s *OpenAIGatewayService) openAIRuntimeDefaults(ctx context.Context) (oauthWSDefaultEnabled, fingerprintDefaultFullEnabled bool) {
	if s == nil || s.settingService == nil {
		return true, true
	}
	return s.settingService.GetOpenAIRuntimeDefaults(ctx)
}
