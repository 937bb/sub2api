//go:build unit

package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type recordingTokenCacheInvalidator struct {
	calls       []*service.Account
	changeCalls []recordingTokenCacheInvalidatorChangeCall
}

func (r *recordingTokenCacheInvalidator) InvalidateToken(ctx context.Context, account *service.Account) error {
	if account != nil {
		cloned := *account
		r.calls = append(r.calls, &cloned)
	}
	return nil
}

type recordingTokenCacheInvalidatorChangeCall struct {
	previous *service.Account
	updated  *service.Account
}

func (r *recordingTokenCacheInvalidator) InvalidateTokenForAccountChange(ctx context.Context, previousAccount, updatedAccount *service.Account) error {
	call := recordingTokenCacheInvalidatorChangeCall{}
	if previousAccount != nil {
		cloned := *previousAccount
		call.previous = &cloned
	}
	if updatedAccount != nil {
		cloned := *updatedAccount
		call.updated = &cloned
	}
	r.changeCalls = append(r.changeCalls, call)

	account := updatedAccount
	if account == nil || !shouldRecordTokenInvalidationAccount(account) {
		account = previousAccount
	}
	return r.InvalidateToken(ctx, account)
}

func shouldRecordTokenInvalidationAccount(account *service.Account) bool {
	if account == nil {
		return false
	}
	if account.IsOpenAIOAuthLike() {
		return true
	}
	return account.Platform == service.PlatformAntigravity && account.Type == service.AccountTypeOAuth
}

func setupAccountUpdateRouterWithInvalidator(adminSvc service.AdminService, invalidator service.TokenCacheInvalidator) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, invalidator)
	router.PUT("/api/v1/admin/accounts/:id", handler.Update)
	return router
}

func TestAccountHandlerUpdateInvalidatesPreviousOpenAIOAuthLikeWhenTypeChangesToAPIKey(t *testing.T) {
	adminSvc := newStubAdminService()
	adminSvc.getAccountResult = &service.Account{
		ID:       44,
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeOAuth,
		Status:   service.StatusActive,
	}
	adminSvc.updateAccountFunc = func(ctx context.Context, id int64, input *service.UpdateAccountInput) (*service.Account, error) {
		return &service.Account{
			ID:       id,
			Platform: service.PlatformOpenAI,
			Type:     service.AccountTypeAPIKey,
			Status:   service.StatusActive,
		}, nil
	}
	invalidator := &recordingTokenCacheInvalidator{}
	router := setupAccountUpdateRouterWithInvalidator(adminSvc, invalidator)

	body, err := json.Marshal(map[string]any{
		"type": "apikey",
		"credentials": map[string]any{
			"api_key": "test-value",
		},
	})
	require.NoError(t, err)
	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodPut, "/api/v1/admin/accounts/44", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Len(t, invalidator.calls, 1)
	require.Equal(t, int64(44), invalidator.calls[0].ID)
	require.Equal(t, service.AccountTypeOAuth, invalidator.calls[0].Type)
}

func TestAccountHandlerUpdateInvalidatesResultingOpenAISetupToken(t *testing.T) {
	adminSvc := newStubAdminService()
	adminSvc.getAccountResult = &service.Account{
		ID:       45,
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeAPIKey,
		Status:   service.StatusActive,
	}
	adminSvc.updateAccountFunc = func(ctx context.Context, id int64, input *service.UpdateAccountInput) (*service.Account, error) {
		return &service.Account{
			ID:       id,
			Platform: service.PlatformOpenAI,
			Type:     service.AccountTypeSetupToken,
			Status:   service.StatusActive,
		}, nil
	}
	invalidator := &recordingTokenCacheInvalidator{}
	router := setupAccountUpdateRouterWithInvalidator(adminSvc, invalidator)

	body, err := json.Marshal(map[string]any{
		"type": "setup-token",
		"credentials": map[string]any{
			"access_token": "test-value",
		},
	})
	require.NoError(t, err)
	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodPut, "/api/v1/admin/accounts/45", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Len(t, invalidator.calls, 1)
	require.Equal(t, int64(45), invalidator.calls[0].ID)
	require.Equal(t, service.AccountTypeSetupToken, invalidator.calls[0].Type)
}

