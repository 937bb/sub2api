//go:build unit

package admin

import (
	"bytes"
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

func setupApplyOAuthCredentialsRouter(adminSvc service.AdminService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router.POST("/api/v1/admin/accounts/:id/apply-oauth-credentials", handler.ApplyOAuthCredentials)
	return router
}

func TestAccountHandlerApplyOAuthCredentialsRejectsInvalidExtraUpdate(t *testing.T) {
	svc := newStubAdminService()
	svc.getAccountResult = &service.Account{ID: 51, Name: "oauth", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth}
	svc.applyOAuthCredentialsFunc = func(_ context.Context, _ int64, input *service.ApplyOAuthCredentialsInput) (*service.Account, error) {
		require.Contains(t, input.Extra, "openai_oauth_passthrough", "invalid extra must be validated before credentials/type are persisted")
		return nil, infraerrors.BadRequest("OPENAI_OAUTH_CONFIG_INVALID", "OpenAI OAuth config validation failed: scope=write account_id=51 key=openai_oauth_passthrough action=reject recommendation=delete legacy OAuth passthrough key")
	}
	router := setupApplyOAuthCredentialsRouter(svc)

	body, err := json.Marshal(ApplyOAuthCredentialsRequest{
		Type:        service.AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "token"},
		Extra:       map[string]any{"openai_oauth_passthrough": true},
	})
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/51/apply-oauth-credentials", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.True(t, svc.applyOAuthCredentialsCalled)
	require.False(t, svc.updateAccountCalled)
	require.False(t, svc.updateAccountExtraCalled)
	require.Contains(t, rec.Body.String(), "OPENAI_OAUTH_CONFIG_INVALID")
	require.Contains(t, rec.Body.String(), "openai_oauth_passthrough")
}

func TestAccountHandlerApplyOAuthCredentialsAllowsSuccessfulExtraUpdate(t *testing.T) {
	svc := newStubAdminService()
	svc.getAccountResult = &service.Account{ID: 52, Name: "oauth", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth}
	router := setupApplyOAuthCredentialsRouter(svc)

	body, err := json.Marshal(ApplyOAuthCredentialsRequest{
		Type:        service.AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "token"},
		Extra:       map[string]any{"org_uuid": "org"},
	})
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/52/apply-oauth-credentials", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, svc.applyOAuthCredentialsCalled)
	require.False(t, svc.updateAccountCalled)
	require.False(t, svc.updateAccountExtraCalled)
	require.NotNil(t, svc.lastApplyOAuthCredentialsInput)
	require.Equal(t, "org", svc.lastApplyOAuthCredentialsInput.Extra["org_uuid"])
}

func TestAccountHandlerApplyOAuthCredentialsForwardsExtraDeleteKeys(t *testing.T) {
	svc := newStubAdminService()
	svc.getAccountResult = &service.Account{ID: 55, Name: "setup", Platform: service.PlatformOpenAI, Type: service.AccountTypeSetupToken}
	router := setupApplyOAuthCredentialsRouter(svc)

	body, err := json.Marshal(ApplyOAuthCredentialsRequest{
		Type:            service.AccountTypeSetupToken,
		Credentials:     map[string]any{"access_token": "token"},
		Extra:           map[string]any{"openai_oauth_ws_mode": service.OpenAIOAuthWSModeManagedSession},
		ExtraDeleteKeys: []string{"openai_oauth_passthrough"},
	})
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/55/apply-oauth-credentials", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, svc.applyOAuthCredentialsCalled)
	require.Equal(t, []string{"openai_oauth_passthrough"}, svc.lastApplyOAuthCredentialsInput.ExtraDeleteKeys)
}

func TestAccountHandlerApplyOAuthCredentialsRejectsNonOAuthAccount(t *testing.T) {
	svc := newStubAdminService()
	svc.getAccountResult = &service.Account{ID: 53, Name: "apikey", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey}
	router := setupApplyOAuthCredentialsRouter(svc)

	body, err := json.Marshal(ApplyOAuthCredentialsRequest{
		Type:        service.AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "token"},
	})
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/53/apply-oauth-credentials", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.False(t, svc.updateAccountExtraCalled)
}

func TestAccountHandlerApplyOAuthCredentialsUsesRequestContext(t *testing.T) {
	svc := &contextCapturingAdminService{stubAdminService: newStubAdminService()}
	svc.getAccountResult = &service.Account{ID: 54, Name: "oauth", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth}
	router := setupApplyOAuthCredentialsRouter(svc)

	body, err := json.Marshal(ApplyOAuthCredentialsRequest{
		Type:        service.AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "token"},
		Extra:       map[string]any{"org_uuid": "org"},
	})
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/54/apply-oauth-credentials", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), contextMarkerKey{}, "marker")
	router.ServeHTTP(rec, req.WithContext(ctx))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "marker", svc.applyOAuthCredentialsContextValue)
}

type contextMarkerKey struct{}

type contextCapturingAdminService struct {
	*stubAdminService
	applyOAuthCredentialsContextValue any
}

func (s *contextCapturingAdminService) ApplyOAuthCredentials(ctx context.Context, id int64, input *service.ApplyOAuthCredentialsInput) (*service.Account, error) {
	s.applyOAuthCredentialsContextValue = ctx.Value(contextMarkerKey{})
	return s.stubAdminService.ApplyOAuthCredentials(ctx, id, input)
}
