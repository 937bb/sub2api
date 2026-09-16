package service

import (
	"context"
	"maps"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	AntiDegradationExtraKey = "anti_degradation"
	ProtectionScopeExtraKey = "protection_scope"
)

// AntiDegradationEnabled preserves the meaning of old rows. Absence of the new
// flag is never interpreted as permission to change an existing account.
func (a *Account) AntiDegradationEnabled() bool {
	if a == nil {
		return false
	}
	if enabled, ok := a.Extra[AntiDegradationExtraKey].(bool); ok {
		return enabled
	}
	return antiDegradeEnabled(a)
}

func (a *Account) ProtectionScope() string {
	if !a.AntiDegradationEnabled() {
		return "disabled"
	}
	if isMode1ProtectionEnabled(a) {
		return "codex_v3"
	}
	if a.Extra[ProtectionScopeExtraKey] == "generic_v1" {
		return "generic_v1"
	}
	return "legacy"
}

// ProtectionMode returns the concrete strategy currently persisted on the
// account. Unlike ProtectionScope (which is a coarse capability bucket), this
// value is suitable for admin UI/audit so operators can tell mode1, mode2 and

func (a *Account) ProtectionMode() string {
	if a == nil || !a.AntiDegradationEnabled() {
		return "disabled"
	}
	if isMode1ProtectionEnabled(a) {
		return string(AntiDegradeMode1)
	}
	if isLegacyProtectionEnabled(a) {
		return string(AntiDegradeModeLegacy)
	}
	if marker, ok := a.Extra[AntiDegradeMarkerExtraKey].(map[string]any); ok {
		if mode, ok := marker["mode"].(string); ok && mode != "" {
			return mode
		}
	}
	return string(AntiDegradeMode2)
}

// ProtectionManagedWrite is true only for a server-internal context created by
// a dedicated protection operation. No request JSON can supply this value.
func ProtectionManagedWrite(ctx context.Context) bool {
	return ctx.Value(mode1ManagedWriteKey{}) == true
}

func ProtectionManagedKeys(a *Account) []string {
	keys := []string{AntiDegradationExtraKey, ProtectionScopeExtraKey, AntiDegradeMarkerExtraKey}

	// own the fingerprint/TLS fields they write.  Preserve those fields when
	// an ordinary account edit submits a stale/partial extra object; otherwise
	// saving the modal could silently turn the legacy strategy off.
	if isMode1ProtectionRequested(a) || (antiDegradeEnabled(a) && antiDegradeStrategyProfile(antiDegradeMode(a)).ApplySupported) {
		keys = append(keys, mode1ManagedExtraKeys[1:]...)
	}
	return keys
}

// PreserveAccountProtection is also invoked under the repository row lock, so
// a stale form or refresh cannot undo a concurrently confirmed protection change.
func PreserveAccountProtection(ctx context.Context, current *Account, incoming map[string]any) map[string]any {
	if ProtectionManagedWrite(ctx) {
		return incoming
	}
	result := maps.Clone(incoming)
	if result == nil {
		result = map[string]any{}
	}
	for _, key := range ProtectionManagedKeys(current) {
		if value, exists := current.Extra[key]; exists {
			result[key] = value
		} else {
			delete(result, key)
		}
	}
	// Full-object imports and stale refreshes may omit the identity seed. Keep
	// the committed seed; unlike policy snapshots it never belongs to a caller.
	if seed, ok := codexFingerprintSeed(current.Extra); ok {
		result[codexFingerprintSeedExtraKey] = seed
	}
	return result
}

func BoundAccountProtectionConcurrency(a *Account) {
	if a == nil || !a.AntiDegradationEnabled() {
		return
	}
	if a.Concurrency <= 0 {
		a.Concurrency = AntiDegradeConcurrencyCap
	}
	// The account field is the administrator's editable ceiling. The marker
	// mirrors it for historical displays; strategy presets never clamp it.
	if marker := mode1Marker(a); marker != nil {
		a.Extra = maps.Clone(a.Extra)
		copy := maps.Clone(marker)
		copy["max_concurrency"] = a.Concurrency
		a.Extra[AntiDegradeMarkerExtraKey] = copy
	}
}

func ValidateAccountProtectionConfiguration(a *Account) error {
	if a != nil {
		if err := validateRequestIntegrityExtra(a.Extra); err != nil {
			return err
		}
	}
	if !isMode1ProtectionRequested(a) {
		return validateRegisteredProtection(a)
	}
	if !isMode1ProtectionEnabled(a) {
		return infraerrors.BadRequest("MODE1_CONFIGURATION_INVALID", "账号保护配置不完整，请通过专用保护入口修复")
	}
	if issues := mode1ConfigurationIssues(a); len(issues) > 0 {
		return infraerrors.BadRequest("MODE1_CONFIGURATION_INVALID", issues[0])
	}
	return nil
}
