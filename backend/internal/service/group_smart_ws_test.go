package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type smartWSSettingsStub struct {
	SettingRepository
	mu    sync.Mutex
	value *string
	reads int
	err   error
}

type smartWSGroupReader map[int64]*Group

func (r smartWSGroupReader) GetByID(_ context.Context, id int64) (*Group, error) {
	if g := r[id]; g != nil {
		return g, nil
	}
	return nil, ErrGroupNotFound
}

func (r *smartWSSettingsStub) GetValue(context.Context, string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reads++
	if r.err != nil {
		return "", r.err
	}
	if r.value == nil {
		return "", ErrSettingNotFound
	}
	return *r.value, nil
}

func (r *smartWSSettingsStub) CompareAndSwapGroupSmartWS(_ context.Context, _ int64, _ string, previous *string, next string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if (previous == nil) != (r.value == nil) || (previous != nil && *previous != *r.value) {
		return false, nil
	}
	r.value = &next
	return true, nil
}

func TestGroupSmartWSPolicyConflictDisableAndUnavailable(t *testing.T) {
	ctx := context.Background()
	repo := &smartWSSettingsStub{}
	s := NewSettingService(repo, &config.Config{})
	s.SetDefaultSubscriptionGroupReader(smartWSGroupReader{
		7: {ID: 7, Platform: PlatformOpenAI}, 8: {ID: 8, Platform: PlatformAnthropic},
	})
	require.False(t, s.GroupSmartWSEnabled(ctx, 7))
	policy, err := s.UpdateGroupSmartWS(ctx, 7, true, 0)
	require.NoError(t, err)
	require.Equal(t, int64(1), policy.Revision)
	require.True(t, s.GroupSmartWSEnabled(ctx, 7))
	_, err = s.UpdateGroupSmartWS(ctx, 7, false, 0)
	require.ErrorIs(t, err, ErrGroupSmartWSConflict)
	require.True(t, s.GroupSmartWSEnabled(ctx, 7))
	_, err = s.UpdateGroupSmartWS(ctx, 8, true, 0)
	require.ErrorIs(t, err, ErrGroupSmartWSUnsupported)
	_, err = s.UpdateGroupSmartWS(ctx, 7, false, 1)
	require.NoError(t, err)
	require.False(t, s.GroupSmartWSEnabled(ctx, 7))
	_, err = s.UpdateGroupSmartWS(ctx, 7, true, 2)
	require.NoError(t, err)
	repo.err = errors.New("unavailable")
	s.groupSmartWSCache[7] = groupSmartWSCacheEntry{enabled: true, expires: time.Now().Add(-time.Second)}
	require.False(t, s.GroupSmartWSEnabled(ctx, 7), "expired enabled policy must not survive database failure")
}

func TestGroupSmartWSIdentityUsesAuthenticatedTenant(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("api_key", &APIKey{ID: 31})
	ctx := context.WithValue(context.Background(), ctxkey.Group, &Group{ID: 7, Platform: PlatformOpenAI, Hydrated: true, Status: StatusActive})
	ctx = context.WithValue(ctx, ctxkey.UserID, int64(11))
	ctx, _ = beginGroupSmartWSAttempt(ctx, c)
	id, scope := groupSmartWSIdentity(ctx)
	require.Equal(t, int64(7), id)
	require.Equal(t, "group:7/user:11/key:31", scope)
	_, anonymous := groupSmartWSIdentity(context.WithValue(ctx, ctxkey.UserID, int64(0)))
	require.Empty(t, anonymous)
	_, missingKey := groupSmartWSIdentity(context.WithValue(ctx, groupSmartWSAttemptKey{}, &groupSmartWSAttempt{}))
	require.Empty(t, missingKey)
}

func TestGroupSmartWSDispatchedAttemptCannotRetryInsideForward(t *testing.T) {
	attempt := &groupSmartWSAttempt{}
	attempt.sent.Store(true)
	ctx := context.WithValue(context.Background(), groupSmartWSAttemptKey{}, attempt)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://chatgpt.com/backend-api/codex/responses", strings.NewReader(`{"stream":true}`))
	require.NoError(t, err)
	// No HTTP client or plugin: touching either would panic instead of returning
	// the non-replayable error. Covers internal compact/model fallback loops.
	s := &OpenAIGatewayService{}
	response, err := s.doOpenAIUpstream(req, "", &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth})
	require.Nil(t, response)
	var sent *PluginTransportError
	require.ErrorAs(t, err, &sent)
	require.True(t, sent.RequestSent)
	require.Nil(t, finishGroupSmartWSAttempt(attempt, nil))
}

