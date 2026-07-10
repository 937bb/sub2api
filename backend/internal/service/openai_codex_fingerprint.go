package service

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
)

const (
	OpenAICodexFingerprintExtraKey    = "openai_codex_fingerprint"
	openAICodexFingerprintSchemaV1    = 1
	legacyBuiltInOpenAICodexUserAgent = "codex-tui/0.136.0 (Mac OS 26.5.0; arm64) Apple_Terminal/470.2 (codex-tui; 0.136.0)"
)

var (
	openAICodexUARe                    = regexp.MustCompile(`^([^/\s]+)/([^\s]+) \(([^)]*)\)\s+(.+?)(?:\s+\(([^;()]+);\s*([^()]+)\))?$`)
	legacyBuiltInOpenAICodexUAProfile  = ParseOpenAICodexUAProfile(legacyBuiltInOpenAICodexUserAgent)
	currentBuiltInOpenAICodexUAProfile = ParseOpenAICodexUAProfile(DefaultOpenAICodexUserAgent)
)

// OpenAICodexUAProfile is the codex-tui identity snapshot persisted per
// OpenAI OAuth-like account. RawUserAgent stays authoritative so admin-provided
// UA strings do not get reformatted after the initial snapshot.
type OpenAICodexUAProfile struct {
	Originator    string `json:"originator"`
	CodexVersion  string `json:"codex_version"`
	OSFingerprint string `json:"os_fingerprint"`
	TerminalToken string `json:"terminal_token"`
	RawUserAgent  string `json:"raw_user_agent,omitempty"`
}

// UserAgent returns the exact snapshotted UA when present, otherwise rebuilds
// the agreed codex-tui UA shape from structured fields.
func (p OpenAICodexUAProfile) UserAgent() string {
	if raw := strings.TrimSpace(p.RawUserAgent); raw != "" && isOpenAICodexHeaderValueSafe(raw) {
		return raw
	}
	originator := safeOpenAICodexUAPathTokenComponent(p.Originator, codexOfficialOriginator)
	version := safeOpenAICodexUAPathTokenComponent(p.CodexVersion, codexCLIVersion)
	osFingerprint := safeOpenAICodexUACommentComponent(p.OSFingerprint, codexOSFingerprint)
	terminalToken := safeOpenAICodexUATokenComponent(p.TerminalToken, codexTerminalName)
	return fmt.Sprintf("%s/%s (%s) %s (%s; %s)", originator, version, osFingerprint, terminalToken, originator, version)
}

// OpenAICodexFingerprint is account-scoped Codex identity for OpenAI
// OAuth/setup-token paths. Request/session fields such as thread/window state
// intentionally do not live here.
type OpenAICodexFingerprint struct {
	SchemaVersion  int                  `json:"schema_version"`
	InstallationID string               `json:"installation_id"`
	UAProfile      OpenAICodexUAProfile `json:"ua_profile"`
	CreatedAt      string               `json:"created_at"`
	UpdatedAt      string               `json:"updated_at"`
}

func ParseOpenAICodexUAProfile(rawUA string) OpenAICodexUAProfile {
	rawUA = strings.TrimSpace(rawUA)
	if !isOpenAICodexHeaderValueSafe(rawUA) {
		rawUA = ""
	}
	profile := defaultOpenAICodexUAProfile(rawUA)
	if rawUA == "" {
		return profile
	}
	matches := openAICodexUARe.FindStringSubmatch(rawUA)
	if len(matches) == 0 {
		return profile
	}
	profile.Originator = strings.TrimSpace(matches[1])
	profile.CodexVersion = strings.TrimSpace(matches[2])
	profile.OSFingerprint = strings.TrimSpace(matches[3])
	profile.TerminalToken = strings.TrimSpace(matches[4])
	if suffixOriginator := strings.TrimSpace(matches[5]); suffixOriginator != "" {
		profile.Originator = suffixOriginator
	}
	if suffixVersion := strings.TrimSpace(matches[6]); suffixVersion != "" {
		profile.CodexVersion = suffixVersion
	}
	return profile
}

func defaultOpenAICodexUAProfile(rawUA string) OpenAICodexUAProfile {
	return OpenAICodexUAProfile{
		Originator:    codexOfficialOriginator,
		CodexVersion:  codexCLIVersion,
		OSFingerprint: codexOSFingerprint,
		TerminalToken: codexTerminalName,
		RawUserAgent:  strings.TrimSpace(rawUA),
	}
}

func NormalizeOpenAICodexFingerprint(existing any, defaultProfile OpenAICodexUAProfile, now time.Time) (OpenAICodexFingerprint, bool) {
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	createdAt := now.Format(time.RFC3339)
	fp, ok := coerceOpenAICodexFingerprint(existing)
	if !ok {
		return OpenAICodexFingerprint{
			SchemaVersion:  openAICodexFingerprintSchemaV1,
			InstallationID: uuid.NewString(),
			UAProfile:      normalizeOpenAICodexUAProfile(defaultProfile),
			CreatedAt:      createdAt,
			UpdatedAt:      createdAt,
		}, true
	}
	changed := false
	if fp.SchemaVersion != openAICodexFingerprintSchemaV1 {
		fp.SchemaVersion = openAICodexFingerprintSchemaV1
		changed = true
	}
	if canonical, ok := canonicalOpenAICodexInstallationID(fp.InstallationID); ok {
		if fp.InstallationID != canonical {
			fp.InstallationID = canonical
			changed = true
		}
	} else {
		fp.InstallationID = uuid.NewString()
		changed = true
	}
	if isOpenAICodexUAProfileZero(fp.UAProfile) {
		fp.UAProfile = normalizeOpenAICodexUAProfile(defaultProfile)
		changed = true
	} else {
		normalized := normalizeOpenAICodexUAProfile(fp.UAProfile)
		if normalized == legacyBuiltInOpenAICodexUAProfile {
			normalized = currentBuiltInOpenAICodexUAProfile
		}
		if normalized != fp.UAProfile {
			fp.UAProfile = normalized
			changed = true
		}
	}
	if strings.TrimSpace(fp.CreatedAt) == "" {
		fp.CreatedAt = createdAt
		changed = true
	}
	if changed || strings.TrimSpace(fp.UpdatedAt) == "" {
		fp.UpdatedAt = createdAt
		changed = true
	}
	return fp, changed
}

