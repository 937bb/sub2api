package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

type openAICodexFingerprintRepoStub struct {
	updates          []map[string]any
	expectedOldCalls []*OpenAICodexFingerprint
	ensureReturn     *OpenAICodexFingerprint
}

func (r *openAICodexFingerprintRepoStub) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	copied := make(map[string]any, len(updates))
	for k, v := range updates {
		copied[k] = v
	}
	r.updates = append(r.updates, copied)
	return nil
}

func (r *openAICodexFingerprintRepoStub) EnsureOpenAICodexFingerprint(_ context.Context, _ int64, fingerprint OpenAICodexFingerprint, expectedOld *OpenAICodexFingerprint) (OpenAICodexFingerprint, error) {
	r.expectedOldCalls = append(r.expectedOldCalls, expectedOld)
	if r.ensureReturn != nil {
		fingerprint = *r.ensureReturn
	}
	return fingerprint, r.UpdateExtra(context.Background(), 0, map[string]any{OpenAICodexFingerprintExtraKey: fingerprint})
}

type openAICodexFingerprintUAProviderStub struct {
	ua string
}

func (p openAICodexFingerprintUAProviderStub) GetOpenAICodexUserAgent(context.Context) string {
	return p.ua
}

func TestOpenAICodexFingerprintServiceEnsureCreatesAndPersistsOAuthLike(t *testing.T) {
	now := time.Date(2026, 6, 14, 1, 2, 3, 0, time.UTC)
	repo := &openAICodexFingerprintRepoStub{}
	svc := NewOpenAICodexFingerprintService(repo, openAICodexFingerprintUAProviderStub{ua: DefaultOpenAICodexUserAgent})
	svc.now = func() time.Time { return now }
	account := &Account{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{}}

	fp, err := svc.Ensure(context.Background(), account)
	if err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	parsed, err := uuid.Parse(fp.InstallationID)
	if err != nil {
		t.Fatalf("installation id is not uuid: %v", err)
	}
	if parsed.Version() != 4 {
		t.Fatalf("installation uuid version = %d, want 4", parsed.Version())
	}
	if fp.UAProfile.RawUserAgent != DefaultOpenAICodexUserAgent {
		t.Fatalf("raw ua = %q", fp.UAProfile.RawUserAgent)
	}
	if len(repo.updates) != 1 {
		t.Fatalf("UpdateExtra calls = %d, want 1", len(repo.updates))
	}
	if len(repo.expectedOldCalls) != 1 || repo.expectedOldCalls[0] != nil {
		t.Fatalf("expected-old calls = %#v, want [nil]", repo.expectedOldCalls)
	}
	if _, ok := account.Extra[OpenAICodexFingerprintExtraKey].(OpenAICodexFingerprint); !ok {
		t.Fatalf("account extra fingerprint not updated: %#v", account.Extra[OpenAICodexFingerprintExtraKey])
	}
}

func TestOpenAICodexFingerprintServiceEnsureDoesNotMutateWithoutRepository(t *testing.T) {
	now := time.Date(2026, 6, 14, 1, 2, 3, 0, time.UTC)
	svc := NewOpenAICodexFingerprintService(nil, openAICodexFingerprintUAProviderStub{ua: DefaultOpenAICodexUserAgent})
	svc.now = func() time.Time { return now }
	account := &Account{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{}}

	fp, err := svc.Ensure(context.Background(), account)
	if err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	if fp == (OpenAICodexFingerprint{}) {
		t.Fatal("Ensure() returned empty fingerprint")
	}
	if _, ok := account.Extra[OpenAICodexFingerprintExtraKey]; ok {
		t.Fatal("account extra was marked converged without repository persistence")
	}
}

func TestOpenAICodexFingerprintServiceEnsureSkipsPersistWhenValid(t *testing.T) {
	now := time.Date(2026, 6, 14, 1, 2, 3, 0, time.UTC)
	existing := OpenAICodexFingerprint{
		SchemaVersion:  openAICodexFingerprintSchemaV1,
		InstallationID: "550e8400-e29b-41d4-a716-446655440000",
		UAProfile:      ParseOpenAICodexUAProfile(DefaultOpenAICodexUserAgent),
		CreatedAt:      "2026-06-12T00:00:00Z",
		UpdatedAt:      "2026-06-12T00:00:00Z",
	}
	repo := &openAICodexFingerprintRepoStub{}
	svc := NewOpenAICodexFingerprintService(repo, openAICodexFingerprintUAProviderStub{ua: "ignored/1 (OS; arch) term (ignored; 1)"})
	svc.now = func() time.Time { return now }
	account := &Account{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeSetupToken, Extra: map[string]any{OpenAICodexFingerprintExtraKey: existing}}

	fp, err := svc.Ensure(context.Background(), account)
	if err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	if fp != existing {
		t.Fatalf("fingerprint = %#v, want existing %#v", fp, existing)
	}
	if len(repo.updates) != 0 {
		t.Fatalf("UpdateExtra calls = %d, want 0", len(repo.updates))
	}
}

