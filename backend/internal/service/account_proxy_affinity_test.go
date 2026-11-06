//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func affinityTestAccount() *Account {
	id := int64(1)
	return &Account{ID: 9850, Platform: PlatformOpenAI, ProxyID: &id,
		Extra: map[string]any{AccountProxyFallbackIDsKey: []int64{2, 3}},
		Proxy: &Proxy{ID: 1, Protocol: "http", Host: "primary.invalid", Port: 8080, Status: StatusActive},
		FallbackProxies: []*Proxy{
			{ID: 2, Protocol: "http", Host: "backup.invalid", Port: 8080, Status: StatusActive},
			{ID: 3, Protocol: "http", Host: "third.invalid", Port: 8080, Status: StatusActive},
		},
	}
}

func affinityTestContext(thread string, key int64) context.Context {
	c := newOpenAIWSExecutionScopeTestContext(map[string]string{"thread-id": thread})
	c.Set("api_key", &APIKey{ID: key})
	return withAccountProxyAffinity(context.Background(), c, nil)
}

func TestAccountProxyAffinityStableAcrossSnapshotsAndCredentials(t *testing.T) {
	a := affinityTestAccount()
	next := affinityTestAccount()
	next.Extra[AccountProxyFallbackIDsKey] = []int64{3, 2}
	next.FallbackProxies[0], next.FallbackProxies[1] = next.FallbackProxies[1], next.FallbackProxies[0]
	for i := 0; i < 128; i++ {
		ctx := affinityTestContext(fmt.Sprintf("thread-%d", i), 1)
		original, err := accountProxyRoutesForContext(ctx, a, "")
		require.NoError(t, err)
		require.Len(t, original, 1)
		reordered, err := accountProxyRoutesForContext(ctx, next, "")
		require.NoError(t, err)
		require.Equal(t, original, reordered)
	}
	ctx := affinityTestContext("credentials-rotate", 1)
	before, err := accountProxyRoutesForContext(ctx, a, "")
	require.NoError(t, err)
	for _, p := range append([]*Proxy{a.Proxy}, a.FallbackProxies...) {
		if p.URL() == before[0] {
			p.Username, p.Password = "rotated", "fixture"
			after, err := accountProxyRoutesForContext(ctx, a, "")
			require.NoError(t, err)
			require.Equal(t, []string{p.URL()}, after)
			require.NotEqual(t, before, after)
			return
		}
	}
	t.Fatal("selected proxy not found")
}

func TestAccountProxyAffinityCapturesOriginalIdentity(t *testing.T) {
	c := newOpenAIWSExecutionScopeTestContext(nil)
	c.Set("api_key", &APIKey{ID: 7})
	body := []byte(`{"client_metadata":{"thread_id":"original"},"input":"first"}`)
	ctx := withAccountProxyAffinity(context.Background(), c, body)
	want := ctx.Value(accountProxyAffinityKey{})
	require.NotEmpty(t, want)
	rewritten := []byte(`{"client_metadata":{"thread_id":"account-default"},"prompt_cache_key":"generated"}`)
	require.Equal(t, want, withAccountProxyAffinity(ctx, c, rewritten).Value(accountProxyAffinityKey{}))
	require.Equal(t, want, withAccountProxyAffinity(context.Background(), c, []byte(`{"client_metadata":{"thread_id":"original"},"input":"next tool output"}`)).Value(accountProxyAffinityKey{}))
	require.NotEqual(t, want, affinityTestContext("original", 8).Value(accountProxyAffinityKey{}))
	empty := withAccountProxyAffinity(context.Background(), c, []byte(`{"input":"no identity"}`))
	require.Equal(t, "", withAccountProxyAffinity(empty, c, rewritten).Value(accountProxyAffinityKey{}))

	session := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	for _, version := range []string{"2.1.1", "2.1.100"} {
		raw := FormatMetadataUserID(strings.Repeat("a", 64), "", session, version)
		b, err := json.Marshal(map[string]any{"metadata": map[string]string{"user_id": raw}})
		require.NoError(t, err)
		metadataCtx := withAccountProxyAffinity(context.Background(), c, b)
		c.Request.Header.Set(claudeCodeSessionHeader, session)
		headerCtx := withAccountProxyAffinity(context.Background(), c, nil)
		require.NotEmpty(t, metadataCtx.Value(accountProxyAffinityKey{}))
		require.Equal(t, metadataCtx.Value(accountProxyAffinityKey{}), headerCtx.Value(accountProxyAffinityKey{}))
		c.Request.Header.Del(claudeCodeSessionHeader)
	}
	require.Equal(t, "", withAccountProxyAffinity(context.Background(), c, []byte(`{"metadata":{"user_id":"user-not-session"}}`)).Value(accountProxyAffinityKey{}))
}

