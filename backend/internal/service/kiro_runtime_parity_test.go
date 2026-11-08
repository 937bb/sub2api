package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestKiroRuntimeParityAcrossProtocols(t *testing.T) {
	text := kiroTestFrame("assistantResponseEvent", `{"content":"partial"}`)
	refusal := kiroTestFrame("metadataEvent", `{"stopReason":"CONTENT_FILTERED","stopDetails":{"refusal":{"category":"CYBER","explanation":"private detail"}}}`)
	usage := kiroTestFrame("metadataEvent", `{"tokenUsage":{"uncachedInputTokens":101,"outputTokens":23,"cacheReadInputTokens":300,"cacheWriteInputTokens":40}}`)
	completeTool := kiroTestFrame("toolUseEvent", `{"name":"read_probe","toolUseId":"call1","input":"{}","stop":true}`)
	pendingTool := kiroTestFrame("toolUseEvent", `{"name":"read_probe","toolUseId":"call1","input":"{"}`)
	for _, tc := range []struct {
		name                               string
		frames                             [][]byte
		tools, failed, refused, localCache bool
		want                               ClaudeUsage
	}{
		{name: "text_refusal", frames: [][]byte{text, usage, refusal}, refused: true},
		{name: "completed_tool_refusal", frames: [][]byte{completeTool, refusal}, tools: true, refused: true},
		{name: "pending_tool_refusal", frames: [][]byte{pendingTool, refusal}, tools: true, failed: true},
		{name: "provider_usage", frames: [][]byte{text, usage}, want: ClaudeUsage{InputTokens: 101, OutputTokens: 23, CacheReadInputTokens: 300, CacheCreationInputTokens: 40}},
		{name: "last_snapshot_zero", frames: [][]byte{text, usage, kiroTestFrame("metadataEvent", `{"tokenUsage":{"uncachedInputTokens":0,"outputTokens":0}}`)}},
		{name: "local_creation", frames: [][]byte{text, usage}, localCache: true, want: ClaudeUsage{OutputTokens: 23, CacheCreationInputTokens: 441, CacheCreation5mTokens: 441}},
	} {
		for _, protocol := range []string{"messages", "chat", "responses", "gemini"} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/stream=%t", tc.name, protocol, stream), func(t *testing.T) {
					upstream := &kiroEmptyUpstream{kiroCompatUpstream: kiroCompatUpstream{t: t}, body: io.NopCloser(bytes.NewReader(bytes.Join(tc.frames, nil)))}
					cache := &kiroMemoryCache{values: map[string][]byte{}}
					groupID := int64(7)
					policy := KiroCachePolicy{Enabled: tc.localCache || tc.refused, TargetPercent: 100, TTLSeconds: 300, MaxEntries: 128}
					svc := &GatewayService{httpUpstream: upstream, cfg: &config.Config{}, cache: cache, groupRepo: &kiroGroupRepo{group: &Group{ID: groupID, KiroResponseCache: policy}}}
					account := fixedKiroAccount()
					account.Credentials["access_token"] = t.Name()
					account.Credentials["expires_at"] = time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
					body := map[string]any{"model": "claude-haiku-4.5", "max_tokens": 1024, "stream": stream, "messages": []any{map[string]any{"role": "user", "content": "hi"}}}
					schema := map[string]any{"type": "object", "properties": map[string]any{}}
					switch protocol {
					case "messages":
						if tc.tools {
							body["tools"] = []any{map[string]any{"name": "read_probe", "input_schema": schema}}
						}
					case "chat":
						if tc.tools {
							body["tools"] = []any{map[string]any{"type": "function", "function": map[string]any{"name": "read_probe", "parameters": schema}}}
						}
					case "responses":
						delete(body, "messages")
						delete(body, "max_tokens")
						body["input"] = "hi"
						if tc.tools {
							body["tools"] = []any{map[string]any{"type": "function", "name": "read_probe", "parameters": schema}}
						}
					case "gemini":
						body = map[string]any{"contents": []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": "hi"}}}}}
						if tc.tools {
							body["tools"] = []any{map[string]any{"functionDeclarations": []any{map[string]any{"name": "read_probe", "parameters": schema}}}}
						}
					}
					raw, err := json.Marshal(body)
					require.NoError(t, err)
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest("POST", "/test", bytes.NewReader(raw))
					c.Set("api_key", &APIKey{ID: 1, UserID: 2, GroupID: &groupID})
					var result *ForwardResult
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
					wire := rec.Body.String()
					require.NotContains(t, wire, "private detail")
					if tc.failed {
						require.Error(t, err)
						require.Contains(t, wire, "incomplete tool call")
						for _, terminal := range []string{"message_stop", "response.completed", "response.incomplete", "[DONE]", "finishReason", "\"finish_reason\":\"tool_calls\""} {
							require.NotContains(t, wire, terminal)
						}
						if result != nil {
							require.Equal(t, ClaudeUsage{}, result.Usage)
						}
					} else {
						require.NoError(t, err)
						require.NotNil(t, result)
						require.Equal(t, tc.want, result.Usage)
						require.Equal(t, tc.refused, result.KiroRefused)
						require.Equal(t, 200, rec.Code)
						if tc.refused {
							marker := map[string]string{"messages": `"stop_reason":"refusal"`, "chat": `"finish_reason":"content_filter"`, "responses": `"reason":"content_filter"`, "gemini": `"finishReason":"SAFETY"`}[protocol]
							require.Contains(t, wire, marker)
						}
					}
					if tc.localCache {
						require.Len(t, cache.values, 1)
					} else {
						require.Empty(t, cache.values)
					}
					require.Equal(t, 2, upstream.requests, "discovery and one generation only")
				})
			}
		}
	}
}

