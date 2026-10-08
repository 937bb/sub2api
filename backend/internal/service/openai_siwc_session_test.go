package service

import (
	"context"
	"errors"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/siwc"
	"github.com/stretchr/testify/require"
)

func TestSIWCSessionRetriesPersistenceWithoutDuplicateAccount(t *testing.T) {
	service := &OpenAIOAuthService{}
	t.Cleanup(service.Stop)
	result, err := service.GenerateSIWCAuthURL(context.Background(), nil, "")
	require.NoError(t, err)
	flow := service.siwcSessions[result.SessionID]
	flow.credential = &siwc.Credential{ClientID: "oaiapp_test", Subject: "original", HostID: result.HostID, AccessToken: "token"}
	flow.models = []string{"gpt-6-astra"}
	callback := siwc.RedirectURI + "?" + url.Values{"code": {"valid-code"}, "state": {flow.protocol.State}, "client_id": {"oaiapp_test"}}.Encode()
	attempts := 0
	create := func(map[string]any, *int64) (*Account, error) {
		attempts++
		if attempts == 1 {
			return nil, errors.New("transaction rolled back")
		}
		return &Account{ID: 102}, nil
	}
	_, err = service.CompleteSIWC(context.Background(), result.SessionID, callback, 0, create)
	require.Error(t, err)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			account, err := service.CompleteSIWC(context.Background(), result.SessionID, callback, 0, create)
			if err != nil || account == nil || account.ID != 102 {
				t.Errorf("retry returned wrong account or error: %v", err)
			}
		}()
	}
	wg.Wait()
	require.Equal(t, 2, attempts)
	_, err = service.CompleteSIWC(context.Background(), result.SessionID, callback, 22, create)
	require.ErrorContains(t, err, "mismatch")
}

func TestSIWCReauthBindsOriginalIdentityAndPreservesAdminCredentials(t *testing.T) {
	service := &OpenAIOAuthService{}
	t.Cleanup(service.Stop)
	account := siwcFixtureAccount()
	account.Credentials["client_id"] = "oaiapp_original"
	account.Credentials["subject"] = "original"
	account.Credentials["ext_agent_host_id"] = "urn:uuid:12121212-1212-4212-8212-121212121212"
	result, err := service.GenerateSIWCReauthURL(context.Background(), account)
	require.NoError(t, err)
	u, err := url.Parse(result.AuthURL)
	require.NoError(t, err)
	require.Equal(t, "oaiapp_original", u.Query().Get("client_id"))
	require.Equal(t, account.GetCredential("ext_agent_host_id"), u.Query().Get("ext_agent_host_id"))
	require.Equal(t, "original", service.siwcSessions[result.SessionID].protocol.Subject)
	require.Empty(t, u.Query().Get("id_token_hint"))
	update := &UpdateAccountInput{Credentials: map[string]any{"model_mapping": map[string]any{"alias": "gpt-6-astra"}}}
	require.NoError(t, preserveSIWCAdminCredentials(account, update))
	require.Equal(t, "siwc", update.Credentials["auth_mode"])
	require.Equal(t, "upstream-token", update.Credentials["access_token"])
	for _, field := range []string{"auth_mode", "subject", "client_id", "access_token", "id_token"} {
		require.Error(t, preserveSIWCAdminCredentials(account, &UpdateAccountInput{Credentials: map[string]any{field: "other"}}))
	}
	refresher := &OpenAITokenRefresher{}
	account.Credentials["refresh_token"] = "refresh"
	account.Credentials["earliest_refresh_at"] = time.Now().Add(time.Hour).Unix()
	require.False(t, refresher.NeedsRefresh(account, 2*time.Hour))
}

func TestSIWCCallbackAuditRedaction(t *testing.T) {
	redacted := RedactAuditBody([]byte(`{"callback_url":"http://127.0.0.1:1455/auth/callback?code=secret-code&state=private-state","session_id":"session-visible"}`), "application/json")
	require.NotContains(t, redacted, "secret-code")
	require.NotContains(t, redacted, "private-state")
	require.Contains(t, redacted, "session-visible")
}

func TestSIWCAdminEditPreservesRotatedCredentialSnapshot(t *testing.T) {
	old := map[string]any{"auth_mode": "siwc", "access_token": "obsolete", "refresh_token": "obsolete-rt", "siwc_models": []string{"old"}, "model_mapping": map[string]any{"alias": "new"}}
	current := map[string]any{"auth_mode": "siwc", "access_token": "rotated", "refresh_token": "rotated-rt", "siwc_models": []string{"new"}, "_token_version": int64(2)}
	merged := PreserveSIWCManagedCredentials(old, current)
	require.Equal(t, "rotated", merged["access_token"])
	require.Equal(t, "rotated-rt", merged["refresh_token"])
	require.Equal(t, []string{"new"}, merged["siwc_models"])
	require.Equal(t, old["model_mapping"], merged["model_mapping"])
	require.Equal(t, "obsolete", old["access_token"])
}
