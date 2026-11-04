package service

import (
	"context"
	"errors"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type conversationBindingTestRepo struct {
	schedulerTestOpenAIAccountRepo
	mu       sync.Mutex
	bindings map[string]*OpenAIConversationBinding
	failure  error
}

func (r *conversationBindingTestRepo) GetOpenAIConversationBinding(_ context.Context, key string) (*OpenAIConversationBinding, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failure != nil {
		return nil, r.failure
	}
	if b := r.bindings[key]; b != nil {
		copy := *b
		return &copy, nil
	}
	return nil, nil
}
func (r *conversationBindingTestRepo) ClaimOpenAIConversationBinding(_ context.Context, key string, b *OpenAIConversationBinding) (*OpenAIConversationBinding, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failure != nil {
		return nil, r.failure
	}
	if r.bindings == nil {
		r.bindings = map[string]*OpenAIConversationBinding{}
	}
	if r.bindings[key] == nil {
		copy := *b
		r.bindings[key] = &copy
	}
	copy := *r.bindings[key]
	return &copy, nil
}
func conversationFixture() (*OpenAIGatewayService, *conversationBindingTestRepo) {
	repo := &conversationBindingTestRepo{schedulerTestOpenAIAccountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 2, Credentials: map[string]any{"chatgpt_account_id": "credential-one"}, Extra: map[string]any{"openai_device_id": "device-one"}},
		{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 2, Credentials: map[string]any{"chatgpt_account_id": "credential-two"}, Extra: map[string]any{"openai_device_id": "device-two"}},
	}}}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIConversationBindingEnabled = true
	return &OpenAIGatewayService{accountRepo: repo, cache: &schedulerTestGatewayCache{}, cfg: cfg}, repo
}
func conversationContext(s *OpenAIGatewayService, key int64, session, body string) context.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	c.Set("api_key", &APIKey{ID: key})
	return s.WithOpenAIConversationBinding(c.Request.Context(), c, nil, session, "", []byte(body))
}
func TestConversationBindingConcurrentFirstTurnsElectOneIdentity(t *testing.T) {
	s, r := conversationFixture()
	var wg sync.WaitGroup
	results := make(chan int64, 40)
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			a := &r.accounts[i%2]
			ctx := conversationContext(s, 7, "same-session", `{"input":"hello"}`)
			if err := s.claimSelectedOpenAIConversation(ctx, &AccountSelectionResult{Account: a}); err == nil {
				results <- a.ID
			}
		}(i)
	}
	wg.Wait()
	close(results)
	var winner int64
	for id := range results {
		if winner == 0 {
			winner = id
		}
		require.Equal(t, winner, id)
	}
	require.NotZero(t, winner)
	require.Len(t, r.bindings, 1)
}
func TestConversationBindingSurvivesCacheLossAndRejectsFailover(t *testing.T) {
	s, r := conversationFixture()
	first := conversationContext(s, 7, "stable", `{"input":"hello"}`)
	require.NoError(t, s.claimSelectedOpenAIConversation(first, &AccountSelectionResult{Account: &r.accounts[0]}))
	s.cache = &schedulerTestGatewayCache{}
	ctx := conversationContext(s, 7, "stable", `{"input":[{"role":"assistant","content":"answer"}]}`)
	req := OpenAIAccountScheduleRequest{Platform: PlatformOpenAI, SessionHash: "stable", RequiredTransport: OpenAIUpstreamTransportHTTPSSE, ExcludedIDs: map[int64]struct{}{1: {}}}
	selection, _, handled, err := s.selectBoundOpenAIConversation(ctx, req)
	require.True(t, handled)
	require.Nil(t, selection)
	require.Error(t, err)
	require.Len(t, r.bindings, 1)
	req.ExcludedIDs = nil
	selected, decision, handled, err := s.selectBoundOpenAIConversation(ctx, req)
	require.True(t, handled)
	require.NoError(t, err)
	require.Equal(t, int64(1), selected.Account.ID)
	require.Equal(t, "conversation_binding", decision.Layer)
	if selected.ReleaseFunc != nil {
		selected.ReleaseFunc()
	}
	r.accounts[0].Schedulable = false
	_, _, handled, err = s.selectBoundOpenAIConversation(ctx, req)
	require.True(t, handled)
	require.Error(t, err)
	r.accounts[0].Schedulable = true
	r.accounts[0].Extra["openai_device_id"] = "changed"
	_, _, handled, err = s.selectBoundOpenAIConversation(ctx, req)
	require.True(t, handled)
	require.Error(t, err)
}
func TestConversationBindingScopesKeysAndRejectsUnknownHistory(t *testing.T) {
	s, r := conversationFixture()
	for _, key := range []int64{7, 8} {
		ctx := conversationContext(s, key, "same", `{"input":"hello"}`)
		require.NoError(t, s.claimSelectedOpenAIConversation(ctx, &AccountSelectionResult{Account: &r.accounts[int(key)%2]}))
	}
	require.Len(t, r.bindings, 2)
	ctx := conversationContext(s, 7, "missing", `{"input":[{"type":"function_call_output","call_id":"c","output":"x"}]}`)
	err := s.claimSelectedOpenAIConversation(ctx, &AccountSelectionResult{Account: &r.accounts[0]})
	var bindingErr *OpenAIConversationError
	require.ErrorAs(t, err, &bindingErr)
	require.Equal(t, "conversation_binding_unknown", bindingErr.Code)
	r.failure = errors.New("database down")
	_, _, handled, err := s.selectBoundOpenAIConversation(conversationContext(s, 7, "same", `{}`), OpenAIAccountScheduleRequest{Platform: PlatformOpenAI})
	require.True(t, handled)
	require.ErrorAs(t, err, &bindingErr)
	require.Equal(t, "conversation_binding_unavailable", bindingErr.Code)
}
func TestConversationBindingShadowSharesCredentialAndProtectsOutbound(t *testing.T) {
	s, r := conversationFixture()
	parent := int64(1)
	shadow := Account{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeOAuth, ParentAccountID: &parent}
	r.accounts = append(r.accounts, shadow)
	ctx := conversationContext(s, 7, "s", `{"input":"hello"}`)
	require.NoError(t, s.claimSelectedOpenAIConversation(ctx, &AccountSelectionResult{Account: &shadow}))
	b := openAIConversationStateFromContext(ctx).binding
	require.Equal(t, parent, b.CredentialAccountID)
	require.NoError(t, s.validateOpenAIConversationIdentity(ctx, &shadow, &r.accounts[0]))
	require.Error(t, s.validateOpenAIConversationIdentity(ctx, &shadow, &r.accounts[1]))
	require.NoError(t, s.claimSelectedOpenAIConversation(ctx, &AccountSelectionResult{Account: &r.accounts[0]}))
}
func TestConversationBindingLeavesAPIKeysAndOtherPlatformsUnchanged(t *testing.T) {
	s, _ := conversationFixture()
	ctx := conversationContext(s, 7, "", `{"messages":[{"role":"assistant","content":"a"}]}`)
	require.NoError(t, s.claimSelectedOpenAIConversation(ctx, &AccountSelectionResult{Account: &Account{ID: 4, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}}))
	_, _, handled, err := s.selectBoundOpenAIConversation(ctx, OpenAIAccountScheduleRequest{Platform: PlatformGrok})
	require.False(t, handled)
	require.NoError(t, err)
}