func canonicalOpenAICodexInstallationID(value string) (string, bool) {
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Version() != 4 {
		return "", false
	}
	return parsed.String(), true
}

func coerceOpenAICodexFingerprint(existing any) (OpenAICodexFingerprint, bool) {
	switch v := existing.(type) {
	case OpenAICodexFingerprint:
		return v, true
	case *OpenAICodexFingerprint:
		if v == nil {
			return OpenAICodexFingerprint{}, false
		}
		return *v, true
	case map[string]any:
		return openAICodexFingerprintFromMap(v)
	default:
		return OpenAICodexFingerprint{}, false
	}
}

func openAICodexFingerprintFromMap(m map[string]any) (OpenAICodexFingerprint, bool) {
	if len(m) == 0 {
		return OpenAICodexFingerprint{}, false
	}
	fp := OpenAICodexFingerprint{
		SchemaVersion:  intFromOpenAICodexAny(m["schema_version"]),
		InstallationID: stringFromOpenAICodexAny(m["installation_id"]),
		CreatedAt:      stringFromOpenAICodexAny(m["created_at"]),
		UpdatedAt:      stringFromOpenAICodexAny(m["updated_at"]),
	}
	if profile, ok := m["ua_profile"].(map[string]any); ok {
		fp.UAProfile = openAICodexUAProfileFromMap(profile)
	} else if profile, ok := m["ua_profile"].(map[string]interface{}); ok {
		fp.UAProfile = openAICodexUAProfileFromMap(map[string]any(profile))
	}
	return fp, true
}

func openAICodexUAProfileFromMap(m map[string]any) OpenAICodexUAProfile {
	return OpenAICodexUAProfile{
		Originator:    stringFromOpenAICodexAny(m["originator"]),
		CodexVersion:  stringFromOpenAICodexAny(m["codex_version"]),
		OSFingerprint: stringFromOpenAICodexAny(m["os_fingerprint"]),
		TerminalToken: firstNonEmptyOpenAICodexString(stringFromOpenAICodexAny(m["terminal_token"]), stringFromOpenAICodexAny(m["terminal_name"])),
		RawUserAgent:  stringFromOpenAICodexAny(m["raw_user_agent"]),
	}
}

// NormalizeOpenAICodexUAProfile sanitizes a Codex UA profile using the same
// defaults as account fingerprint creation.
func NormalizeOpenAICodexUAProfile(profile OpenAICodexUAProfile) OpenAICodexUAProfile {
	return normalizeOpenAICodexUAProfile(profile)
}

func normalizeOpenAICodexUAProfile(profile OpenAICodexUAProfile) OpenAICodexUAProfile {
	profile.Originator = safeOpenAICodexUAPathTokenComponent(profile.Originator, codexOfficialOriginator)
	profile.CodexVersion = safeOpenAICodexUAPathTokenComponent(profile.CodexVersion, codexCLIVersion)
	profile.OSFingerprint = safeOpenAICodexUACommentComponent(profile.OSFingerprint, codexOSFingerprint)
	profile.TerminalToken = safeOpenAICodexUATokenComponent(profile.TerminalToken, codexTerminalName)
	profile.RawUserAgent = strings.TrimSpace(profile.RawUserAgent)
	if !isOpenAICodexHeaderValueSafe(profile.RawUserAgent) {
		profile.RawUserAgent = ""
	}
	return profile
}

func isOpenAICodexUAProfileZero(profile OpenAICodexUAProfile) bool {
	return strings.TrimSpace(profile.Originator) == "" && strings.TrimSpace(profile.CodexVersion) == "" && strings.TrimSpace(profile.OSFingerprint) == "" && strings.TrimSpace(profile.TerminalToken) == "" && strings.TrimSpace(profile.RawUserAgent) == ""
}

func safeOpenAICodexUAComponent(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" || !isOpenAICodexHeaderValueSafe(value) {
		return fallback
	}
	return value
}

func safeOpenAICodexUAPathTokenComponent(value, fallback string) string {
	value = safeOpenAICodexUAComponent(value, fallback)
	if strings.Contains(value, "/") || !isOpenAICodexUATokenComponentSafe(value) {
		return fallback
	}
	return value
}

func safeOpenAICodexUATokenComponent(value, fallback string) string {
	value = safeOpenAICodexUAComponent(value, fallback)
	if !isOpenAICodexUATokenComponentSafe(value) {
		return fallback
	}
	return value
}

func safeOpenAICodexUACommentComponent(value, fallback string) string {
	value = safeOpenAICodexUAComponent(value, fallback)
	if strings.ContainsAny(value, "()") {
		return fallback
	}
	return value
}

func isOpenAICodexUATokenComponentSafe(value string) bool {
	if strings.ContainsAny(value, "();") {
		return false
	}
	for _, r := range value {
		if unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

func isOpenAICodexHeaderValueSafe(value string) bool {
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func stringFromOpenAICodexAny(v any) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func intFromOpenAICodexAny(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int32:
		return int(n)
	case int64:
		return int(n)
	case float64:
		return int(n)
	case float32:
		return int(n)
	default:
		return 0
	}
}
