//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/siwc"
	"github.com/stretchr/testify/require"
)

type siwcFailureCASRepo struct {
	*refreshAPIAccountRepo
	casCalls  int
	beforeCAS func()
}

func (r *siwcFailureCASRepo) SetOpenAISiwcRefreshErrorIfUnchanged(_ context.Context, expected *Account) (bool, error) {
	r.casCalls++
	if r.beforeCAS != nil {
		r.beforeCAS()
	}
	if r.account.GetCredential("refresh_token") != expected.GetCredential("refresh_token") {
		return false, nil
	}
	current := snapshotOAuthRefreshAccount(r.account)
	current.Status, current.ErrorMessage = StatusError, SIWCTerminalRefreshErrorMessage
	r.account = current
	return true, nil
}

func TestSIWCTerminalRefreshQuarantinesOnlyFailedGrantAndAllowsFailover(t *testing.T) {
	for _, reauthorized := range []bool{false, true} {
		account := siwcFixtureAccount()
		account.Credentials["subject"] = "subject"
		account.Credentials["refresh_token"] = "old-refresh"
		account.Credentials["expires_at"] = time.Now().Add(-time.Minute).Format(time.RFC3339)
		repo := &siwcFailureCASRepo{refreshAPIAccountRepo: &refreshAPIAccountRepo{account: snapshotOAuthRefreshAccount(account)}}
		if reauthorized {
			repo.beforeCAS = func() {
				current := snapshotOAuthRefreshAccount(repo.account)
				current.Credentials["refresh_token"] = "new-refresh"
				repo.account = current
			}
		}
		provider := NewOpenAITokenProvider(repo, nil, nil)
		provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, nil), &refreshAPIExecutorStub{needsRefresh: true, err: &siwc.AuthorizationError{Code: "invalid_grant"}})
		gateway := newOpenAIRejectedFieldTestService(nil)
		gateway.openAITokenProvider = provider
		token, _, err := gateway.GetAccessToken(context.Background(), account)
		require.Empty(t, token)
		var failure *UpstreamFailoverError
		require.ErrorAs(t, err, &failure)
		require.True(t, failure.ShouldRetryNextAccount())
		require.False(t, failure.RetryableOnSameAccount)
		require.Equal(t, 503, failure.ClientStatusCode)
		require.Equal(t, 1, repo.casCalls)
		require.True(t, repo.account.Schedulable, "manual scheduling preference must be preserved")
		if reauthorized {
			require.Equal(t, StatusActive, repo.account.Status)
			require.Equal(t, "new-refresh", repo.account.GetCredential("refresh_token"))
		} else {
			require.Equal(t, StatusError, repo.account.Status)
		}
	}
}

func TestSIWCTransientRefreshPreservesStillValidToken(t *testing.T) {
	account := siwcFixtureAccount()
	account.Credentials["subject"] = "subject"
	account.Credentials["refresh_token"] = "refresh"
	account.Credentials["expires_at"] = time.Now().Add(time.Minute).Format(time.RFC3339)
	repo := &siwcFailureCASRepo{refreshAPIAccountRepo: &refreshAPIAccountRepo{account: account}}
	provider := NewOpenAITokenProvider(repo, nil, nil)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, nil), &refreshAPIExecutorStub{needsRefresh: true, err: errors.New("network temporarily unavailable")})
	token, err := provider.GetAccessToken(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "upstream-token", token)
	require.Zero(t, repo.casCalls)
}

func TestSIWCBackgroundTerminalRefreshUsesConditionalQuarantine(t *testing.T) {
	for _, unified := range []bool{false, true} {
		for _, reauthorized := range []bool{false, true} {
			account := siwcFixtureAccount()
			account.Credentials["refresh_token"] = "failed-refresh"
			repo := &siwcFailureCASRepo{refreshAPIAccountRepo: &refreshAPIAccountRepo{account: snapshotOAuthRefreshAccount(account)}}
			if reauthorized {
				repo.beforeCAS = func() {
					current := snapshotOAuthRefreshAccount(repo.account)
					current.Credentials["refresh_token"] = "reauthorized-refresh"
					repo.account = current
				}
			}
			svc := NewTokenRefreshService(repo, nil, nil, nil, nil, nil, nil,
				&config.Config{TokenRefresh: config.TokenRefreshConfig{MaxRetries: 1}}, nil)
			if unified {
				svc.SetRefreshAPI(NewOAuthRefreshAPI(repo, nil))
			}
			failure := &siwc.AuthorizationError{Code: "invalid_refresh_token"}
			refresher := &siwcReauthorizedRefreshFailure{
				tokenRefresherStub: tokenRefresherStub{err: failure}, reauthorize: func() {},
			}
			executor := &refreshAPIExecutorStub{needsRefresh: true, err: failure}
			err := svc.refreshWithRetry(context.Background(), account, refresher, executor, time.Hour)
			require.ErrorIs(t, err, failure)
			require.Equal(t, 1, repo.casCalls)
			require.True(t, repo.account.Schedulable)
			if reauthorized {
				require.Equal(t, StatusActive, repo.account.Status)
				require.Equal(t, "reauthorized-refresh", repo.account.GetCredential("refresh_token"))
			} else {
				require.Equal(t, StatusError, repo.account.Status)
			}
		}
	}
}
