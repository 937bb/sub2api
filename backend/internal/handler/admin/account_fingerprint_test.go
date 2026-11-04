package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type fingerprintAdminStub struct {
	service.AdminService
	id int64
}

func (s *fingerprintAdminStub) GetAccountFingerprint(_ context.Context, id int64) (*service.AccountFingerprint, error) {
	s.id = id
	if id == 404 {
		return nil, service.ErrAccountNotFound
	}
	return &service.AccountFingerprint{AccountID: id, InstallationID: "device"}, nil
}
func TestGetFingerprintValidatesIDAndDisablesCaching(t *testing.T) {
	for _, tc := range []struct {
		id     string
		status int
	}{{"0", 400}, {"-2", 400}, {"invalid", 400}, {"404", 404}, {"42", 200}} {
		t.Run(tc.id, func(t *testing.T) {
			stub := &fingerprintAdminStub{}
			h := &AccountHandler{adminService: stub}
			router := gin.New()
			router.GET("/accounts/:id/fingerprint", h.GetFingerprint)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/accounts/"+tc.id+"/fingerprint", nil))
			require.Equal(t, tc.status, rec.Code)
			require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
			if tc.status == 400 {
				require.Zero(t, stub.id)
			}
			if tc.status == 200 {
				require.Equal(t, int64(42), stub.id)
				require.Contains(t, rec.Body.String(), `"installation_id":"device"`)
			}
		})
	}
}
