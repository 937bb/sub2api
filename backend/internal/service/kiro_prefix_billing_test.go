package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type kiroPrefixTestCache struct {
	kiroMemoryCache
	mu      sync.Mutex
	points  map[string]kiro.PrefixPoint
	commits int
	fail    bool
}

func (c *kiroPrefixTestCache) LookupKiroPrefix(_ context.Context, group int64, scope string, p *kiro.PrefixPlan, _ KiroCachePolicy) (*kiro.PrefixPoint, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fail {
		return nil, errors.New("store unavailable")
	}
	var best *kiro.PrefixPoint
	for _, lookup := range p.Lookups {
		if point, ok := c.points[fmt.Sprint(group)+scope+lookup.Digest]; ok && (best == nil || point.Tokens > best.Tokens) {
			copy := point
			best = &copy
		}
	}
	return best, nil
}

func (c *kiroPrefixTestCache) CommitKiroPrefix(_ context.Context, group int64, scope string, p *kiro.PrefixPlan, _ *kiro.PrefixPoint, _ KiroCachePolicy) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fail {
		return errors.New("store unavailable")
	}
	if c.points == nil {
		c.points = map[string]kiro.PrefixPoint{}
	}
	for _, point := range p.Writes {
		c.points[fmt.Sprint(group)+scope+point.Digest] = point
	}
	c.commits++
	return nil
}

func TestKiroPrefixFinalizerGuardsAndConcurrentColdRequests(t *testing.T) {
	raw := []byte(`{"system":"Stable instructions","messages":[{"role":"user","content":"hi"}]}`)
	p := KiroCachePolicy{Enabled: true, Mode: "prefix", AutoPrefix: true, TargetPercent: 90, TTLSeconds: 3600, MaxEntries: 128}
	cache := &kiroPrefixTestCache{}
	svc := &GatewayService{cache: cache}
	response := func() *kiro.Response {
		return &kiro.Response{StopReason: "end_turn", Content: []kiro.Block{{Type: "text", Text: "OK"}}, Usage: kiro.Usage{InputTokens: 1000, OutputTokens: 20}, UsageSource: "estimated"}
	}
	for _, reason := range []string{"refusal", "model_context_window_exceeded", ""} {
		out := response()
		out.StopReason = reason
		svc.kiroPrefixFinalizer(context.Background(), 7, "scope", raw, "model", p)(out)
		require.Zero(t, out.Usage.CacheCreationInputTokens)
	}
	out := response()
	out.UsageSource = "upstream"
	svc.kiroPrefixFinalizer(context.Background(), 7, "scope", raw, "model", p)(out)
	require.Equal(t, "upstream", out.UsageSource)
	require.Zero(t, cache.commits)
	ctx, cancel := context.WithCancel(context.Background())
	finalize := svc.kiroPrefixFinalizer(ctx, 7, "scope", raw, "model", p)
	cancel()
	finalize(response())
	require.Zero(t, cache.commits)
	first := svc.kiroPrefixFinalizer(context.Background(), 7, "scope", raw, "model", p)
	second := svc.kiroPrefixFinalizer(context.Background(), 7, "scope", raw, "model", p)
	a, b := response(), response()
	first(a)
	second(b)
	require.Zero(t, a.Usage.CacheReadInputTokens)
	require.Zero(t, b.Usage.CacheReadInputTokens, "in-flight cold requests must not see each other's future writes")
	require.Positive(t, a.Usage.CacheCreation.Ephemeral1hInputTokens)
	out = response()
	svc.kiroPrefixFinalizer(context.Background(), 7, "scope", raw, "model", p)(out)
	require.Positive(t, out.Usage.CacheReadInputTokens)
	require.Equal(t, 1000, out.Usage.InputTokens+out.Usage.CacheReadInputTokens+out.Usage.CacheCreationInputTokens)
	require.Equal(t, 20, out.Usage.OutputTokens)
	require.Equal(t, "local_prefix", out.Usage.CacheSource)
	finalize = svc.kiroPrefixFinalizer(context.Background(), 7, "scope", raw, "model", p)
	cache.fail = true
	out = response()
	finalize(out)
	require.Equal(t, "estimated", out.UsageSource)
	require.Zero(t, out.Usage.CacheReadInputTokens)
	require.Nil(t, svc.kiroPrefixFinalizer(context.Background(), 7, "scope", raw, "model", p))
}

