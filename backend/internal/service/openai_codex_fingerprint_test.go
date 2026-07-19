package service

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestParseOpenAICodexUAProfileCodexTUI(t *testing.T) {
	raw := "codex-tui/0.136.0 (Mac OS 26.5.0; arm64) Apple_Terminal/470.2 (codex-tui; 0.136.0)"
	profile := ParseOpenAICodexUAProfile(raw)

	if profile.Originator != "codex-tui" {
		t.Fatalf("originator = %q", profile.Originator)
	}
	if profile.CodexVersion != "0.136.0" {
		t.Fatalf("codex version = %q", profile.CodexVersion)
	}
	if profile.OSFingerprint != "Mac OS 26.5.0; arm64" {
		t.Fatalf("os fingerprint = %q", profile.OSFingerprint)
	}
	if profile.TerminalToken != "Apple_Terminal/470.2" {
		t.Fatalf("terminal token = %q", profile.TerminalToken)
	}
	if profile.RawUserAgent != raw {
		t.Fatalf("raw user agent = %q", profile.RawUserAgent)
	}
}

func TestParseOpenAICodexUAProfileNonAppleTerminalTokens(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		terminal string
	}{
		{
			name:     "ghostty",
			raw:      "codex-tui/0.136.0 (Mac OS 26.5.0; arm64) Ghostty/1.2.3 (codex-tui; 0.136.0)",
			terminal: "Ghostty/1.2.3",
		},
		{
			name:     "vscode",
			raw:      "codex-tui/0.136.0 (Linux 6.8.0; x86_64) vscode (codex-tui; 0.136.0)",
			terminal: "vscode",
		},
		{
			name:     "unknown",
			raw:      "codex-tui/0.136.0 (Windows 11; x86_64) unknown (codex-tui; 0.136.0)",
			terminal: "unknown",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := ParseOpenAICodexUAProfile(tt.raw)
			if profile.TerminalToken != tt.terminal {
				t.Fatalf("terminal token = %q, want %q", profile.TerminalToken, tt.terminal)
			}
			if got := profile.UserAgent(); got != tt.raw {
				t.Fatalf("UserAgent() = %q, want raw %q", got, tt.raw)
			}
		})
	}
}

func TestOpenAICodexUAProfileUserAgentUsesRawWhenPresent(t *testing.T) {
	profile := OpenAICodexUAProfile{
		Originator:    "codex-tui",
		CodexVersion:  "0.136.0",
		OSFingerprint: "Mac OS 26.5.0; arm64",
		TerminalToken: "Apple_Terminal/470.2",
		RawUserAgent:  "custom-codex-tui/9 (Custom OS) CustomTerminal (custom-codex-tui; 9)",
	}
	if got := profile.UserAgent(); got != profile.RawUserAgent {
		t.Fatalf("UserAgent() = %q, want raw %q", got, profile.RawUserAgent)
	}
}

func TestOpenAICodexUAProfileUserAgentRebuildsCodexTUIShape(t *testing.T) {
	profile := OpenAICodexUAProfile{
		Originator:    "codex-tui",
		CodexVersion:  "0.136.0",
		OSFingerprint: "Mac OS 26.5.0; arm64",
		TerminalToken: "Apple_Terminal/470.2",
	}
	want := "codex-tui/0.136.0 (Mac OS 26.5.0; arm64) Apple_Terminal/470.2 (codex-tui; 0.136.0)"
	if got := profile.UserAgent(); got != want {
		t.Fatalf("UserAgent() = %q, want %q", got, want)
	}
}

func TestParseOpenAICodexUAProfileMalformedKeepsRawAndFallbacks(t *testing.T) {
	raw := "not a codex ua but still a header-safe value"
	profile := ParseOpenAICodexUAProfile(raw)

	if profile.RawUserAgent != raw {
		t.Fatalf("raw user agent = %q", profile.RawUserAgent)
	}
	if profile.Originator != codexOfficialOriginator {
		t.Fatalf("originator = %q", profile.Originator)
	}
	if profile.CodexVersion != codexCLIVersion {
		t.Fatalf("codex version = %q", profile.CodexVersion)
	}
	if profile.OSFingerprint != codexOSFingerprint {
		t.Fatalf("os fingerprint = %q", profile.OSFingerprint)
	}
	if profile.TerminalToken != codexTerminalName {
		t.Fatalf("terminal token = %q", profile.TerminalToken)
	}
}

