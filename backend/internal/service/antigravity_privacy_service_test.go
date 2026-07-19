//go:build unit

package service

import (
	"context"
	"testing"
)

type antigravityPrivacyRecorderRepo struct {
	mockAccountRepoForGemini
	calls     int
	updates   map[string]any
	updateErr error
}

func (r *antigravityPrivacyRecorderRepo) UpdateExtra(ctx context.Context, id int64, updates map[string]any) error {
	r.calls++
	r.updates = updates
	return r.updateErr
}

func withAntigravityPrivacySetter(t *testing.T, setter func(context.Context, string, string, string) string) {
	t.Helper()
	previous := setAntigravityPrivacyForAccount
	setAntigravityPrivacyForAccount = setter
	t.Cleanup(func() {
		setAntigravityPrivacyForAccount = previous
	})
}

func applyAntigravitySubscriptionResult(account *Account, result AntigravitySubscriptionResult) (map[string]any, map[string]any) {
	credentials := make(map[string]any)
	for k, v := range account.Credentials {
		credentials[k] = v
	}
	credentials["plan_type"] = result.PlanType

	extra := make(map[string]any)
	for k, v := range account.Extra {
		extra[k] = v
	}
	if result.SubscriptionStatus != "" {
		extra["subscription_status"] = result.SubscriptionStatus
	} else {
		delete(extra, "subscription_status")
	}
	if result.SubscriptionError != "" {
		extra["subscription_error"] = result.SubscriptionError
	} else {
		delete(extra, "subscription_error")
	}
	return credentials, extra
}

func TestApplyAntigravityPrivacyMode_SetsInMemoryExtra(t *testing.T) {
	account := &Account{}

	applyAntigravityPrivacyMode(account, AntigravityPrivacySet)

	if account.Extra == nil {
		t.Fatal("expected account.Extra to be initialized")
	}
	if got := account.Extra["privacy_mode"]; got != AntigravityPrivacySet {
		t.Fatalf("expected privacy_mode %q, got %v", AntigravityPrivacySet, got)
	}
}

func TestApplyAntigravityPrivacyMode_PreservedBySubscriptionResult(t *testing.T) {
	account := &Account{
		Credentials: map[string]any{
			"access_token": "token",
		},
		Extra: map[string]any{
			"existing": "value",
		},
	}
	applyAntigravityPrivacyMode(account, AntigravityPrivacySet)

	_, extra := applyAntigravitySubscriptionResult(account, AntigravitySubscriptionResult{
		PlanType: "Pro",
	})

	if got := extra["privacy_mode"]; got != AntigravityPrivacySet {
		t.Fatalf("expected subscription writeback to keep privacy_mode %q, got %v", AntigravityPrivacySet, got)
	}
	if got := extra["existing"]; got != "value" {
		t.Fatalf("expected existing extra fields to be preserved, got %v", got)
	}
}

func TestAdminService_EnsureAntigravityPrivacy_UsesFallbackProjectID(t *testing.T) {
	repo := &antigravityPrivacyRecorderRepo{}
	var gotProjectID string
	withAntigravityPrivacySetter(t, func(ctx context.Context, accessToken, projectID, proxyURL string) string {
		gotProjectID = projectID
		return AntigravityPrivacySet
	})
	svc := &adminServiceImpl{accountRepo: repo}
	account := &Account{
		ID:       301,
		Platform: PlatformAntigravity,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":                          "token",
			"project_id":                            " ",
			antigravityProjectFallbackCredentialKey: " fallback-project ",
		},
	}

	mode := svc.EnsureAntigravityPrivacy(context.Background(), account)

	if mode != AntigravityPrivacySet {
		t.Fatalf("expected mode %q, got %q", AntigravityPrivacySet, mode)
	}
	if gotProjectID != "fallback-project" {
		t.Fatalf("expected fallback project_id, got %q", gotProjectID)
	}
	if repo.calls != 1 {
		t.Fatalf("expected one UpdateExtra call, got %d", repo.calls)
	}
	if account.Extra["privacy_mode"] != AntigravityPrivacySet {
		t.Fatalf("expected in-memory privacy_mode %q, got %v", AntigravityPrivacySet, account.Extra["privacy_mode"])
	}
}

