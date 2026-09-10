package service

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"strings"

	"github.com/google/uuid"
)

const openAICodexInstallationIDExtraKey = "openai_device_id"

func canonicalOpenAICodexInstallationID(value string) (string, bool) {
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Version() != 4 {
		return "", false
	}
	return parsed.String(), true
}

func normalizeOpenAICodexInstallationIDForCreate(platform, accountType string, extra map[string]any) map[string]any {
	if platform != PlatformOpenAI || (accountType != AccountTypeOAuth && accountType != AccountTypeSetupToken) {
		return extra
	}
	next := maps.Clone(extra)
	if next == nil {
		next = make(map[string]any)
	}
	next[openAICodexInstallationIDExtraKey] = uuid.NewString()
	return next
}

func normalizeOpenAICodexInstallationIDForUpdate(platform, accountType string, extra map[string]any, previousInstallationID string) map[string]any {
	next := maps.Clone(extra)
	if platform != PlatformOpenAI || (accountType != AccountTypeOAuth && accountType != AccountTypeSetupToken) {
		if next != nil {
			delete(next, openAICodexInstallationIDExtraKey)
		}
		return next
	}
	if next == nil {
		next = make(map[string]any)
	}
	if existing, ok := canonicalOpenAICodexInstallationID(previousInstallationID); ok {
		next[openAICodexInstallationIDExtraKey] = existing
	} else {
		next[openAICodexInstallationIDExtraKey] = uuid.NewString()
	}
	return next
}

// withOpenAICodexInstallationID returns a request-local account copy carrying
// a stable installation ID. Existing accounts are lazily backfilled once and
// new accounts receive the same field during creation.
func (s *OpenAIGatewayService) withOpenAICodexInstallationID(ctx context.Context, account *Account) *Account {
	if account == nil || !account.IsOpenAIOAuthLike() {
		return account
	}
	if existing, ok := canonicalOpenAICodexInstallationID(account.GetOpenAIDeviceID()); ok {
		return cloneAccountWithOpenAICodexInstallationID(account, existing)
	}
	if seed, ok := codexFingerprintSeed(account.Extra); ok {
		// Use the same stable ID even when session convergence is disabled by
		// the runtime setting. No token, account row or managed seed is changed.
		return cloneAccountWithOpenAICodexInstallationID(account, resolveConvergedInstallationID(account, seed))
	}

	installationID := uuid.NewString()
	shouldPersist := true
	if s != nil && account.ID > 0 {
		actual, loaded := s.openaiCodexInstallationIDs.LoadOrStore(account.ID, installationID)
		if cached, ok := actual.(string); ok {
			installationID = cached
		}
		shouldPersist = !loaded
	}
	requestAccount := cloneAccountWithOpenAICodexInstallationID(account, installationID)
	if s == nil || s.accountRepo == nil || account.ID <= 0 || !shouldPersist {
		return requestAccount
	}
	if err := persistOpenAICodexInstallationID(ctx, s.accountRepo, account.ID, installationID); err != nil {
		slog.Warn("openai_codex_installation_id_backfill_failed", "account_id", account.ID, "error", err)
	}
	return requestAccount
}

func persistOpenAICodexInstallationID(ctx context.Context, repo AccountRepository, accountID int64, installationID string) (err error) {
	// Some forwarding adapters intentionally embed a partial repository. A
	// failed lazy backfill must never take down the request path.
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("update account extra panicked: %v", recovered)
		}
	}()
	return repo.UpdateExtra(ctx, accountID, map[string]any{openAICodexInstallationIDExtraKey: installationID})
}

func cloneAccountWithOpenAICodexInstallationID(account *Account, installationID string) *Account {
	if account == nil {
		return nil
	}
	cloned := *account
	cloned.Extra = maps.Clone(account.Extra)
	if cloned.Extra == nil {
		cloned.Extra = make(map[string]any)
	}
	cloned.Extra[openAICodexInstallationIDExtraKey] = installationID
	return &cloned
}

func openAICodexInstallationIDString(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}