func TestParseOpenAICodexUAProfileRejectsHeaderInvalidRaw(t *testing.T) {
	profile := ParseOpenAICodexUAProfile("codex-tui/0.136.0\r\nX-Bad: y")
	if profile.RawUserAgent != "" {
		t.Fatalf("raw user agent = %q, want empty", profile.RawUserAgent)
	}
	if got := profile.UserAgent(); strings.ContainsAny(got, "\r\n") {
		t.Fatalf("UserAgent() contains header control characters: %q", got)
	}
}

func TestNormalizeOpenAICodexFingerprintDropsHeaderInvalidRaw(t *testing.T) {
	now := time.Date(2026, 6, 13, 1, 2, 3, 0, time.UTC)
	existing := OpenAICodexFingerprint{
		SchemaVersion:  openAICodexFingerprintSchemaV1,
		InstallationID: "550e8400-e29b-41d4-a716-446655440000",
		UAProfile: OpenAICodexUAProfile{
			Originator:    "codex-tui",
			CodexVersion:  "0.136.0",
			OSFingerprint: "Mac OS 26.5.0; arm64",
			TerminalToken: "Apple_Terminal/470.2",
			RawUserAgent:  "codex-tui/0.136.0\r\nX-Bad: y",
		},
		CreatedAt: "2026-06-12T00:00:00Z",
		UpdatedAt: "2026-06-12T00:00:00Z",
	}
	fp, changed := NormalizeOpenAICodexFingerprint(existing, ParseOpenAICodexUAProfile(DefaultOpenAICodexUserAgent), now)
	if !changed {
		t.Fatal("changed = false")
	}
	if fp.UAProfile.RawUserAgent != "" {
		t.Fatalf("raw user agent = %q, want empty", fp.UAProfile.RawUserAgent)
	}
	if got := fp.UAProfile.UserAgent(); strings.ContainsAny(got, "\r\n") {
		t.Fatalf("UserAgent() contains header control characters: %q", got)
	}
}

func TestNormalizeOpenAICodexFingerprintRepairsHeaderInvalidStructuredFields(t *testing.T) {
	now := time.Date(2026, 6, 13, 1, 2, 3, 0, time.UTC)
	existing := OpenAICodexFingerprint{
		SchemaVersion:  openAICodexFingerprintSchemaV1,
		InstallationID: "550e8400-e29b-41d4-a716-446655440000",
		UAProfile: OpenAICodexUAProfile{
			Originator:    "codex-tui\r\nX-Bad: y",
			CodexVersion:  "0.136.0",
			OSFingerprint: "Mac OS 26.5.0; arm64",
			TerminalToken: "Apple_Terminal/470.2",
		},
		CreatedAt: "2026-06-12T00:00:00Z",
		UpdatedAt: "2026-06-12T00:00:00Z",
	}
	fp, changed := NormalizeOpenAICodexFingerprint(existing, ParseOpenAICodexUAProfile(DefaultOpenAICodexUserAgent), now)
	if !changed {
		t.Fatal("changed = false")
	}
	if fp.UAProfile.Originator != codexOfficialOriginator {
		t.Fatalf("originator = %q, want fallback %q", fp.UAProfile.Originator, codexOfficialOriginator)
	}
	if got := fp.UAProfile.UserAgent(); strings.ContainsAny(got, "\r\n") {
		t.Fatalf("UserAgent() contains header control characters: %q", got)
	}
}

func TestOpenAICodexUAProfileUserAgentRepairsHeaderInvalidStructuredFields(t *testing.T) {
	profile := OpenAICodexUAProfile{
		Originator:    "codex-tui",
		CodexVersion:  "0.136.0",
		OSFingerprint: "Mac OS 26.5.0; arm64",
		TerminalToken: "Apple_Terminal/470.2\r\nX-Bad: y",
	}
	if got := profile.UserAgent(); strings.ContainsAny(got, "\r\n") {
		t.Fatalf("UserAgent() contains header control characters: %q", got)
	}
}

