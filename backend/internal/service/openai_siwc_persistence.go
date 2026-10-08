package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

type openAISiwcCredentialsUpdater interface {
	UpdateOpenAISiwcCredentials(context.Context, *Account, map[string]any) (bool, error)
}

var siwcManagedCredentialKeys = []string{"auth_mode", "issuer", "subject", "client_id", "ext_agent_host_id", "access_token", "refresh_token", "id_token", "expires_at", "earliest_refresh_at", "granted_scope", "token_type", "email", "siwc_models", "_token_version"}

// PreserveSIWCManagedCredentials is also applied under the repository row lock
// so an ordinary account edit cannot restore an obsolete token or model list.
func PreserveSIWCManagedCredentials(incoming, current map[string]any) map[string]any {
	merged := shallowCopyMap(incoming)
	if merged == nil {
		merged = make(map[string]any)
	}
	for _, key := range siwcManagedCredentialKeys {
		delete(merged, key)
		if value, exists := current[key]; exists {
			merged[key] = value
		}
	}
	return merged
}

func preserveSIWCAdminCredentials(account *Account, input *UpdateAccountInput) error {
	if !account.IsOpenAISiwc() {
		if mode, _ := input.Credentials["auth_mode"].(string); strings.EqualFold(mode, "siwc") {
			return infraerrors.New(http.StatusBadRequest, "SIWC_AUTHORIZATION_REQUIRED", "Create SIWC accounts through the dedicated authorization flow")
		}
		return nil
	}
	if input.Type != "" && input.Type != account.Type {
		return siwcBadRequest(errors.New("SIWC account type cannot be changed"))
	}
	if input.Credentials == nil {
		return nil
	}
	merged := shallowCopyMap(input.Credentials)
	for _, key := range []string{"auth_mode", "issuer", "subject", "client_id", "ext_agent_host_id", "access_token", "refresh_token", "id_token", "expires_at", "earliest_refresh_at", "granted_scope", "token_type"} {
		if value, supplied := merged[key]; supplied && fmt.Sprint(value) != fmt.Sprint(account.Credentials[key]) {
			return siwcBadRequest(errors.New("Use SIWC reauthorization or token refresh to replace authorization credentials"))
		}
		if value, exists := account.Credentials[key]; exists {
			merged[key] = value
		}
	}
	merged["siwc_models"] = account.Credentials["siwc_models"]
	input.Credentials = merged
	return nil
}

func persistSIWCCredentials(ctx context.Context, repo AccountRepository, expected *Account, credentials map[string]any) (*Account, bool, error) {
	updater, ok := repo.(openAISiwcCredentialsUpdater)
	if !ok {
		return nil, false, errors.New("SIWC credential persistence unavailable")
	}
	applied, err := updater.UpdateOpenAISiwcCredentials(ctx, expected, credentials)
	if err != nil {
		return nil, false, err
	}
	current, err := repo.GetByID(ctx, expected.ID)
	return current, applied, err
}

// SaveSIWCCredentials never replaces account configuration or another identity.
func (s *adminServiceImpl) SaveSIWCCredentials(ctx context.Context, expected *Account, credentials map[string]any) (*Account, error) {
	if !expected.IsOpenAISiwc() {
		return nil, errors.New("SIWC account required")
	}
	for _, key := range []string{"auth_mode", "subject", "client_id", "ext_agent_host_id"} {
		if credentials[key] != expected.GetCredential(key) {
			return nil, errors.New("SIWC authorization identity mismatch")
		}
	}
	credentials["_token_version"] = time.Now().UnixMilli()
	current, applied, err := persistSIWCCredentials(ctx, s.accountRepo, expected, credentials)
	if err != nil {
		return nil, err
	}
	if !applied {
		return nil, errors.New("SIWC credentials changed during authorization; retry saving")
	}
	return current, nil
}