func TestOpenAICodexFingerprintServiceEnsureMigratesLegacyOnlyOnceForHTTP(t *testing.T) {
	repo := &openAICodexFingerprintRepoStub{}
	legacy := OpenAICodexFingerprint{SchemaVersion: 1, InstallationID: "550e8400-e29b-41d4-a716-446655440000", UAProfile: legacyBuiltInOpenAICodexUAProfile, CreatedAt: "2026-06-12T00:00:00Z", UpdatedAt: "2026-06-13T00:00:00Z"}
	account := &Account{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{OpenAICodexFingerprintExtraKey: legacy}}
	req, _ := http.NewRequest(http.MethodPost, "https://example.test/responses", nil)
	fp, err := ensureOpenAICodexFingerprintForRequest(context.Background(), repo, account, req)
	if err != nil || req.Header.Get("User-Agent") != DefaultOpenAICodexUserAgent || fp.InstallationID != legacy.InstallationID {
		t.Fatalf("migration failed: fp=%#v headers=%#v err=%v", fp, req.Header, err)
	}
	if len(repo.updates) != 1 || len(repo.expectedOldCalls) != 1 || repo.expectedOldCalls[0] == nil || *repo.expectedOldCalls[0] != legacy {
		t.Fatalf("CAS calls = %#v updates=%d", repo.expectedOldCalls, len(repo.updates))
	}
	_, err = ensureOpenAICodexFingerprintForRequest(context.Background(), repo, account, req)
	if err != nil || len(repo.updates) != 1 {
		t.Fatalf("second request wrote again: err=%v writes=%d", err, len(repo.updates))
	}
}

func TestOpenAICodexFingerprintServiceEnsureRepairsCorruptExisting(t *testing.T) {
	now := time.Date(2026, 6, 14, 1, 2, 3, 0, time.UTC)
	repo := &openAICodexFingerprintRepoStub{}
	svc := NewOpenAICodexFingerprintService(repo, openAICodexFingerprintUAProviderStub{ua: DefaultOpenAICodexUserAgent})
	svc.now = func() time.Time { return now }
	account := &Account{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{OpenAICodexFingerprintExtraKey: "corrupt"}}

	fp, err := svc.Ensure(context.Background(), account)
	if err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	if fp == (OpenAICodexFingerprint{}) {
		t.Fatal("fingerprint was not repaired")
	}
	if len(repo.expectedOldCalls) != 1 || repo.expectedOldCalls[0] != nil {
		t.Fatalf("expected-old calls = %#v, want [nil] for non-decodable value", repo.expectedOldCalls)
	}
	if got := account.Extra[OpenAICodexFingerprintExtraKey]; got != fp {
		t.Fatalf("account extra fingerprint = %#v, want %#v", got, fp)
	}
}

func TestOpenAICodexFingerprintServiceEnsureUsesPersistedWinner(t *testing.T) {
	now := time.Date(2026, 6, 14, 1, 2, 3, 0, time.UTC)
	persisted := OpenAICodexFingerprint{
		SchemaVersion:  openAICodexFingerprintSchemaV1,
		InstallationID: "550e8400-e29b-41d4-a716-446655440000",
		UAProfile:      ParseOpenAICodexUAProfile("persisted-codex/9.9.9 (Persist OS; arch) Persist_Term/1.0 (persisted-codex; 9.9.9)"),
		CreatedAt:      "2026-06-12T00:00:00Z",
		UpdatedAt:      "2026-06-12T00:00:00Z",
	}
	repo := &openAICodexFingerprintRepoStub{ensureReturn: &persisted}
	svc := NewOpenAICodexFingerprintService(repo, openAICodexFingerprintUAProviderStub{ua: DefaultOpenAICodexUserAgent})
	svc.now = func() time.Time { return now }
	account := &Account{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeSetupToken, Extra: map[string]any{}}

	fp, err := svc.Ensure(context.Background(), account)
	if err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	if fp != persisted {
		t.Fatalf("fingerprint = %#v, want persisted %#v", fp, persisted)
	}
	if got := account.Extra[OpenAICodexFingerprintExtraKey]; got != persisted {
		t.Fatalf("account extra fingerprint = %#v, want persisted %#v", got, persisted)
	}
}

func TestOpenAICodexFingerprintServiceEnsureSkipsAPIKey(t *testing.T) {
	repo := &openAICodexFingerprintRepoStub{}
	svc := NewOpenAICodexFingerprintService(repo, openAICodexFingerprintUAProviderStub{ua: DefaultOpenAICodexUserAgent})
	account := &Account{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{}}

	fp, err := svc.Ensure(context.Background(), account)
	if err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	if fp != (OpenAICodexFingerprint{}) {
		t.Fatalf("fingerprint = %#v, want zero", fp)
	}
	if len(repo.updates) != 0 {
		t.Fatalf("UpdateExtra calls = %d, want 0", len(repo.updates))
	}
	if _, ok := account.Extra[OpenAICodexFingerprintExtraKey]; ok {
		t.Fatal("APIKey account extra was modified")
	}
}
