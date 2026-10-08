//go:build unit

package admin

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/siwc"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type siwcRefreshOAuthStub struct {
	info  *service.OpenAITokenInfo
	calls int
}

func (s *siwcRefreshOAuthStub) RefreshAccountToken(context.Context, *service.Account) (*service.OpenAITokenInfo, error) {
	s.calls++
	return s.info, nil
}

func (s *siwcRefreshOAuthStub) BuildAccountCredentials(info *service.OpenAITokenInfo) map[string]any {
	return service.NewOpenAIOAuthService(nil, nil).BuildAccountCredentials(info)
}

type siwcRefreshAdminStub struct {
	service.AdminService
	expected    *service.Account
	credentials map[string]any
	updated     *service.Account
	err         error
	saveCalls   int
	updateCalls int
}

func (s *siwcRefreshAdminStub) SaveSIWCCredentials(_ context.Context, expected *service.Account, credentials map[string]any) (*service.Account, error) {
	s.saveCalls++
	s.expected, s.credentials = expected, credentials
	return s.updated, s.err
}

func (s *siwcRefreshAdminStub) UpdateAccount(context.Context, int64, *service.UpdateAccountInput) (*service.Account, error) {
	s.updateCalls++
	return nil, errors.New("SIWC refresh must not use an unconditional account update")
}

func siwcRefreshHandlerFixture() (*service.Account, *siwcRefreshOAuthStub) {
	account := &service.Account{
		ID: 807, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		Credentials: map[string]any{
			"auth_mode": siwc.AuthMode, "access_token": "old-access", "refresh_token": "old-refresh",
			"subject": "subject-one", "client_id": "oaiapp_fixture", "ext_agent_host_id": "urn:uuid:fixture",
			"model_mapping": map[string]any{"old-model": "old-model"}, "custom_setting": "stale",
		},
	}
	refresher := &siwcRefreshOAuthStub{info: &service.OpenAITokenInfo{SIWC: &siwc.Credential{
		AccessToken: "rotated-access", RefreshToken: "rotated-refresh", IDToken: "validated-id",
		ClientID: "oaiapp_fixture", HostID: "urn:uuid:fixture", Subject: "subject-one",
		Scope: siwc.Scopes, ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}}}
	return account, refresher
}

func TestRefreshSingleAccountSIWCUsesConditionalTokenOnlySave(t *testing.T) {
	t.Parallel()
	for _, changed := range []bool{false, true} {
		name := "preserve concurrent settings"
		if changed {
			name = "reject replaced grant"
		}
		t.Run(name, func(t *testing.T) {
			account, refresher := siwcRefreshHandlerFixture()
			durable := &service.Account{ID: account.ID, Credentials: map[string]any{"custom_setting": "current"}}
			admin := &siwcRefreshAdminStub{updated: durable}
			if changed {
				admin.updated = nil
				admin.err = errors.New("SIWC credentials changed during authorization")
			}
			handler := &AccountHandler{adminService: admin, openaiOAuthService: refresher}

			updated, warning, err := handler.refreshSingleAccount(context.Background(), account)

			if changed {
				require.ErrorIs(t, err, admin.err)
				require.Nil(t, updated)
			} else {
				require.NoError(t, err)
				require.Same(t, durable, updated)
			}
			require.Empty(t, warning)
			require.Equal(t, 1, refresher.calls)
			require.Equal(t, 1, admin.saveCalls)
			require.Zero(t, admin.updateCalls)
			require.Same(t, account, admin.expected)
			require.Equal(t, "rotated-refresh", admin.credentials["refresh_token"])
			require.NotContains(t, admin.credentials, "model_mapping")
			require.NotContains(t, admin.credentials, "custom_setting")
			require.Equal(t, "old-refresh", account.Credentials["refresh_token"])
		})
	}
}

func TestRefreshSingleAccountSIWCRequiresPersistenceBeforeTokenRotation(t *testing.T) {
	t.Parallel()
	account, refresher := siwcRefreshHandlerFixture()
	handler := &AccountHandler{adminService: newStubAdminService(), openaiOAuthService: refresher}

	updated, _, err := handler.refreshSingleAccount(context.Background(), account)

	require.ErrorContains(t, err, "SIWC credential persistence unavailable")
	require.Nil(t, updated)
	require.Zero(t, refresher.calls)
}
