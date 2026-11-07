//go:build unit

package admin

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestKiroManualRefreshNeverFallsThroughToClaudeOAuth(t *testing.T) {
	// Both services are absent: the Kiro branch must return an error without
	// dereferencing the unrelated Claude OAuth service or updating credentials.
	h := &AccountHandler{}
	_, _, err := h.refreshSingleAccount(context.Background(), &service.Account{ID: 42, Platform: service.PlatformKiro, Type: service.AccountTypeOAuth})
	require.ErrorContains(t, err, "Kiro")
}