func TestAccountHandlerUpdateInvalidatesAntigravityWithPreviousProjectIdentity(t *testing.T) {
	adminSvc := newStubAdminService()
	adminSvc.getAccountResult = &service.Account{
		ID:       46,
		Platform: service.PlatformAntigravity,
		Type:     service.AccountTypeOAuth,
		Status:   service.StatusActive,
		Credentials: map[string]any{
			"project_id": "old-project",
		},
	}
	adminSvc.updateAccountFunc = func(ctx context.Context, id int64, input *service.UpdateAccountInput) (*service.Account, error) {
		return &service.Account{
			ID:       id,
			Platform: service.PlatformAntigravity,
			Type:     service.AccountTypeOAuth,
			Status:   service.StatusActive,
			Credentials: map[string]any{
				"antigravity_project_id": "configured-project",
			},
		}, nil
	}
	invalidator := &recordingTokenCacheInvalidator{}
	router := setupAccountUpdateRouterWithInvalidator(adminSvc, invalidator)

	body, err := json.Marshal(map[string]any{
		"credentials": map[string]any{
			"antigravity_project_id": "configured-project",
		},
	})
	require.NoError(t, err)
	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodPut, "/api/v1/admin/accounts/46", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Len(t, invalidator.changeCalls, 1)
	require.NotNil(t, invalidator.changeCalls[0].previous)
	require.NotNil(t, invalidator.changeCalls[0].updated)
	require.Equal(t, "old-project", invalidator.changeCalls[0].previous.GetCredential("project_id"))
	require.Empty(t, invalidator.changeCalls[0].updated.GetCredential("project_id"))
	require.Equal(t, "configured-project", invalidator.changeCalls[0].updated.GetCredential("antigravity_project_id"))
}

type batchCredentialInvalidationAdminService struct {
	*stubAdminService
	accounts        map[int64]*service.Account
	failOnAccountID int64
}

func (s *batchCredentialInvalidationAdminService) GetAccount(ctx context.Context, id int64) (*service.Account, error) {
	account := s.accounts[id]
	if account == nil {
		return nil, errors.New("not found")
	}
	cloned := *account
	if account.Credentials != nil {
		cloned.Credentials = make(map[string]any, len(account.Credentials))
		for k, v := range account.Credentials {
			cloned.Credentials[k] = v
		}
	}
	return &cloned, nil
}

func (s *batchCredentialInvalidationAdminService) UpdateAccount(ctx context.Context, id int64, input *service.UpdateAccountInput) (*service.Account, error) {
	if id == s.failOnAccountID {
		return nil, errors.New("database error")
	}
	account := s.accounts[id]
	if account == nil {
		return nil, errors.New("not found")
	}
	cloned := *account
	cloned.Credentials = input.Credentials
	return &cloned, nil
}

func setupBatchUpdateCredentialsRouterWithInvalidator(adminSvc service.AdminService, invalidator service.TokenCacheInvalidator) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, invalidator)
	router.POST("/api/v1/admin/accounts/batch-update-credentials", handler.BatchUpdateCredentials)
	return router
}

func TestBatchUpdateCredentialsInvalidatesSuccessfulOpenAIOAuthLikeAccountsOnly(t *testing.T) {
	adminSvc := &batchCredentialInvalidationAdminService{
		stubAdminService: newStubAdminService(),
		failOnAccountID:  2,
		accounts: map[int64]*service.Account{
			1: {ID: 1, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive},
			2: {ID: 2, Platform: service.PlatformOpenAI, Type: service.AccountTypeSetupToken, Status: service.StatusActive},
			3: {ID: 3, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Status: service.StatusActive},
		},
	}
	invalidator := &recordingTokenCacheInvalidator{}
	router := setupBatchUpdateCredentialsRouterWithInvalidator(adminSvc, invalidator)

	body, err := json.Marshal(BatchUpdateCredentialsRequest{
		AccountIDs: []int64{1, 2, 3},
		Field:      "account_uuid",
		Value:      "test-uuid",
	})
	require.NoError(t, err)
	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodPost, "/api/v1/admin/accounts/batch-update-credentials", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Len(t, invalidator.calls, 1)
	require.Equal(t, int64(1), invalidator.calls[0].ID)
	require.Equal(t, service.AccountTypeOAuth, invalidator.calls[0].Type)
}

func TestBulkUpdateCredentialsInvalidatesSuccessfulOpenAIOAuthLikeAccounts(t *testing.T) {
	adminSvc := newStubAdminService()
	adminSvc.getAccountsByIDs = func(ctx context.Context, ids []int64) ([]*service.Account, error) {
		return []*service.Account{
			{ID: 1, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive},
			{ID: 2, Platform: service.PlatformOpenAI, Type: service.AccountTypeSetupToken, Status: service.StatusActive},
			{ID: 3, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Status: service.StatusActive},
		}, nil
	}
	invalidator := &recordingTokenCacheInvalidator{}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, invalidator)
	router.POST("/api/v1/admin/accounts/bulk-update", handler.BulkUpdate)

	body, err := json.Marshal(map[string]any{
		"account_ids": []int64{1, 2, 3},
		"credentials": map[string]any{
			"account_uuid": "test-uuid",
		},
	})
	require.NoError(t, err)
	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodPost, "/api/v1/admin/accounts/bulk-update", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Len(t, invalidator.calls, 2)
	require.Equal(t, int64(1), invalidator.calls[0].ID)
	require.Equal(t, int64(2), invalidator.calls[1].ID)
}
