package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const oauthRetrySettingKey = "oauth_upstream_retry"

type OAuthRetrySettings struct {
	Enabled     bool  `json:"enabled"`
	MaxRetries  int   `json:"max_retries"`
	StatusCodes []int `json:"status_codes"`
}

type cachedOAuthRetrySettings struct {
	settings OAuthRetrySettings
	expires  time.Time
}

func DefaultOAuthRetrySettings() OAuthRetrySettings {
	return OAuthRetrySettings{MaxRetries: 3, StatusCodes: []int{429, 502, 503, 504}}
}

func ValidateOAuthRetrySettings(v OAuthRetrySettings) error {
	if v.MaxRetries < 0 || v.MaxRetries > 10 {
		return fmt.Errorf("max_retries must be between 0 and 10")
	}
	if len(v.StatusCodes) == 0 || len(v.StatusCodes) > 100 {
		return fmt.Errorf("status_codes must contain 1 to 100 HTTP error codes")
	}
	seen := map[int]bool{}
	for _, code := range v.StatusCodes {
		if code < 400 || code > 599 || seen[code] {
			return fmt.Errorf("status_codes must contain distinct HTTP codes between 400 and 599")
		}
		seen[code] = true
	}
	return nil
}

func cloneOAuthRetrySettings(v OAuthRetrySettings) OAuthRetrySettings {
	v.StatusCodes = append([]int(nil), v.StatusCodes...)
	return v
}

func (s *SettingService) GetOAuthRetrySettings(ctx context.Context) (OAuthRetrySettings, error) {
	if s == nil || s.settingRepo == nil {
		return DefaultOAuthRetrySettings(), nil
	}
	s.oauthRetryMu.Lock()
	defer s.oauthRetryMu.Unlock()
	if cached := s.oauthRetryCache; cached != nil && time.Now().Before(cached.expires) {
		return cloneOAuthRetrySettings(cached.settings), nil
	}
	raw, err := s.settingRepo.GetValue(ctx, oauthRetrySettingKey)
	if err != nil && !errors.Is(err, ErrSettingNotFound) {
		return OAuthRetrySettings{}, err
	}
	value := DefaultOAuthRetrySettings()
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			return OAuthRetrySettings{}, err
		}
		if err := ValidateOAuthRetrySettings(value); err != nil {
			return OAuthRetrySettings{}, err
		}
	}
	s.oauthRetryCache = &cachedOAuthRetrySettings{settings: cloneOAuthRetrySettings(value), expires: time.Now().Add(5 * time.Second)}
	return value, nil
}

func (s *SettingService) SetOAuthRetrySettings(ctx context.Context, value OAuthRetrySettings) error {
	if err := ValidateOAuthRetrySettings(value); err != nil {
		return err
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	s.oauthRetryMu.Lock()
	defer s.oauthRetryMu.Unlock()
	if err := s.settingRepo.Set(ctx, oauthRetrySettingKey, string(raw)); err != nil {
		return err
	}
	s.oauthRetryCache = &cachedOAuthRetrySettings{settings: cloneOAuthRetrySettings(value), expires: time.Now().Add(5 * time.Second)}
	return nil
}
