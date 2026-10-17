package siwc

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStableSIWCHostConcurrentInitialization(t *testing.T) {
	dir := t.TempDir()
	var wg sync.WaitGroup
	ids := make(chan string, 16)
	for range 16 {
		wg.Go(func() {
			id, err := StableHostID(dir, "")
			if err != nil {
				t.Error(err)
				return
			}
			ids <- id
		})
	}
	wg.Wait()
	close(ids)
	first := ""
	for id := range ids {
		if first == "" {
			first = id
		}
		require.Equal(t, first, id)
	}
	require.NotEmpty(t, first)
	info, err := os.Stat(filepath.Join(dir, "host-id"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())
	id, err := StableHostID(dir, "urn:uuid:12121212-1212-4212-8212-121212121212")
	require.NoError(t, err)
	require.Equal(t, first, id)
}

func TestSIWCRegistrationPersistsIssuedClientWithoutCredentials(t *testing.T) {
	dir := t.TempDir()
	value := Registration{ClientID: "oaiapp_saved", HostID: "urn:uuid:12121212-1212-4212-8212-121212121212", CreatedAt: time.Now()}
	require.NoError(t, SaveRegistration(dir, "opaque-session", value))
	loaded, err := LoadRegistration(dir, "opaque-session")
	require.NoError(t, err)
	require.Equal(t, value.ClientID, loaded.ClientID)
	_, err = LoadRegistration(dir, "wrong-session")
	require.Error(t, err)
	value.CreatedAt = time.Now().Add(-25 * time.Hour)
	require.NoError(t, SaveRegistration(dir, "expired-session", value))
	_, err = LoadRegistration(dir, "expired-session")
	require.Error(t, err)
}
