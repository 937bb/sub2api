package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestKiroCacheBillingAllProtocols(t *testing.T) {
	for _, protocol := range []string{"messages", "chat", "responses", "gemini"} {
		for _, stream := range []bool{false, true} {
			for _, ttl := range []int{300, 3600} {
				t.Run(fmt.Sprintf("%s/stream=%v/ttl=%d", protocol, stream, ttl), func(t *testing.T) {
					up := &kiroCompatUpstream{t: t}
					cache := &kiroMemoryCache{values: map[string][]byte{}}
					groupID := int64(7)
					svc := &GatewayService{httpUpstream: up, cfg: &config.Config{}, cache: cache, groupRepo: &kiroGroupRepo{group: &Group{ID: groupID, KiroResponseCache: KiroCachePolicy{Enabled: true, TargetPercent: 100, TTLSeconds: ttl, MaxEntries: 128}}}}
					a := fixedKiroAccount()
					a.Credentials["access_token"] = t.Name()
					a.Credentials["expires_at"] = time.Now().Add(time.Hour).Format(time.RFC3339)
					for _, hit := range []bool{false, true} {
						raw := []byte(fmt.Sprintf(`{"model":"claude-haiku-4.5","max_tokens":1024,"stream":%t,"messages":[{"role":"user","content":"hi"}]}`, stream))
						if protocol == "responses" {
							raw = []byte(fmt.Sprintf(`{"model":"claude-haiku-4.5","stream":%t,"input":"hi"}`, stream))
						}
						if protocol == "gemini" {
							raw = []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
						}
						rec := httptest.NewRecorder()
						c, _ := gin.CreateTestContext(rec)
						c.Request = httptest.NewRequest("POST", "/test", strings.NewReader(string(raw)))
						c.Set("api_key", &APIKey{ID: 1, UserID: 2, GroupID: &groupID})
						var result *ForwardResult
						var err error
						switch protocol {
						case "messages":
							parsed, e := ParseGatewayRequest(NewRequestBodyRef(raw), PlatformAnthropic)
							require.NoError(t, e)
							result, err = svc.Forward(context.Background(), c, a, parsed)
						case "chat":
							result, err = svc.ForwardAsChatCompletions(context.Background(), c, a, raw, nil)
						case "responses":
							result, err = svc.ForwardAsResponses(context.Background(), c, a, raw, nil)
						case "gemini":
							result, err = svc.ForwardKiroGemini(context.Background(), c, a, "claude-haiku-4.5", "generateContent", stream, raw)
						}
						require.NoError(t, err)
						require.Contains(t, rec.Body.String(), "OK")
						u := result.Usage
						require.Zero(t, u.InputTokens)
						if hit {
							require.Equal(t, 11, u.CacheReadInputTokens)
							require.Zero(t, u.CacheCreationInputTokens)
							require.Zero(t, u.OutputTokens)
						} else {
							require.Equal(t, 11, u.CacheCreationInputTokens)
							require.Zero(t, u.CacheReadInputTokens)
							require.Equal(t, 2, u.OutputTokens)
							if ttl == 300 {
								require.Equal(t, 11, u.CacheCreation5mTokens)
							} else {
								require.Equal(t, 11, u.CacheCreation1hTokens)
							}
						}
						billing := NewBillingService(&config.Config{}, nil)
						price, e := billing.GetModelPricing("claude-haiku-4.5")
						require.NoError(t, e)
						cost, e := billing.CalculateCost("claude-haiku-4.5", UsageTokens{InputTokens: u.InputTokens, OutputTokens: u.OutputTokens, CacheCreationTokens: u.CacheCreationInputTokens, CacheReadTokens: u.CacheReadInputTokens, CacheCreation5mTokens: u.CacheCreation5mTokens, CacheCreation1hTokens: u.CacheCreation1hTokens}, 0.4)
						require.NoError(t, e)
						if hit {
							require.InDelta(t, 11*price.CacheReadPricePerToken, cost.CacheReadCost, 1e-12)
							require.Zero(t, cost.CacheCreationCost)
						} else {
							require.Positive(t, cost.CacheCreationCost)
							require.Zero(t, cost.CacheReadCost)
						}
						require.InDelta(t, cost.TotalCost*0.4, cost.ActualCost, 1e-12)
					}
					require.Equal(t, 2, up.requests, "one catalog request and one inference")
				})
			}
		}
	}
}

