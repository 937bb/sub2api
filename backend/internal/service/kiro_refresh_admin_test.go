//go:build unit

package service

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
	"github.com/stretchr/testify/require"
)

type kiroAdminRefreshRepo struct {
	AccountRepository
	account *Account
	writes  int
}

func (r *kiroAdminRefreshRepo) GetByID(context.Context, int64) (*Account, error) {
	return r.account, nil
}
func (r *kiroAdminRefreshRepo) UpdateKiroOAuthCredentialsIfUnchanged(_ context.Context, id int64, expected map[string]any, proxy *int64, updated map[string]any) (bool, error) {
	if id != r.account.ID || !reflect.DeepEqual(expected, r.account.Credentials) || !reflect.DeepEqual(proxy, r.account.ProxyID) {
		return false, nil
	}
	r.writes++
	next := *r.account
	next.Credentials = updated
	r.account = &next
	return true, nil
}
func TestKiroAdminRefreshSharesLockAndDurableRotation(t *testing.T) {
	a := fixedKiroAccount()
	a.Status = StatusActive
	a.Credentials["access_token"] = "old-access"
	a.Credentials["expires_at"] = time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	repo := &kiroAdminRefreshRepo{account: a}
	cache := &refreshAPICacheStub{lockResult: true}
	upstream := &kiroProxyUpstream{}
	svc := &AccountTestService{httpUpstream: upstream, claudeTokenProvider: &ClaudeTokenProvider{refreshAPI: NewOAuthRefreshAPI(repo, cache)}}
	refreshed, err := svc.RefreshKiroAccount(context.Background(), a)
	require.NoError(t, err)
	require.Equal(t, 1, upstream.calls)
	require.Equal(t, "prod.us-east-1.auth.desktop.kiro.dev", upstream.request.URL.Host)
	require.Equal(t, "/refreshToken", upstream.request.URL.Path)
	require.Equal(t, "new-refresh", refreshed.GetCredential("refresh_token"))
	require.Equal(t, 1, repo.writes)
	require.Equal(t, 1, cache.releaseCalls)
	require.Equal(t, kiro.EnvironmentKey(a.Credentials), kiro.EnvironmentKey(refreshed.Credentials))
	// A concurrent manual request selected the old snapshot before rotation.
	_, err = svc.RefreshKiroAccount(context.Background(), a)
	require.NoError(t, err)
	require.Equal(t, 1, upstream.calls)
	require.Equal(t, 1, repo.writes)
	require.Equal(t, 2, cache.releaseCalls)
}
func TestKiroAdminRefreshRequiresDistributedLock(t *testing.T) {
	a := fixedKiroAccount()
	a.Status = StatusActive
	repo := &kiroAdminRefreshRepo{account: a}
	upstream := &kiroProxyUpstream{}
	svc := &AccountTestService{httpUpstream: upstream, claudeTokenProvider: &ClaudeTokenProvider{refreshAPI: NewOAuthRefreshAPI(repo, nil)}}
	_, err := svc.RefreshKiroAccount(context.Background(), a)
	require.ErrorContains(t, err, "distributed lock")
	require.Zero(t, upstream.calls)
	require.Zero(t, repo.writes)
}
func TestKiroNullableUpdateKeepsStoredSecretsAndIdentity(t *testing.T) {
	a := fixedKiroAccount()
	a.Credentials["client_id"] = "client"
	a.Credentials["client_secret"] = "original-secret"
	a.Credentials["auth_method"] = "idc"
	require.NoError(t, normalizeKiroAccount(a))
	before := kiro.EnvironmentKey(a.Credentials)
	incoming := map[string]any{"credentials": map[string]any{"refreshToken": nil, "clientId": nil, "clientSecret": nil, "machineId": nil, "authMethod": nil}}
	normalized, err := kiro.Normalize(mergeAccountCredentials(a, incoming))
	require.NoError(t, err)
	require.Equal(t, "test-refresh", normalized["refresh_token"])
	require.Equal(t, "original-secret", normalized["client_secret"])
	require.Equal(t, before, kiro.EnvironmentKey(normalized))
}
func TestKiroAdminErrorsDoNotExposeUnknownErrorText(t *testing.T) {
	require.NotContains(t, KiroAdminErrorMessage(errors.New("token=private-secret")), "private-secret")
	require.Contains(t, KiroAdminErrorMessage(&kiro.UpstreamError{Operation: kiro.OperationRefresh, Status: 401, Reason: "unauthorized"}), "credential refresh HTTP 401")
}
