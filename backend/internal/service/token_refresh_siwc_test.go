//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type siwcReauthorizedRefreshFailure struct {
	tokenRefresherStub
	reauthorize func()
}

func (r *siwcReauthorizedRefreshFailure) Refresh(context.Context, *Account) (map[string]any, error) {
	r.calls++
	r.reauthorize()
	return nil, r.err
}

func TestSIWCBackgroundRefreshFailurePreservesReauthorizedGrant(t *testing.T) {
	t.Parallel()
	for _, unified := range []bool{false, true} {
		path := "legacy"
		if unified {
			path = "unified"
		}
		for _, failure := range []string{"invalid_refresh_token", "SIWC authorization network request failed"} {
			t.Run(path+"/"+failure, func(t *testing.T) {
				account := &Account{
					ID: 901, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true,
					Credentials: map[string]any{"auth_mode": "siwc", "access_token": "old-access", "refresh_token": "old-refresh"},
				}
				repo := &tokenRefreshAccountRepo{snapshotReads: true}
				repo.accountsByID = map[int64]*Account{account.ID: snapshotOAuthRefreshAccount(account)}
				invalidator := &tokenCacheInvalidatorStub{}
				blocker := &tokenRefreshRuntimeBlocker{}
				svc := NewTokenRefreshService(repo, nil, nil, nil, nil, invalidator, nil,
					&config.Config{TokenRefresh: config.TokenRefreshConfig{MaxRetries: 1}}, nil)
				svc.SetAccountRuntimeBlocker(blocker)
				if unified {
					svc.SetRefreshAPI(NewOAuthRefreshAPI(repo, nil))
				}
				refresher := &siwcReauthorizedRefreshFailure{
					tokenRefresherStub: tokenRefresherStub{err: errors.New(failure)},
					reauthorize: func() {
						current := snapshotOAuthRefreshAccount(repo.accountsByID[account.ID])
						current.Credentials["access_token"] = "reauthorized-access"
						current.Credentials["refresh_token"] = "reauthorized-refresh"
						repo.accountsByID[account.ID] = current
					},
				}

				err := svc.refreshWithRetry(context.Background(), account, refresher, refresher, time.Hour)

				require.ErrorIs(t, err, refresher.err)
				require.Equal(t, 1, refresher.calls)
				require.Zero(t, repo.setErrorCalls)
				require.Zero(t, repo.setTempUnschedCalls)
				require.Zero(t, repo.updateCalls)
				require.Zero(t, blocker.blockCalls)
				require.Zero(t, invalidator.calls)
				current := repo.accountsByID[account.ID]
				require.Equal(t, StatusActive, current.Status)
				require.True(t, current.Schedulable)
				require.Nil(t, current.TempUnschedulableUntil)
				require.Equal(t, "reauthorized-refresh", current.GetCredential("refresh_token"))
			})
		}
	}
}
