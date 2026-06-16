//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type redeemRateLimitCacheStub struct {
	incrementUserID int64
	incrementWindow time.Duration
	incrementCalls  int

	deleteUserID int64
	deleteCalls  int
}

func (s *redeemRateLimitCacheStub) GetRedeemAttemptCount(ctx context.Context, userID int64) (int, error) {
	return 0, nil
}

func (s *redeemRateLimitCacheStub) IncrementRedeemAttemptCount(ctx context.Context, userID int64, window time.Duration) error {
	s.incrementUserID = userID
	s.incrementWindow = window
	s.incrementCalls++
	return nil
}

func (s *redeemRateLimitCacheStub) DeleteRedeemAttemptCount(ctx context.Context, userID int64) error {
	s.deleteUserID = userID
	s.deleteCalls++
	return nil
}

func (s *redeemRateLimitCacheStub) AcquireRedeemLock(ctx context.Context, code string, ttl time.Duration) (bool, error) {
	return true, nil
}

func (s *redeemRateLimitCacheStub) ReleaseRedeemLock(ctx context.Context, code string) error {
	return nil
}

func TestRedeemService_IncrementRedeemErrorCountUsesFixedWindow(t *testing.T) {
	cache := &redeemRateLimitCacheStub{}
	svc := &RedeemService{cache: cache}

	svc.incrementRedeemErrorCount(context.Background(), 12)

	require.Equal(t, 1, cache.incrementCalls)
	require.Equal(t, int64(12), cache.incrementUserID)
	require.Equal(t, redeemRateLimitDuration, cache.incrementWindow)
}

func TestRedeemService_ResetRedeemErrorCount(t *testing.T) {
	cache := &redeemRateLimitCacheStub{}
	svc := &RedeemService{cache: cache}

	svc.resetRedeemErrorCount(context.Background(), 12)

	require.Equal(t, 1, cache.deleteCalls)
	require.Equal(t, int64(12), cache.deleteUserID)
}
