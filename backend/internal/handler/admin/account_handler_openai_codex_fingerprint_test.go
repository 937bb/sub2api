//go:build unit

package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func setupResetOpenAICodexFingerprintRouter(adminSvc service.AdminService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router.POST("/api/v1/admin/accounts/:id/reset-openai-codex-fingerprint", handler.ResetOpenAICodexFingerprint)
	return router
}

func TestAccountHandlerResetOpenAICodexFingerprintReturnsRedactedAccount(t *testing.T) {
	fingerprint := service.OpenAICodexFingerprint{
		SchemaVersion:  1,
		InstallationID: "550e8400-e29b-41d4-a716-446655440000",
		UAProfile:      service.ParseOpenAICodexUAProfile(service.DefaultOpenAICodexUserAgent),
		CreatedAt:      "2026-06-16T00:00:00Z",
		UpdatedAt:      "2026-06-16T00:00:00Z",
	}
	svc := newStubAdminService()
	svc.resetOpenAICodexFingerprintFunc = func(ctx context.Context, id int64) (*service.Account, error) {
		return &service.Account{
			ID:       id,
			Name:     "oauth",
			Platform: service.PlatformOpenAI,
			Type:     service.AccountTypeOAuth,
			Status:   service.StatusActive,
			Extra: map[string]any{
				service.OpenAICodexFingerprintExtraKey: fingerprint,
				"keep":                                 "value",
			},
		}, nil
	}
	router := setupResetOpenAICodexFingerprintRouter(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/61/reset-openai-codex-fingerprint", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, svc.resetOpenAICodexFingerprintCalled)
	require.NotContains(t, rec.Body.String(), fingerprint.InstallationID)
	require.NotContains(t, rec.Body.String(), service.DefaultOpenAICodexUserAgent)

	var payload struct {
		Data struct {
			Extra map[string]any `json:"extra"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	require.Equal(t, "value", payload.Data.Extra["keep"])
	redacted, ok := payload.Data.Extra[service.OpenAICodexFingerprintExtraKey].(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, redacted["present"])
	require.Equal(t, float64(1), redacted["schema_version"])
	require.Equal(t, fingerprint.CreatedAt, redacted["created_at"])
	require.Equal(t, fingerprint.UpdatedAt, redacted["updated_at"])
	require.NotContains(t, redacted, "installation_id")
	require.NotContains(t, redacted, "ua_profile")
}

func TestAccountHandlerResetOpenAICodexFingerprintRejectsUnsupportedAccount(t *testing.T) {
	svc := newStubAdminService()
	svc.resetOpenAICodexFingerprintErr = infraerrors.BadRequest("OPENAI_CODEX_FINGERPRINT_RESET_UNSUPPORTED", "OpenAI Codex fingerprint can only be reset for OpenAI OAuth/setup-token accounts")
	router := setupResetOpenAICodexFingerprintRouter(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/62/reset-openai-codex-fingerprint", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.True(t, svc.resetOpenAICodexFingerprintCalled)
	require.Contains(t, rec.Body.String(), "OPENAI_CODEX_FINGERPRINT_RESET_UNSUPPORTED")
}
