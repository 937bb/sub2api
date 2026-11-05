package service

import (
	"bytes"
	"context"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestKiroMixedGroupsRespectModelAndEndpointCapabilities(t *testing.T) {
	a := fixedKiroAccount()
	a.Status = StatusActive
	a.Schedulable = true
	for _, platform := range []string{PlatformAnthropic, PlatformGemini, PlatformOpenAI} {
		require.True(t, (&GatewayService{}).isAccountAllowedForPlatform(a, platform, true))
		require.False(t, (&GatewayService{}).isAccountAllowedForPlatform(a, platform, false))
	}
	require.False(t, a.IsModelSupported("gpt-5.5"))
	require.False(t, a.IsModelSupported("gemini-3-pro"))
	require.True(t, a.IsModelSupported("claude-haiku-4.5"))
	a.Credentials["model_mapping"] = map[string]any{"gpt-test": "claude-haiku-4.5"}
	require.True(t, a.IsModelSupported("gpt-test"))
	require.True(t, isOpenAICompatibleAccountEligibleForRequest(context.Background(), a, PlatformOpenAI, "gpt-test", false, OpenAIEndpointCapabilityChatCompletions))
	require.False(t, isOpenAICompatibleAccountEligibleForRequest(context.Background(), a, PlatformOpenAI, "gpt-test", true, ""))
	for _, capability := range []OpenAIEndpointCapability{OpenAIEndpointCapabilityResponses, OpenAIEndpointCapabilityEmbeddings, OpenAIEndpointCapabilityLive, OpenAIEndpointCapabilityAlphaSearch} {
		require.False(t, a.SupportsOpenAIEndpointCapability(capability))
	}
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	require.False(t, svc.isOpenAIAccountTransportCompatible(a, OpenAIUpstreamTransportResponsesWebsocketV2Ingress))
	require.True(t, svc.isOpenAIAccountTransportCompatible(a, OpenAIUpstreamTransportHTTPSSE))
}

func TestKiroGroupCacheOverridesLegacyAccountPolicyAndIsolatesGroups(t *testing.T) {
	upstream := &kiroCompatUpstream{t: t}
	cache := &kiroMemoryCache{values: map[string][]byte{}}
	policy := KiroCachePolicy{Enabled: true, TargetPercent: 100, TTLSeconds: 300, MaxEntries: 128}
	group := &Group{ID: 7, KiroResponseCache: policy}
	svc := &GatewayService{httpUpstream: upstream, cfg: &config.Config{}, cache: cache, groupRepo: &kiroGroupRepo{group: group}}
	a := fixedKiroAccount()
	a.Credentials["access_token"] = t.Name()
	a.Credentials["expires_at"] = time.Now().Add(time.Hour).Format(time.RFC3339)
	a.Extra = map[string]any{"kiro_response_cache": map[string]any{"enabled": false}}
	raw := []byte(`{"model":"claude-haiku-4.5","messages":[{"role":"user","content":"hello"}],"stream":false}`)
	invoke := func(groupID int64, expected string) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(raw))
		c.Set("api_key", &APIKey{ID: 1, UserID: 2, GroupID: &groupID})
		result, err := svc.ForwardAsChatCompletions(context.Background(), c, a, raw, nil)
		require.NoError(t, err)
		require.Equal(t, expected, rec.Header().Get("X-Sub2api-Cache"))
		if expected == "hit" {
			require.Zero(t, result.Usage.InputTokens)
		}
	}
	invoke(7, "miss")
	invoke(7, "hit")
	group.ID = 8
	invoke(8, "miss")
	invoke(8, "hit")
	group.KiroResponseCache.Enabled = false
	a.Extra["kiro_response_cache"] = map[string]any{"enabled": true}
	invoke(8, "bypass")
	require.Equal(t, 4, upstream.requests)
}

func TestKiroOpenAIAndGeminiBridgesPreserveResponseAndUsage(t *testing.T) {
	for _, protocol := range []string{"chat", "responses", "messages", "gemini"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%t", protocol, stream), func(t *testing.T) {
				upstream := &kiroCompatUpstream{t: t}
				gateway := &GatewayService{httpUpstream: upstream, cfg: &config.Config{}}
				openai := &OpenAIGatewayService{kiroGateway: gateway}
				a := fixedKiroAccount()
				a.Credentials["access_token"] = t.Name()
				a.Credentials["expires_at"] = time.Now().Add(time.Hour).Format(time.RFC3339)
				raw := []byte(fmt.Sprintf(`{"model":"claude-haiku-4.5","max_tokens":1024,"stream":%t,"messages":[{"role":"user","content":"hi"}]}`, stream))
				if protocol == "responses" {
					raw = []byte(fmt.Sprintf(`{"model":"claude-haiku-4.5","stream":%t,"input":"hi"}`, stream))
				}
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest("POST", "/test", bytes.NewReader(raw))
				c.Set("api_key", &APIKey{ID: 1, UserID: 2})
				if protocol == "gemini" {
					raw = []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
					result, err := gateway.ForwardKiroGemini(context.Background(), c, a, "claude-haiku-4.5", "generateContent", stream, raw)
					require.NoError(t, err)
					require.Equal(t, 11, result.Usage.InputTokens)
					require.Contains(t, rec.Body.String(), "candidates")
				} else {
					result, err := openai.forwardKiroCompatible(context.Background(), c, a, raw, protocol)
					require.NoError(t, err)
					require.Equal(t, 11, result.Usage.InputTokens)
					require.Equal(t, 2, result.Usage.OutputTokens)
				}
				require.Contains(t, rec.Body.String(), "OK")
				require.Equal(t, 200, rec.Code)
			})
		}
	}
}
