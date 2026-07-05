package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const antigravityProjectFallbackCredentialKey = "antigravity_project_id"

var errAntigravityProjectIDRequired = errors.New("antigravity oauth account requires project_id or antigravity_project_id")
var errAntigravityProjectBackfillUnavailable = errors.New("antigravity project_id backfill unavailable")

func resolveAntigravityProjectID(account *Account) (string, error) {
	if account == nil {
		return "", errAntigravityProjectIDRequired
	}
	if account.Platform != PlatformAntigravity || account.Type != AccountTypeOAuth {
		return "", errAntigravityProjectIDRequired
	}
	if projectID := strings.TrimSpace(account.GetCredential("project_id")); projectID != "" {
		return projectID, nil
	}
	if projectID := strings.TrimSpace(account.GetCredential(antigravityProjectFallbackCredentialKey)); projectID != "" {
		return projectID, nil
	}
	return "", errAntigravityProjectIDRequired
}

func resolveAntigravityProjectIDAfterToken(ctx context.Context, account *Account, tokenProvider *AntigravityTokenProvider, accessToken string) (string, error) {
	projectID, err := resolveAntigravityProjectID(account)
	if err == nil {
		return projectID, nil
	}
	if account == nil || account.Platform != PlatformAntigravity || account.Type != AccountTypeOAuth {
		return "", err
	}
	if tokenProvider == nil {
		return "", err
	}
	if backfillErr := tokenProvider.BackfillProjectIDIfMissing(ctx, account, accessToken); backfillErr != nil {
		return "", fmt.Errorf("%w: %w", err, errAntigravityProjectBackfillUnavailable)
	}
	return resolveAntigravityProjectID(account)
}
