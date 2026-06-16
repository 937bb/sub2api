package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestOpenAIOAuthRedisSessionStore_SharesSessionsAcrossInstances(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	ctx := context.Background()
	storeA := newOpenAIOAuthSessionStore(rdb)
	storeB := newOpenAIOAuthSessionStore(rdb)
	t.Cleanup(storeA.Stop)
	t.Cleanup(storeB.Stop)

	session := &openai.OAuthSession{
		State:        "state-1",
		CodeVerifier: "verifier-1",
		ClientID:     openai.ClientID,
		RedirectURI:  openai.DefaultRedirectURI,
		ProxyURL:     "http://proxy.example",
		CreatedAt:    time.Now(),
	}
	require.NoError(t, storeA.Set(ctx, "session-1", session))

	loaded, ok := storeB.Get(ctx, "session-1")
	require.True(t, ok)
	require.Equal(t, session.State, loaded.State)
	require.Equal(t, session.CodeVerifier, loaded.CodeVerifier)
	require.Equal(t, session.ProxyURL, loaded.ProxyURL)

	storeB.Delete(ctx, "session-1")
	_, ok = storeA.Get(ctx, "session-1")
	require.False(t, ok, "delete in Redis must not be resurrected by another instance's memory fallback")
}

func TestOpenAIOAuthRedisSessionStore_DoesNotFallbackToStaleMemoryAfterRedisDelete(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	ctx := context.Background()
	storeA := newOpenAIOAuthSessionStore(rdb)
	storeB := newOpenAIOAuthSessionStore(rdb)
	t.Cleanup(storeA.Stop)
	t.Cleanup(storeB.Stop)

	session := &openai.OAuthSession{
		State:        "state-stale",
		CodeVerifier: "verifier-stale",
		ClientID:     openai.ClientID,
		RedirectURI:  openai.DefaultRedirectURI,
		CreatedAt:    time.Now(),
	}
	require.NoError(t, storeA.Set(ctx, "session-stale", session))
	loaded, ok := storeA.Get(ctx, "session-stale")
	require.True(t, ok)
	require.Equal(t, session.State, loaded.State)

	storeB.Delete(ctx, "session-stale")
	mr.Close()

	_, ok = storeA.Get(ctx, "session-stale")
	require.False(t, ok, "a session that reached Redis must not fall back to stale memory after another instance deletes it")
}

func TestOpenAIOAuthRedisSessionStore_FallsBackToMemoryWhenRedisWriteFails(t *testing.T) {
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

	ctx := context.Background()
	store := newOpenAIOAuthSessionStore(rdb)
	t.Cleanup(store.Stop)

	session := &openai.OAuthSession{
		State:        "state-2",
		CodeVerifier: "verifier-2",
		ClientID:     openai.ClientID,
		RedirectURI:  openai.DefaultRedirectURI,
		CreatedAt:    time.Now(),
	}
	require.NoError(t, store.Set(ctx, "session-2", session))

	loaded, ok := store.Get(ctx, "session-2")
	require.True(t, ok)
	require.Equal(t, session.State, loaded.State)
	require.Equal(t, session.CodeVerifier, loaded.CodeVerifier)
}

func TestOpenAIOAuthRedisSessionStore_RejectsExpiredSession(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	ctx := context.Background()
	store := newOpenAIOAuthSessionStore(rdb)
	t.Cleanup(store.Stop)

	session := &openai.OAuthSession{
		State:        "state-expired",
		CodeVerifier: "verifier-expired",
		RedirectURI:  openai.DefaultRedirectURI,
		CreatedAt:    time.Now().Add(-openai.SessionTTL - time.Minute),
	}
	require.NoError(t, store.Set(ctx, "session-expired", session))

	_, ok := store.Get(ctx, "session-expired")
	require.False(t, ok)
	require.False(t, mr.Exists(openAIOAuthSessionKey("session-expired")))
}