func TestOpenAICodexUAProfileUserAgentRepairsGrammarInvalidStructuredFields(t *testing.T) {
	profile := OpenAICodexUAProfile{
		Originator:    "Claude Code",
		CodexVersion:  "0.136.0 beta",
		OSFingerprint: "Bad ) OS",
		TerminalToken: "Term (bad)",
	}
	if got := profile.UserAgent(); got != DefaultOpenAICodexUserAgent {
		t.Fatalf("UserAgent() = %q, want default %q", got, DefaultOpenAICodexUserAgent)
	}
}

func TestNormalizeOpenAICodexFingerprintCreatesUUIDv4(t *testing.T) {
	now := time.Date(2026, 6, 13, 1, 2, 3, 0, time.UTC)
	fp, changed := NormalizeOpenAICodexFingerprint(nil, ParseOpenAICodexUAProfile(DefaultOpenAICodexUserAgent), now)

	if !changed {
		t.Fatal("changed = false")
	}
	parsed, err := uuid.Parse(fp.InstallationID)
	if err != nil {
		t.Fatalf("installation id is not uuid: %v", err)
	}
	if parsed.Version() != 4 {
		t.Fatalf("installation uuid version = %d, want 4", parsed.Version())
	}
	if fp.SchemaVersion != openAICodexFingerprintSchemaV1 {
		t.Fatalf("schema version = %d", fp.SchemaVersion)
	}
	if fp.CreatedAt != now.Format(time.RFC3339) || fp.UpdatedAt != now.Format(time.RFC3339) {
		t.Fatalf("timestamps = %q/%q", fp.CreatedAt, fp.UpdatedAt)
	}
	if fp.UAProfile.RawUserAgent != DefaultOpenAICodexUserAgent {
		t.Fatalf("raw ua = %q", fp.UAProfile.RawUserAgent)
	}
}

func TestNormalizeOpenAICodexFingerprintCanonicalizesUUID(t *testing.T) {
	now := time.Date(2026, 6, 13, 1, 2, 3, 0, time.UTC)
	existing := OpenAICodexFingerprint{
		SchemaVersion:  openAICodexFingerprintSchemaV1,
		InstallationID: "550E8400-E29B-41D4-A716-446655440000",
		UAProfile:      ParseOpenAICodexUAProfile(DefaultOpenAICodexUserAgent),
		CreatedAt:      "2026-06-12T00:00:00Z",
		UpdatedAt:      "2026-06-12T00:00:00Z",
	}
	fp, changed := NormalizeOpenAICodexFingerprint(existing, ParseOpenAICodexUAProfile(DefaultOpenAICodexUserAgent), now)

	if !changed {
		t.Fatal("changed = false")
	}
	if fp.InstallationID != strings.ToLower(existing.InstallationID) {
		t.Fatalf("installation id = %q", fp.InstallationID)
	}
}

func TestNormalizeOpenAICodexFingerprintRepairsNonV4UUID(t *testing.T) {
	now := time.Date(2026, 6, 13, 1, 2, 3, 0, time.UTC)
	existing := OpenAICodexFingerprint{
		SchemaVersion:  openAICodexFingerprintSchemaV1,
		InstallationID: "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
		UAProfile:      ParseOpenAICodexUAProfile(DefaultOpenAICodexUserAgent),
		CreatedAt:      "2026-06-12T00:00:00Z",
		UpdatedAt:      "2026-06-12T00:00:00Z",
	}
	fp, changed := NormalizeOpenAICodexFingerprint(existing, ParseOpenAICodexUAProfile(DefaultOpenAICodexUserAgent), now)
	if !changed {
		t.Fatal("changed = false")
	}
	parsed, err := uuid.Parse(fp.InstallationID)
	if err != nil {
		t.Fatalf("installation id is not uuid: %v", err)
	}
	if parsed.Version() != 4 {
		t.Fatalf("installation uuid version = %d, want 4", parsed.Version())
	}
	if fp.InstallationID == existing.InstallationID {
		t.Fatal("non-v4 installation id was preserved")
	}
}

