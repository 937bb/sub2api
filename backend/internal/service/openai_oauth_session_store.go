package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/redis/go-redis/v9"
)

const openAIOAuthSessionKeyPrefix = "openai:oauth:session:"

type openAIOAuthSessionStore interface {
	Set(ctx context.Context, sessionID string, session *openai.OAuthSession) error
	Get(ctx context.Context, sessionID string) (*openai.OAuthSession, bool)
	Delete(ctx context.Context, sessionID string)
	Stop()
}

type openAIOAuthMemorySessionStore struct {
	store *openai.SessionStore
}

func newOpenAIOAuthMemorySessionStore() *openAIOAuthMemorySessionStore {
	return &openAIOAuthMemorySessionStore{store: openai.NewSessionStore()}
}

func (s *openAIOAuthMemorySessionStore) Set(_ context.Context, sessionID string, session *openai.OAuthSession) error {
	if s == nil || s.store == nil || strings.TrimSpace(sessionID) == "" || session == nil {
		return nil
	}
	s.store.Set(sessionID, session)
	return nil
}

func (s *openAIOAuthMemorySessionStore) Get(_ context.Context, sessionID string) (*openai.OAuthSession, bool) {
	if s == nil || s.store == nil || strings.TrimSpace(sessionID) == "" {
		return nil, false
	}
	return s.store.Get(sessionID)
}

func (s *openAIOAuthMemorySessionStore) Delete(_ context.Context, sessionID string) {
	if s == nil || s.store == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	s.store.Delete(sessionID)
}

func (s *openAIOAuthMemorySessionStore) Stop() {
	if s == nil || s.store == nil {
		return
	}
	s.store.Stop()
}

// NewOpenAIOAuthServiceWithRedis creates the OAuth service with Redis-backed pending session storage.
func NewOpenAIOAuthServiceWithRedis(proxyRepo ProxyRepository, oauthClient OpenAIOAuthClient, redisClient *redis.Client) *OpenAIOAuthService {
	return newOpenAIOAuthServiceWithSessionStore(proxyRepo, oauthClient, newOpenAIOAuthSessionStore(redisClient))
}

func newOpenAIOAuthSessionStore(redisClient *redis.Client) openAIOAuthSessionStore {
	memory := newOpenAIOAuthMemorySessionStore()
	if redisClient == nil {
		return memory
	}
	store := &openAIOAuthRedisSessionStore{
		memory: memory,
		rdb:    redisClient,
		stopCh: make(chan struct{}),
	}
	go store.cleanupRedisSetFailures()
	return store
}

type openAIOAuthRedisSessionStore struct {
	memory *openAIOAuthMemorySessionStore
	rdb    *redis.Client

	// Session IDs whose Redis write failed may still be served from memory on this instance.
	redisSetFailures sync.Map
	stopOnce         sync.Once
	stopCh           chan struct{}
}

type openAIOAuthRedisSetFailure struct {
	expiresAt time.Time
}

func (s *openAIOAuthRedisSessionStore) Set(ctx context.Context, sessionID string, session *openai.OAuthSession) error {
	if s == nil {
		return nil
	}
	if err := s.memory.Set(ctx, sessionID, session); err != nil {
		return err
	}
	if s.rdb == nil || strings.TrimSpace(sessionID) == "" || session == nil {
		return nil
	}

	payload, err := json.Marshal(session)
	if err != nil {
		return err
	}
	if err := s.rdb.Set(openAIOAuthSessionContext(ctx), openAIOAuthSessionKey(sessionID), payload, openai.SessionTTL).Err(); err != nil {
		s.redisSetFailures.Store(sessionID, openAIOAuthRedisSetFailure{expiresAt: session.CreatedAt.Add(openai.SessionTTL)})
		slog.Warn("openai_oauth_session_redis_set_failed", "error", err)
		return nil
	}
	s.redisSetFailures.Delete(sessionID)
	return nil
}