func TestAccountProxyAffinityConcurrentDistribution(t *testing.T) {
	a := affinityTestAccount()
	counts := map[string]int{}
	var wg sync.WaitGroup
	results := make(chan string, 600)
	for i := 0; i < 600; i++ {
		ctx := affinityTestContext(fmt.Sprintf("conversation-%d", i), 7)
		wg.Add(1)
		go func() {
			defer wg.Done()
			routes, err := accountProxyRoutesForContext(ctx, a, "")
			if err != nil || len(routes) != 1 {
				t.Error("route selection failed")
				return
			}
			for j := 0; j < 12; j++ {
				again, err := accountProxyRoutesForContext(ctx, a, "")
				if err != nil || len(again) != 1 || routes[0] != again[0] {
					t.Error("conversation moved proxy")
					return
				}
			}
			results <- routes[0]
		}()
	}
	wg.Wait()
	close(results)
	for route := range results {
		counts[route]++
	}
	require.Len(t, counts, 3)
	for _, n := range counts {
		require.InDelta(t, 200, n, 45)
	}
}

func TestAccountProxyAffinityFailsClosedWithoutReplay(t *testing.T) {
	for _, code := range []int{0, 200, 401, 403, 429, 502} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			a := affinityTestAccount()
			ctx := affinityTestContext("no-replay", 1)
			calls := 0
			for turn := 0; turn < 2; turn++ {
				req := httptest.NewRequest(http.MethodPost, "https://example.com", strings.NewReader("fixture")).WithContext(ctx)
				resp, err := doWithAccountProxyFallback(req, "", a, func(_ *http.Request, route string) (*http.Response, error) {
					calls++
					if code == 0 {
						return nil, &net.OpError{Op: "dial", Err: errors.New("refused")}
					}
					return &http.Response{StatusCode: code, Body: http.NoBody}, nil
				})
				if code == 0 {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
					require.Equal(t, code, resp.StatusCode)
				}
			}
			require.Equal(t, 2, calls, "each turn attempts only its assigned proxy")
			selected, err := accountProxyRoutesForContext(ctx, a, "")
			require.NoError(t, err)
			for _, p := range append([]*Proxy{a.Proxy}, a.FallbackProxies...) {
				if p.URL() == selected[0] {
					p.Status = "inactive"
				}
			}
			routes, err := accountProxyRoutesForContext(ctx, a, "http://stale.invalid")
			require.ErrorContains(t, err, "bound conversation proxy unavailable")
			require.Nil(t, routes)
		})
	}
}

func TestAccountProxyAffinityForwardingEntryPoints(t *testing.T) {
	for _, endpoint := range []string{"responses", "chat", "messages"} {
		t.Run(endpoint, func(t *testing.T) {
			a := affinityTestAccount()
			a.Type = AccountTypeAPIKey
			a.Credentials = map[string]any{"api_key": "fixture"}
			a.Concurrency = 1
			u := &httpUpstreamRecorder{err: errors.New("fixture-stop-before-network")}
			s := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: u, toolCorrector: NewCodexToolCorrector()}
			var selected string
			for turn := 0; turn < 3; turn++ {
				body := []byte(fmt.Sprintf(`{"model":"gpt-5.4","max_tokens":32,"stream":false,"input":"turn %d","messages":[{"role":"user","content":"hello"}],"client_metadata":{"thread_id":"original"}}`, turn))
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest("POST", "/v1/"+endpoint, strings.NewReader(string(body)))
				c.Set("api_key", &APIKey{ID: 7})
				var err error
				switch endpoint {
				case "responses":
					_, err = s.Forward(context.Background(), c, a, body)
				case "chat":
					_, err = s.ForwardAsChatCompletions(context.Background(), c, a, body, "generated-cache", "")
				case "messages":
					_, err = s.ForwardAsAnthropic(context.Background(), c, a, body, "generated-cache", "")
				}
				require.Error(t, err)
				require.Len(t, u.requests, turn+1, "%v", err)
				require.Equal(t, affinityTestContext("original", 7).Value(accountProxyAffinityKey{}), u.lastReq.Context().Value(accountProxyAffinityKey{}))
				if turn == 0 {
					selected = u.lastProxyURL
				}
				require.Equal(t, selected, u.lastProxyURL)
			}
		})
	}
}