func TestKiroAccountTestDoesNotAcceptRefusal(t *testing.T) {
	upstream := &kiroEmptyUpstream{kiroCompatUpstream: kiroCompatUpstream{t: t}, body: io.NopCloser(bytes.NewReader(kiroTestFrame("metadataEvent", `{"stopReason":"CONTENT_FILTERED"}`)))}
	account := fixedKiroAccount()
	account.Credentials["access_token"] = t.Name()
	account.Credentials["expires_at"] = time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/test", nil)
	svc := &AccountTestService{httpUpstream: upstream}
	err := svc.testKiroAccountConnection(c, account, "claude-haiku-4.5", "hi")
	require.Error(t, err)
	require.Contains(t, rec.Body.String(), "CONTENT_FILTERED")
	require.NotContains(t, rec.Body.String(), `"success":true`)
}

func TestKiroRefusalSkipsAllBillingModes(t *testing.T) {
	// Empty dependencies intentionally fail if refusal reaches pricing, quota,
	// persistence or per-request billing instead of the early exit.
	svc := &GatewayService{}
	result := &ForwardResult{KiroRefused: true, Usage: ClaudeUsage{InputTokens: 123, OutputTokens: 9}}
	require.NoError(t, svc.RecordUsage(context.Background(), &RecordUsageInput{Result: result}))
	require.Zero(t, svc.QuoteUsageCost(context.Background(), &RecordUsageInput{Result: result}))
}

func TestKiroRefusalFlagPrecedesTerminalDelivery(t *testing.T) {
	upstream := &kiroEmptyUpstream{kiroCompatUpstream: kiroCompatUpstream{t: t}, body: io.NopCloser(bytes.NewReader(kiroTestFrame("metadataEvent", `{"stopReason":"CONTENT_FILTERED"}`)))}
	svc := &GatewayService{httpUpstream: upstream, cfg: &config.Config{}}
	account := fixedKiroAccount()
	account.Credentials["access_token"] = t.Name()
	account.Credentials["expires_at"] = time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	raw := []byte(`{"model":"claude-haiku-4.5","max_tokens":1024,"messages":[{"role":"user","content":"hi"}]}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/test", bytes.NewReader(raw))
	c.Set("api_key", &APIKey{ID: 1, UserID: 2})
	parsed, err := kiroParsedRequest(c, raw)
	require.NoError(t, err)
	resp, err := svc.kiroAsAnthropic(context.Background(), c, account, raw, parsed, "claude-haiku-4.5")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), `"stop_reason":"refusal"`) {
			result := markKiroRefusal(&ForwardResult{}, resp.Body)
			require.True(t, result.KiroRefused, "client may disconnect before message_stop")
			return
		}
	}
	require.NoError(t, scanner.Err())
	t.Fatal("missing refusal delta")
}