type kiroFailedCache struct{ KiroResponseCache }

func (*kiroFailedCache) PutKiroResponse(context.Context, int64, string, []byte, KiroCachePolicy) error {
	return errors.New("redis unavailable")
}

func TestKiroCacheWriteFailureAndIncompleteResponsesNotCharged(t *testing.T) {
	policy := KiroCachePolicy{TTLSeconds: 3600}
	for _, reason := range []string{"end_turn", "tool_use", "max_tokens"} {
		out := &kiro.Response{StopReason: reason, Content: []kiro.Block{{Type: "text", Text: "OK"}}, Usage: kiro.Usage{InputTokens: 11, OutputTokens: 2}}
		kiroCacheFinalizer(context.Background(), &kiroFailedCache{}, 1, "key", policy, true)(out)
		require.Equal(t, 11, out.Usage.InputTokens)
		require.Zero(t, out.Usage.CacheCreationInputTokens)
	}
}

func TestKiroCacheReclassifiesPromptWithoutDoubleCharging(t *testing.T) {
	cache := &kiroMemoryCache{values: map[string][]byte{}}
	out := &kiro.Response{StopReason: "end_turn", Content: []kiro.Block{{Type: "text", Text: "OK"}}, Usage: kiro.Usage{InputTokens: 11, OutputTokens: 2, CacheCreationInputTokens: 5, CacheReadInputTokens: 7}}
	kiroCacheFinalizer(context.Background(), cache, 1, "key", KiroCachePolicy{TTLSeconds: 3600}, true)(out)
	require.Equal(t, 23, out.Usage.CacheCreationInputTokens)
	require.Zero(t, out.Usage.InputTokens)
	require.Zero(t, out.Usage.CacheReadInputTokens)
	var cached kiro.Response
	require.NoError(t, json.Unmarshal(cache.values["key"], &cached))
	kiroCacheRead(&cached)
	require.Equal(t, 23, cached.Usage.CacheReadInputTokens)
	require.Zero(t, cached.Usage.OutputTokens)
	require.Equal(t, "local", cached.Usage.CacheSource)
}

func TestKiroCacheUsageRespectsConfiguredPricing(t *testing.T) {
	billing := NewBillingService(&config.Config{}, nil)
	for _, source := range []string{PricingSourceGroup, PricingSourceChannel} {
		for _, rate := range []float64{0, 3e-6, 9e-6} {
			pricing := &ModelPricing{CacheCreationPricePerToken: rate, CacheCreationPriceExplicit: true, CacheReadPricePerToken: rate}
			for _, hit := range []bool{false, true} {
				tokens := UsageTokens{CacheCreationTokens: 1000, CacheCreation1hTokens: 1000}
				if hit {
					tokens = UsageTokens{CacheReadTokens: 1000}
				}
				cost, err := billing.CalculateCostUnified(CostInput{Ctx: context.Background(), Model: "claude-sonnet-5-5", Tokens: tokens, RateMultiplier: 0.4,
					Resolver: NewModelPricingResolver(nil, billing), Resolved: &ResolvedPricing{Mode: BillingModeToken, Source: source, BasePricing: pricing}})
				require.NoError(t, err)
				require.InDelta(t, 1000*rate, cost.TotalCost, 1e-12)
				require.InDelta(t, 1000*rate*0.4, cost.ActualCost, 1e-12)
			}
		}
	}
}