func TestKiroPrefixBillingAcrossProtocols(t *testing.T) {
	for _, protocol := range []string{"messages", "chat", "responses", "gemini"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%v", protocol, stream), func(t *testing.T) {
				cache := &kiroPrefixTestCache{}
				groupID := int64(7)
				policy := KiroCachePolicy{Enabled: true, Mode: "prefix", AutoPrefix: true, TargetPercent: 90, TTLSeconds: 300, MaxEntries: 128}
				up := &kiroEmptyUpstream{kiroCompatUpstream: kiroCompatUpstream{t: t}}
				svc := &GatewayService{httpUpstream: up, cache: cache, cfg: &config.Config{}, groupRepo: &kiroGroupRepo{group: &Group{ID: groupID, KiroResponseCache: policy}}}
				account := fixedKiroAccount()
				account.Credentials["access_token"] = t.Name()
				account.Credentials["expires_at"] = time.Now().Add(time.Hour).Format(time.RFC3339)
				for turn := 0; turn < 2; turn++ {
					up.body = io.NopCloser(bytes.NewReader(kiroTestFrame("assistantResponseEvent", `{"content":"OK"}`)))
					raw := []byte(fmt.Sprintf(`{"model":"claude-haiku-4.5","stream":%t,"max_tokens":1024,"messages":[{"role":"user","content":"prefix metering test"}]}`, stream))
					if protocol == "responses" {
						raw = []byte(fmt.Sprintf(`{"model":"claude-haiku-4.5","stream":%t,"input":"prefix metering test"}`, stream))
					} else if protocol == "gemini" {
						raw = []byte(`{"contents":[{"role":"user","parts":[{"text":"prefix metering test"}]}]}`)
					}
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest("POST", "/test", bytes.NewReader(raw))
					c.Set("api_key", &APIKey{ID: 1, UserID: 2, GroupID: &groupID})
					var result *ForwardResult
					var err error
					switch protocol {
					case "messages":
						parsed, e := ParseGatewayRequest(NewRequestBodyRef(raw), PlatformAnthropic)
						require.NoError(t, e)
						result, err = svc.Forward(context.Background(), c, account, parsed)
					case "chat":
						result, err = svc.ForwardAsChatCompletions(context.Background(), c, account, raw, nil)
					case "responses":
						result, err = svc.ForwardAsResponses(context.Background(), c, account, raw, nil)
					case "gemini":
						result, err = svc.ForwardKiroGemini(context.Background(), c, account, "claude-haiku-4.5", "generateContent", stream, raw)
					}
					require.NoError(t, err)
					require.Contains(t, rec.Body.String(), "OK")
					require.Equal(t, "local_prefix", rec.Header().Get("X-Sub2api-Cache-Mode"))
					u := result.Usage
					require.Positive(t, u.InputTokens)
					require.Positive(t, u.OutputTokens, "every prefix-metered request still generates and bills output")
					if turn == 0 {
						require.Positive(t, u.CacheCreationInputTokens)
						require.Equal(t, u.CacheCreationInputTokens, u.CacheCreation5mTokens)
						require.Zero(t, u.CacheReadInputTokens)
					} else {
						require.Positive(t, u.CacheReadInputTokens)
						require.Zero(t, u.CacheCreationInputTokens)
					}
					billing := NewBillingService(&config.Config{}, nil)
					cost, e := billing.CalculateCost("claude-haiku-4.5", UsageTokens{InputTokens: u.InputTokens, OutputTokens: u.OutputTokens, CacheCreationTokens: u.CacheCreationInputTokens, CacheReadTokens: u.CacheReadInputTokens, CacheCreation5mTokens: u.CacheCreation5mTokens}, 0.4)
					require.NoError(t, e)
					require.Positive(t, cost.ActualCost)
				}
				require.Equal(t, 2, cache.commits)
			})
		}
	}
}