func TestGroupSmartWSAssignedProxyFailureCannotReachEitherTransport(t *testing.T) {
	proxyID := int64(17)
	req := httptest.NewRequest(http.MethodPost, "https://chatgpt.com/backend-api/codex/responses", nil)
	s := &OpenAIGatewayService{}
	_, err := s.doOpenAIUpstream(req, "", &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, ProxyID: &proxyID})
	require.ErrorContains(t, err, "unavailable")
}

func TestGroupSmartWSGlobalOAuthDisabledPreservesOriginalTransport(t *testing.T) {
	s := &OpenAIGatewayService{settingService: &SettingService{}, cfg: &config.Config{}}
	s.cfg.Gateway.OpenAIWS.Enabled = true
	a := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{"openai_oauth_responses_websockets_v2_enabled": true}}
	req := httptest.NewRequest(http.MethodPost, "https://chatgpt.com/backend-api/codex/responses", nil)
	resp, handled, err := s.tryGroupSmartWS(req, "", a)
	require.NoError(t, err)
	require.False(t, handled)
	require.Nil(t, resp)
}

type smartWSBlockedSettings struct {
	SettingRepository
	entered chan struct{}
	release chan struct{}
}

func (r *smartWSBlockedSettings) GetValue(ctx context.Context, key string) (string, error) {
	if key == groupSmartWSKey(7) {
		close(r.entered)
		select {
		case <-r.release:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return `{"enabled":true,"revision":1}`, nil
}
func TestGroupSmartWSPolicyReadsDoNotSerializeUnrelatedGroups(t *testing.T) {
	repo := &smartWSBlockedSettings{entered: make(chan struct{}), release: make(chan struct{})}
	s := NewSettingService(repo, &config.Config{})
	first := make(chan bool, 1)
	go func() { first <- s.GroupSmartWSEnabled(context.Background(), 7) }()
	<-repo.entered
	second := make(chan bool, 1)
	go func() { second <- s.GroupSmartWSEnabled(context.Background(), 8) }()
	select {
	case enabled := <-second:
		require.True(t, enabled)
	case <-time.After(200 * time.Millisecond):
		t.Error("another group's database read blocked the cache")
	}
	// Simulate invalidation after a settings write while the old read is in flight.
	s.groupSmartWSMu.Lock()
	s.groupSmartWSEpoch++
	s.groupSmartWSMu.Unlock()
	close(repo.release)
	require.False(t, <-first, "an invalidated read must not repopulate enabled policy")
}

func TestGroupSmartWSAccountOffAndHTTPBridgeOverrideLegacyEnabled(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	policy := `{"enabled":true,"revision":1}`
	repo := &smartWSSettingsStub{value: &policy}
	s := &OpenAIGatewayService{settingService: NewSettingService(repo, cfg), cfg: cfg}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("api_key", &APIKey{ID: 31})
	ctx := context.WithValue(context.Background(), ctxkey.Group, &Group{ID: 7, Platform: PlatformOpenAI, Hydrated: true, Status: StatusActive})
	ctx = context.WithValue(ctx, ctxkey.UserID, int64(11))
	ctx, _ = beginGroupSmartWSAttempt(ctx, c)
	before := groupSmartWSEligibilitySnapshot()["skip_account_mode"]
	for _, mode := range []string{OpenAIWSIngressModeOff, OpenAIWSIngressModeHTTPBridge} {
		a := &Account{Concurrency: 100, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{
			"openai_oauth_responses_websockets_v2_enabled": true,
			"openai_oauth_responses_websockets_v2_mode":    mode,
		}}
		req := httptest.NewRequest(http.MethodPost, "https://chatgpt.com/backend-api/codex/responses", nil).WithContext(ctx)
		_, handled, err := s.tryGroupSmartWS(req, "", a)
		require.NoError(t, err)
		require.False(t, handled)
		require.Zero(t, repo.reads, "disabled account must not reach group policy or upstream pool")
	}
	require.Equal(t, before+2, groupSmartWSEligibilitySnapshot()["skip_account_mode"])
}
