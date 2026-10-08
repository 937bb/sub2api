package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestSIWCUnavailableCatalogDoesNotAdvertiseCodexDefaults(t *testing.T) {
	svc := &availableModelsAdminService{stubAdminService: &stubAdminService{}, account: service.Account{
		ID: 1, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		Credentials: map[string]any{"client_id": "oaiapp_legacy"},
	}}
	router := setupAvailableModelsRouter(svc)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/1/models", nil))
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.NotContains(t, w.Body.String(), "gpt-")
}
