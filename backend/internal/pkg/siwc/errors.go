package siwc

import "errors"

// AuthorizationError retains only an allowlisted OAuth code, never token data.
type AuthorizationError struct {
	Code string
}

func (e *AuthorizationError) Error() string { return "SIWC authorization failed: " + e.Code }

func IsTerminalRefreshError(err error) bool {
	var failure *AuthorizationError
	if !errors.As(err, &failure) {
		return false
	}
	switch failure.Code {
	case "invalid_grant", "invalid_refresh_token", "token_expired", "refresh_token_expired", "refresh_token_invalidated", "refresh_token_reused":
		return true
	default:
		return false
	}
}
