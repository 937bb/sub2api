package service

import (
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

type processRecoveryTestCache struct {
	ConcurrencyCache
	owners []string
	fail   bool
}

func (c *processRecoveryTestCache) CleanupProcessSlots(_ context.Context, p string) error {
	if c.fail {
		return errors.New("redis unavailable")
	}
	c.owners = append(c.owners, p)
	return nil
}
func TestProcessRecoveryPreservesLiveOwnerAndRecoversRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "owner")
	require.NoError(t, os.WriteFile(path, []byte("rdead\n"), 0600))
	cache := &processRecoveryTestCache{}
	first := NewConcurrencyService(cache)
	require.NoError(t, first.recoverProcessSlots(path))
	require.Equal(t, []string{"rdead"}, cache.owners)
	second := NewConcurrencyService(cache)
	require.ErrorContains(t, second.recoverProcessSlots(path), "still running")
	require.Len(t, cache.owners, 1)
	require.NoError(t, first.processOwnerLock.Close())
	require.NoError(t, second.recoverProcessSlots(path))
	defer second.processOwnerLock.Close()
	require.Equal(t, []string{"rdead", RequestIDPrefix()}, cache.owners)
}
func TestProcessRecoveryKeepsEvidenceOnRedisFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "owner")
	require.NoError(t, os.WriteFile(path, []byte("rdead"), 0600))
	cache := &processRecoveryTestCache{fail: true}
	svc := NewConcurrencyService(cache)
	require.Error(t, svc.recoverProcessSlots(path))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "rdead", string(data))
	cache.fail = false
	require.NoError(t, svc.recoverProcessSlots(path))
	defer svc.processOwnerLock.Close()
}