func TestNormalizeOpenAICodexFingerprintKeepsValidExisting(t *testing.T) {
	now := time.Date(2026, 6, 13, 1, 2, 3, 0, time.UTC)
	existing := OpenAICodexFingerprint{
		SchemaVersion:  openAICodexFingerprintSchemaV1,
		InstallationID: "550e8400-e29b-41d4-a716-446655440000",
		UAProfile:      ParseOpenAICodexUAProfile(DefaultOpenAICodexUserAgent),
		CreatedAt:      "2026-06-12T00:00:00Z",
		UpdatedAt:      "2026-06-12T00:00:00Z",
	}
	fp, changed := NormalizeOpenAICodexFingerprint(existing, ParseOpenAICodexUAProfile("ignored/1 (OS; arch) term (ignored; 1)"), now)

	if changed {
		t.Fatal("changed = true")
	}
	if fp != existing {
		t.Fatalf("fingerprint changed: %#v", fp)
	}
}

func TestNormalizeOpenAICodexFingerprintMigratesExactLegacyBuiltIn(t *testing.T) {
	now := time.Date(2026, 7, 10, 1, 2, 3, 0, time.UTC)
	existing := OpenAICodexFingerprint{SchemaVersion: 1, InstallationID: "550e8400-e29b-41d4-a716-446655440000", UAProfile: legacyBuiltInOpenAICodexUAProfile, CreatedAt: "2026-06-12T00:00:00Z", UpdatedAt: "2026-06-13T00:00:00Z"}
	fp, changed := NormalizeOpenAICodexFingerprint(existing, ParseOpenAICodexUAProfile("custom/9 (Custom OS) term (custom; 9)"), now)
	if !changed || fp.UAProfile.UserAgent() != DefaultOpenAICodexUserAgent {
		t.Fatalf("legacy fingerprint did not migrate: %#v", fp)
	}
	if fp.InstallationID != existing.InstallationID || fp.CreatedAt != existing.CreatedAt {
		t.Fatalf("installation identity changed: %#v", fp)
	}
}

func TestNormalizeOpenAICodexFingerprintPreservesCustomLegacyVersionProfile(t *testing.T) {
	existing := OpenAICodexFingerprint{SchemaVersion: 1, InstallationID: "550e8400-e29b-41d4-a716-446655440000", UAProfile: ParseOpenAICodexUAProfile("custom/0.136.0 (Linux; x86_64) vscode (custom; 0.136.0)"), CreatedAt: "2026-06-12T00:00:00Z", UpdatedAt: "2026-06-13T00:00:00Z"}
	fp, changed := NormalizeOpenAICodexFingerprint(existing, currentBuiltInOpenAICodexUAProfile, time.Now())
	if changed || fp != existing {
		t.Fatalf("custom fingerprint changed: %#v", fp)
	}
}

func TestNormalizeOpenAICodexFingerprintRepairsMissingFields(t *testing.T) {
	now := time.Date(2026, 6, 13, 1, 2, 3, 0, time.UTC)
	defaultProfile := ParseOpenAICodexUAProfile(DefaultOpenAICodexUserAgent)
	existing := map[string]any{
		"schema_version":  openAICodexFingerprintSchemaV1,
		"installation_id": "",
		"ua_profile":      map[string]any{},
	}
	fp, changed := NormalizeOpenAICodexFingerprint(existing, defaultProfile, now)

	if !changed {
		t.Fatal("changed = false")
	}
	if _, err := uuid.Parse(fp.InstallationID); err != nil {
		t.Fatalf("installation id not repaired: %v", err)
	}
	if fp.UAProfile != defaultProfile {
		t.Fatalf("ua profile = %#v, want %#v", fp.UAProfile, defaultProfile)
	}
	if fp.CreatedAt != now.Format(time.RFC3339) || fp.UpdatedAt != now.Format(time.RFC3339) {
		t.Fatalf("timestamps = %q/%q", fp.CreatedAt, fp.UpdatedAt)
	}
}
