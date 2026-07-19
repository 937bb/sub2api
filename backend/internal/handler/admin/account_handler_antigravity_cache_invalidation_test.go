package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type antigravityTokenCacheDeleteRecorder struct {
	deletedKeys []string
}

func (c *antigravityTokenCacheDeleteRecorder) GetAccessToken(context.Context, string) (string, error) {
	return "", nil
}

func (c *antigravityTokenCacheDeleteRecorder) SetAccessToken(context.Context, string, string, time.Duration) error {
	return nil
}

func (c *antigravityTokenCacheDeleteRecorder) DeleteAccessToken(_ context.Context, cacheKey string) error {
	c.deletedKeys = append(c.deletedKeys, cacheKey)
	return nil
}

func (c *antigravityTokenCacheDeleteRecorder) AcquireRefreshLock(context.Context, string, time.Duration) (bool, error) {
	return true, nil
}

func (c *antigravityTokenCacheDeleteRecorder) ReleaseRefreshLock(context.Context, string) error {
	return nil
}

type antigravityCredentialUpdateAdminService struct {
	*stubAdminService
	accounts map[int64]*service.Account
}

func (s *antigravityCredentialUpdateAdminService) GetAccount(_ context.Context, id int64) (*service.Account, error) {
	account := s.accounts[id]
	if account == nil {
		return nil, errors.New("account not found")
	}
	return cloneAntigravityCacheInvalidationTestAccount(account), nil
}

func (s *antigravityCredentialUpdateAdminService) UpdateAccount(_ context.Context, id int64, input *service.UpdateAccountInput) (*service.Account, error) {
	account := s.accounts[id]
	if account == nil {
		return nil, errors.New("account not found")
	}
	updated := cloneAntigravityCacheInvalidationTestAccount(account)
	updated.Credentials = cloneAntigravityCacheInvalidationTestCredentials(input.Credentials)
	return updated, nil
}

func cloneAntigravityCacheInvalidationTestAccount(account *service.Account) *service.Account {
	if account == nil {
		return nil
	}
	cloned := *account
	cloned.Credentials = cloneAntigravityCacheInvalidationTestCredentials(account.Credentials)
	return &cloned
}

func cloneAntigravityCacheInvalidationTestCredentials(credentials map[string]any) map[string]any {
	if credentials == nil {
		return nil
	}
	cloned := make(map[string]any, len(credentials))
	for key, value := range credentials {
		cloned[key] = value
	}
	return cloned
}

func setupAntigravityCacheInvalidationAccountRouter(adminSvc service.AdminService, invalidator service.TokenCacheInvalidator) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, invalidator)
	router.POST("/api/v1/admin/accounts/batch-update-credentials", handler.BatchUpdateCredentials)
	router.POST("/api/v1/admin/accounts/bulk-update", handler.BulkUpdate)
	return router
}

func TestBatchUpdateCredentialsAntigravityProjectIDOldToNewInvalidatesOldAndNewKeys(t *testing.T) {
	adminSvc := &antigravityCredentialUpdateAdminService{
		stubAdminService: newStubAdminService(),
		accounts: map[int64]*service.Account{
			101: {
				ID:       101,
				Platform: service.PlatformAntigravity,
				Type:     service.AccountTypeOAuth,
				Status:   service.StatusActive,
				Credentials: map[string]any{
					"project_id": "old-project",
				},
			},
		},
	}
	cache := &antigravityTokenCacheDeleteRecorder{}
	router := setupAntigravityCacheInvalidationAccountRouter(adminSvc, service.NewCompositeTokenCacheInvalidator(cache))

	body, err := json.Marshal(BatchUpdateCredentialsRequest{
		AccountIDs: []int64{101},
		Field:      "project_id",
		Value:      "new-project",
	})
	require.NoError(t, err)
	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodPost, "/api/v1/admin/accounts/batch-update-credentials", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, []string{"ag:old-project", "ag:account:101", "ag:new-project"}, cache.deletedKeys)
}