func TestAdminService_ForceAntigravityPrivacy_UsesFallbackProjectID(t *testing.T) {
	repo := &antigravityPrivacyRecorderRepo{}
	var gotProjectID string
	withAntigravityPrivacySetter(t, func(ctx context.Context, accessToken, projectID, proxyURL string) string {
		gotProjectID = projectID
		return AntigravityPrivacySet
	})
	svc := &adminServiceImpl{accountRepo: repo}
	account := &Account{
		ID:       302,
		Platform: PlatformAntigravity,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":                          "token",
			antigravityProjectFallbackCredentialKey: " fallback-project ",
		},
		Extra: map[string]any{
			"privacy_mode": AntigravityPrivacyFailed,
		},
	}

	mode := svc.ForceAntigravityPrivacy(context.Background(), account)

	if mode != AntigravityPrivacySet {
		t.Fatalf("expected mode %q, got %q", AntigravityPrivacySet, mode)
	}
	if gotProjectID != "fallback-project" {
		t.Fatalf("expected fallback project_id, got %q", gotProjectID)
	}
	if repo.calls != 1 {
		t.Fatalf("expected one UpdateExtra call, got %d", repo.calls)
	}
	if account.Extra["privacy_mode"] != AntigravityPrivacySet {
		t.Fatalf("expected in-memory privacy_mode %q, got %v", AntigravityPrivacySet, account.Extra["privacy_mode"])
	}
}

func TestTokenRefreshService_EnsureAntigravityPrivacy_UsesFallbackProjectID(t *testing.T) {
	repo := &antigravityPrivacyRecorderRepo{}
	var gotProjectID string
	withAntigravityPrivacySetter(t, func(ctx context.Context, accessToken, projectID, proxyURL string) string {
		gotProjectID = projectID
		return AntigravityPrivacySet
	})
	svc := &TokenRefreshService{accountRepo: repo}
	account := &Account{
		ID:       303,
		Platform: PlatformAntigravity,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":                          "token",
			antigravityProjectFallbackCredentialKey: " fallback-project ",
		},
	}

	svc.ensureAntigravityPrivacy(context.Background(), account)

	if gotProjectID != "fallback-project" {
		t.Fatalf("expected fallback project_id, got %q", gotProjectID)
	}
	if repo.calls != 1 {
		t.Fatalf("expected one UpdateExtra call, got %d", repo.calls)
	}
	if account.Extra["privacy_mode"] != AntigravityPrivacySet {
		t.Fatalf("expected in-memory privacy_mode %q, got %v", AntigravityPrivacySet, account.Extra["privacy_mode"])
	}
}

func TestAdminService_EnsureAntigravityPrivacy_MissingProjectPreservesFailedMode(t *testing.T) {
	repo := &antigravityPrivacyRecorderRepo{}
	calls := 0
	var gotProjectID string
	withAntigravityPrivacySetter(t, func(ctx context.Context, accessToken, projectID, proxyURL string) string {
		calls++
		gotProjectID = projectID
		return AntigravityPrivacyFailed
	})
	svc := &adminServiceImpl{accountRepo: repo}
	account := &Account{
		ID:       304,
		Platform: PlatformAntigravity,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":                          "token",
			"project_id":                            " ",
			antigravityProjectFallbackCredentialKey: "\t",
		},
	}

	mode := svc.EnsureAntigravityPrivacy(context.Background(), account)

	if mode != AntigravityPrivacyFailed {
		t.Fatalf("expected missing project mode %q, got %q", AntigravityPrivacyFailed, mode)
	}
	if calls != 1 {
		t.Fatalf("expected privacy setter to be called once, got %d calls", calls)
	}
	if gotProjectID != "" {
		t.Fatalf("expected blank project_id for missing-project behavior, got %q", gotProjectID)
	}
	if repo.calls != 1 {
		t.Fatalf("expected one UpdateExtra call, got %d", repo.calls)
	}
	if account.Extra["privacy_mode"] != AntigravityPrivacyFailed {
		t.Fatalf("expected in-memory privacy_mode %q, got %v", AntigravityPrivacyFailed, account.Extra["privacy_mode"])
	}
}

func TestTokenRefreshService_EnsureAntigravityPrivacy_MissingProjectPreservesFailedMode(t *testing.T) {
	repo := &antigravityPrivacyRecorderRepo{}
	calls := 0
	var gotProjectID string
	withAntigravityPrivacySetter(t, func(ctx context.Context, accessToken, projectID, proxyURL string) string {
		calls++
		gotProjectID = projectID
		return AntigravityPrivacyFailed
	})
	svc := &TokenRefreshService{accountRepo: repo}
	account := &Account{
		ID:       305,
		Platform: PlatformAntigravity,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":                          "token",
			"project_id":                            "",
			antigravityProjectFallbackCredentialKey: " ",
		},
	}

	svc.ensureAntigravityPrivacy(context.Background(), account)

	if calls != 1 {
		t.Fatalf("expected privacy setter to be called once, got %d calls", calls)
	}
	if gotProjectID != "" {
		t.Fatalf("expected blank project_id for missing-project behavior, got %q", gotProjectID)
	}
	if repo.calls != 1 {
		t.Fatalf("expected one UpdateExtra call, got %d", repo.calls)
	}
	if account.Extra["privacy_mode"] != AntigravityPrivacyFailed {
		t.Fatalf("expected in-memory privacy_mode %q, got %v", AntigravityPrivacyFailed, account.Extra["privacy_mode"])
	}
}
