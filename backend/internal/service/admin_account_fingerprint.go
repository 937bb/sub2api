package service

import (
	"context"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// AccountFingerprint is an allowlist of identity diagnostics. It never includes
// bearer credentials, fingerprint seeds, or request-dependent session IDs.
type AccountFingerprint struct {
	Kiro                *KiroFingerprint `json:"kiro,omitempty"`
	AccountID           int64            `json:"account_id"`
	CredentialAccountID int64            `json:"credential_account_id"`
	Inherited           bool             `json:"inherited"`
	InstallationID      string           `json:"installation_id"`
	Source              string           `json:"source"`
	ConvergenceMode     string           `json:"convergence_mode"`
	ConvergenceReady    bool             `json:"convergence_ready"`
	UserAgentOverride   string           `json:"user_agent_override"`
	WSAccountMode       string           `json:"ws_account_mode"`
}

func (s *adminServiceImpl) GetAccountFingerprint(ctx context.Context, id int64) (*AccountFingerprint, error) {
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, ErrAccountNotFound
	}
	if account.Platform == PlatformKiro && account.Type == AccountTypeOAuth {
		return kiroAccountFingerprintSummary(account), nil
	}
	if !account.IsOpenAIOAuthLike() {
		return nil, infraerrors.BadRequest("FINGERPRINT_UNSUPPORTED", "Fingerprint diagnostics require an OpenAI OAuth/setup-token or Kiro OAuth account")
	}
	owner, err := resolveCredentialAccount(ctx, s.accountRepo, account)
	if err != nil {
		return nil, err
	}
	return accountFingerprintSummary(account, owner), nil
}

func accountFingerprintSummary(account, owner *Account) *AccountFingerprint {
	installationID := codexAccountInstallationID(owner)
	source := "unavailable"
	if installationID != "" {
		if owner.GetOpenAIDeviceID() != "" {
			source = "explicit_device"
		} else if _, ok := codexFingerprintSeed(owner.Extra); ok {
			source = "persisted_seed"
		} else if strings.TrimSpace(owner.GetChatGPTAccountID()) != "" {
			source = "account_metadata"
		} else {
			source = "setup_token"
		}
	}
	mode := account.GetCodexFingerprintMode()
	_, hasSeed := codexFingerprintSeed(account.Extra)
	// Different defaults expose whether an account-level override actually exists,
	// without pretending the admin HTTP process knows the separate WS runtime.
	wsMode := account.ResolveOpenAIResponsesWebSocketV2Mode(OpenAIWSIngressModeOff)
	if wsMode != account.ResolveOpenAIResponsesWebSocketV2Mode(OpenAIWSIngressModeCtxPool) {
		wsMode = "default"
	}
	return &AccountFingerprint{
		AccountID: account.ID, CredentialAccountID: owner.ID, Inherited: account.IsShadow(),
		InstallationID: installationID, Source: source, ConvergenceMode: string(mode),
		ConvergenceReady:  mode == codexFingerprintOff || hasSeed,
		UserAgentOverride: account.GetOpenAIUserAgent(), WSAccountMode: wsMode,
	}
}
