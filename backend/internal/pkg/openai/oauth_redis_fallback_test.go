package openai

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func openAIOAuthRedisTestSession(state string) *OAuthSession {
	return &OAuthSession{
		State:        state,
		CodeVerifier: "verifier-" + state,
		ClientID:     ClientID,
		ProxyURL:     "http://proxy.example",
		RedirectURI:  DefaultRedirectURI,
		CreatedAt:    time.Now(),
	}
}

func TestRedisSessionStoreSharesOpenAIOAuthSessionsAcrossInstances(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	storeA := NewRedisSessionStore(rdb)
	storeB := NewRedisSessionStore(rdb)
	t.Cleanup(storeA.Stop)
	t.Cleanup(storeB.Stop)

	want := openAIOAuthRedisTestSession("shared")
	storeA.Set("session-shared", want)

	got, ok := storeB.Get("session-shared")
	require.True(t, ok)
	require.Equal(t, want.State, got.State)
	require.Equal(t, want.CodeVerifier, got.CodeVerifier)
	require.Equal(t, want.ProxyURL, got.ProxyURL)

	storeB.Delete("session-shared")
	_, ok = storeA.Get("session-shared")
	require.False(t, ok, "a Redis delete must not resurrect another replica's memory copy")
}

func TestRedisSessionStoreFallsBackOnlyAfterOpenAIOAuthRedisWriteFailure(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr:            "127.0.0.1:0",
		DialTimeout:     50 * time.Millisecond,
		ReadTimeout:     50 * time.Millisecond,
		WriteTimeout:    50 * time.Millisecond,
		MaxRetries:      0,
		MinRetryBackoff: -1,
		MaxRetryBackoff: -1,
	})
	t.Cleanup(func() { _ = rdb.Close() })

	store := NewRedisSessionStore(rdb)
	t.Cleanup(store.Stop)
	want := openAIOAuthRedisTestSession("fallback")
	store.Set("session-fallback", want)

	got, ok := store.Get("session-fallback")
	require.True(t, ok)
	require.Equal(t, want.State, got.State)
}

func TestRedisSessionStoreRejectsExpiredOpenAIOAuthSession(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	store := NewRedisSessionStore(rdb)
	t.Cleanup(store.Stop)
	expired := openAIOAuthRedisTestSession("expired")
	expired.CreatedAt = time.Now().Add(-SessionTTL - time.Minute)
	store.Set("session-expired", expired)

	_, ok := store.Get("session-expired")
	require.False(t, ok)
}
