package dto

import "github.com/Wei-Shaw/sub2api/internal/service"

func RedactAccountExtra(extra map[string]any) map[string]any {
	return redactAccountExtra(extra, true)
}

func RedactAccountExtraForAccount(account *service.Account) map[string]any {
	if account == nil {
		return nil
	}
	return redactAccountExtra(account.Extra, account.IsOpenAIOAuthLike())
}

func redactAccountExtra(extra map[string]any, includeOpenAICodexFingerprint bool) map[string]any {
	if extra == nil {
		return nil
	}
	out := make(map[string]any, len(extra))
	for key, value := range extra {
		if key == service.OpenAICodexFingerprintExtraKey {
			if includeOpenAICodexFingerprint {
				out[key] = redactOpenAICodexFingerprintExtra(value)
			}
			continue
		}
		out[key] = value
	}
	return out
}

func redactOpenAICodexFingerprintExtra(value any) any {
	fp, ok := coerceOpenAICodexFingerprintExtra(value)
	if !ok {
		return map[string]any{"present": true}
	}
	return map[string]any{
		"schema_version": fp.SchemaVersion,
		"present":        true,
		"created_at":     fp.CreatedAt,
		"updated_at":     fp.UpdatedAt,
	}
}

func coerceOpenAICodexFingerprintExtra(value any) (service.OpenAICodexFingerprint, bool) {
	switch fp := value.(type) {
	case service.OpenAICodexFingerprint:
		return fp, true
	case *service.OpenAICodexFingerprint:
		if fp == nil {
			return service.OpenAICodexFingerprint{}, false
		}
		return *fp, true
	case map[string]any:
		return service.OpenAICodexFingerprint{
			SchemaVersion:  intFromAccountExtra(fp["schema_version"]),
			InstallationID: stringFromAccountExtra(fp["installation_id"]),
			CreatedAt:      stringFromAccountExtra(fp["created_at"]),
			UpdatedAt:      stringFromAccountExtra(fp["updated_at"]),
		}, true
	default:
		return service.OpenAICodexFingerprint{}, false
	}
}

func stringFromAccountExtra(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}

func intFromAccountExtra(value any) int {
	switch n := value.(type) {
	case int:
		return n
	case int32:
		return int(n)
	case int64:
		return int(n)
	case float32:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}