func TestConversationBindingProtocolSwitchUsesDurableAccount(t *testing.T) {
	s, r := conversationFixture()
	// Both listeners share the persistent repository while each has its own cache.
	r.accounts[0].Extra["openai_oauth_responses_websockets_v2_mode"] = "ctx_pool"
	s.cfg.Gateway.OpenAIWS.Enabled = true
	s.cfg.Gateway.OpenAIWS.OAuthEnabled = true
	s.cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	s.cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	ctx := conversationContext(s, 7, "shared-session", `{"input":"hello"}`)
	require.NoError(t, s.claimSelectedOpenAIConversation(ctx, &AccountSelectionResult{Account: &r.accounts[0]}))
	other := &OpenAIGatewayService{accountRepo: r, cache: &schedulerTestGatewayCache{}, cfg: s.cfg}
	next := conversationContext(other, 7, "shared-session", `{"input":"continue"}`)
	selected, _, handled, err := other.selectBoundOpenAIConversation(next, OpenAIAccountScheduleRequest{Platform: PlatformOpenAI, SessionHash: "shared-session", RequiredTransport: OpenAIUpstreamTransportResponsesWebsocketV2Ingress})
	require.True(t, handled)
	require.NoError(t, err)
	require.Equal(t, int64(1), selected.Account.ID)
	if selected.ReleaseFunc != nil {
		selected.ReleaseFunc()
	}
}

func TestConversationResponseAliasSurvivesRedisExpiryAndRejectsOtherKey(t *testing.T) {
	s, r := conversationFixture()
	first := conversationContext(s, 7, "first", `{"input":"hello"}`)
	require.NoError(t, s.claimSelectedOpenAIConversation(first, &AccountSelectionResult{Account: &r.accounts[0]}))
	s.bindOpenAIConversationResponse(first, 0, "resp_durable", &r.accounts[0])
	s.cache = &schedulerTestGatewayCache{}
	next := conversationContext(s, 7, "next", `{"previous_response_id":"resp_durable"}`)
	openAIConversationStateFromContext(next).previousResponseID = "resp_durable"
	selected, decision, handled, err := s.selectBoundOpenAIConversation(next, OpenAIAccountScheduleRequest{Platform: PlatformOpenAI, SessionHash: "next", PreviousResponseID: "resp_durable", RequiredTransport: OpenAIUpstreamTransportHTTPSSE})
	require.True(t, handled)
	require.NoError(t, err)
	require.Equal(t, int64(1), selected.Account.ID)
	require.True(t, decision.StickyPreviousHit)
	if selected.ReleaseFunc != nil {
		selected.ReleaseFunc()
	}
	alien := conversationContext(s, 8, "other", `{"previous_response_id":"resp_durable"}`)
	openAIConversationStateFromContext(alien).previousResponseID = "resp_durable"
	_, _, _, err = s.selectBoundOpenAIConversation(alien, OpenAIAccountScheduleRequest{Platform: PlatformOpenAI, SessionHash: "other", PreviousResponseID: "resp_durable"})
	require.NoError(t, err)
	require.Error(t, s.claimSelectedOpenAIConversation(alien, &AccountSelectionResult{Account: &r.accounts[1]}))
}

func TestConversationMetadataSessionStableAcrossProtocols(t *testing.T) {
	s, _ := conversationFixture()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	httpHash := s.GenerateSessionHash(c, []byte(`{"client_metadata":{"session_id":"stable-thread"},"input":"first"}`))
	wsHash := s.GenerateSessionHash(c, []byte(`{"type":"response.create","response":{"client_metadata":{"session_id":"stable-thread"},"input":"second"}}`))
	require.NotEmpty(t, httpHash)
	require.Equal(t, httpHash, wsHash)
	c.Request.Header.Set("thread-id", "stable-thread")
	require.Equal(t, httpHash, s.GenerateSessionHash(c, []byte(`{"input":"third"}`)))
}