func TestBatchUpdateCredentialsAntigravityProjectIDOldToFallbackOnlyInvalidatesOldAndAccountKeys(t *testing.T) {
	adminSvc := &antigravityCredentialUpdateAdminService{
		stubAdminService: newStubAdminService(),
		accounts: map[int64]*service.Account{
			102: {
				ID:       102,
				Platform: service.PlatformAntigravity,
				Type:     service.AccountTypeOAuth,
				Status:   service.StatusActive,
				Credentials: map[string]any{
					"project_id":             "old-project",
					"antigravity_project_id": "configured-project",
				},
			},
		},
	}
	cache := &antigravityTokenCacheDeleteRecorder{}
	router := setupAntigravityCacheInvalidationAccountRouter(adminSvc, service.NewCompositeTokenCacheInvalidator(cache))

	body, err := json.Marshal(BatchUpdateCredentialsRequest{
		AccountIDs: []int64{102},
		Field:      "project_id",
		Value:      "",
	})
	require.NoError(t, err)
	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodPost, "/api/v1/admin/accounts/batch-update-credentials", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, []string{"ag:old-project", "ag:account:102"}, cache.deletedKeys)
}

func TestBulkUpdateAntigravityProjectIDOldToNewInvalidatesOldAndNewKeys(t *testing.T) {
	adminSvc := newStubAdminService()
	adminSvc.bulkUpdateAccountsFunc = func(_ context.Context, input *service.BulkUpdateAccountsInput) (*service.BulkUpdateAccountsResult, error) {
		return &service.BulkUpdateAccountsResult{
			Success:    1,
			SuccessIDs: []int64{201},
			Results: []service.BulkUpdateAccountResult{{
				AccountID: 201,
				Success:   true,
			}},
			PreviousAccounts: []*service.Account{{
				ID:       201,
				Platform: service.PlatformAntigravity,
				Type:     service.AccountTypeOAuth,
				Status:   service.StatusActive,
				Credentials: map[string]any{
					"project_id": "old-project",
				},
			}},
		}, nil
	}
	adminSvc.getAccountsByIDs = func(_ context.Context, ids []int64) ([]*service.Account, error) {
		require.Equal(t, []int64{201}, ids)
		return []*service.Account{{
			ID:       201,
			Platform: service.PlatformAntigravity,
			Type:     service.AccountTypeOAuth,
			Status:   service.StatusActive,
			Credentials: map[string]any{
				"project_id": "new-project",
			},
		}}, nil
	}
	cache := &antigravityTokenCacheDeleteRecorder{}
	router := setupAntigravityCacheInvalidationAccountRouter(adminSvc, service.NewCompositeTokenCacheInvalidator(cache))

	body, err := json.Marshal(map[string]any{
		"account_ids": []int64{201},
		"credentials": map[string]any{
			"project_id": "new-project",
		},
	})
	require.NoError(t, err)
	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodPost, "/api/v1/admin/accounts/bulk-update", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, []string{"ag:old-project", "ag:account:201", "ag:new-project"}, cache.deletedKeys)
}

func TestBulkUpdateAntigravityProjectIDOldToFallbackOnlyInvalidatesOldAndAccountKeys(t *testing.T) {
	adminSvc := newStubAdminService()
	adminSvc.bulkUpdateAccountsFunc = func(_ context.Context, input *service.BulkUpdateAccountsInput) (*service.BulkUpdateAccountsResult, error) {
		return &service.BulkUpdateAccountsResult{
			Success:    1,
			SuccessIDs: []int64{202},
			Results: []service.BulkUpdateAccountResult{{
				AccountID: 202,
				Success:   true,
			}},
			PreviousAccounts: []*service.Account{{
				ID:       202,
				Platform: service.PlatformAntigravity,
				Type:     service.AccountTypeOAuth,
				Status:   service.StatusActive,
				Credentials: map[string]any{
					"project_id": "old-project",
				},
			}},
		}, nil
	}
	adminSvc.getAccountsByIDs = func(_ context.Context, ids []int64) ([]*service.Account, error) {
		require.Equal(t, []int64{202}, ids)
		return []*service.Account{{
			ID:       202,
			Platform: service.PlatformAntigravity,
			Type:     service.AccountTypeOAuth,
			Status:   service.StatusActive,
			Credentials: map[string]any{
				"project_id":             "",
				"antigravity_project_id": "configured-project",
			},
		}}, nil
	}
	cache := &antigravityTokenCacheDeleteRecorder{}
	router := setupAntigravityCacheInvalidationAccountRouter(adminSvc, service.NewCompositeTokenCacheInvalidator(cache))

	body, err := json.Marshal(map[string]any{
		"account_ids": []int64{202},
		"credentials": map[string]any{
			"project_id":             "",
			"antigravity_project_id": "configured-project",
		},
	})
	require.NoError(t, err)
	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodPost, "/api/v1/admin/accounts/bulk-update", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, []string{"ag:old-project", "ag:account:202"}, cache.deletedKeys)
}