func (s *openAIOAuthRedisSessionStore) Get(ctx context.Context, sessionID string) (*openai.OAuthSession, bool) {
	if s == nil || strings.TrimSpace(sessionID) == "" {
		return nil, false
	}
	if s.rdb == nil {
		return s.memory.Get(ctx, sessionID)
	}

	session, status := s.getFromRedis(ctx, sessionID)
	switch status {
	case openAIOAuthSessionRedisHit:
		return session, true
	case openAIOAuthSessionRedisExpired:
		s.Delete(ctx, sessionID)
		return nil, false
	case openAIOAuthSessionRedisMiss:
		// If Redis was writable for this session, a miss is authoritative and prevents
		// another instance's successful exchange/delete from being resurrected by memory.
		if !s.canUseMemoryFallback(sessionID) {
			return nil, false
		}
	case openAIOAuthSessionRedisError:
		if !s.canUseMemoryFallback(sessionID) {
			return nil, false
		}
	}

	return s.memory.Get(ctx, sessionID)
}

func (s *openAIOAuthRedisSessionStore) Delete(_ context.Context, sessionID string) {
	if s == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	s.memory.Delete(context.Background(), sessionID)
	s.redisSetFailures.Delete(sessionID)
	if s.rdb == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.rdb.Del(ctx, openAIOAuthSessionKey(sessionID)).Err(); err != nil {
		slog.Warn("openai_oauth_session_redis_delete_failed", "error", err)
	}
}

func (s *openAIOAuthRedisSessionStore) Stop() {
	if s == nil {
		return
	}
	s.memory.Stop()
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
}

func (s *openAIOAuthRedisSessionStore) cleanupRedisSetFailures() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.deleteExpiredRedisSetFailures(time.Now())
		}
	}
}

func (s *openAIOAuthRedisSessionStore) deleteExpiredRedisSetFailures(now time.Time) {
	s.redisSetFailures.Range(func(key, value any) bool {
		marker, ok := value.(openAIOAuthRedisSetFailure)
		if !ok || now.After(marker.expiresAt) {
			s.redisSetFailures.Delete(key)
		}
		return true
	})
}

func (s *openAIOAuthRedisSessionStore) canUseMemoryFallback(sessionID string) bool {
	failure, ok := s.redisSetFailures.Load(sessionID)
	if !ok {
		return false
	}
	marker, ok := failure.(openAIOAuthRedisSetFailure)
	if !ok || time.Now().After(marker.expiresAt) {
		s.redisSetFailures.Delete(sessionID)
		return false
	}
	return true
}

type openAIOAuthSessionRedisStatus int

const (
	openAIOAuthSessionRedisMiss openAIOAuthSessionRedisStatus = iota
	openAIOAuthSessionRedisHit
	openAIOAuthSessionRedisExpired
	openAIOAuthSessionRedisError
)

func (s *openAIOAuthRedisSessionStore) getFromRedis(ctx context.Context, sessionID string) (*openai.OAuthSession, openAIOAuthSessionRedisStatus) {
	payload, err := s.rdb.Get(openAIOAuthSessionContext(ctx), openAIOAuthSessionKey(sessionID)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, openAIOAuthSessionRedisMiss
	}
	if err != nil {
		slog.Warn("openai_oauth_session_redis_get_failed", "error", err)
		return nil, openAIOAuthSessionRedisError
	}

	var session openai.OAuthSession
	if err := json.Unmarshal(payload, &session); err != nil {
		slog.Warn("openai_oauth_session_redis_decode_failed", "error", err)
		s.Delete(ctx, sessionID)
		return nil, openAIOAuthSessionRedisError
	}
	if openAIOAuthSessionExpired(&session) {
		return nil, openAIOAuthSessionRedisExpired
	}
	return &session, openAIOAuthSessionRedisHit
}

func openAIOAuthSessionKey(sessionID string) string {
	return openAIOAuthSessionKeyPrefix + sessionID
}

func openAIOAuthSessionExpired(session *openai.OAuthSession) bool {
	return session == nil || time.Since(session.CreatedAt) > openai.SessionTTL
}

func openAIOAuthSessionContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
