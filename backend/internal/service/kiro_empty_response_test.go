package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type kiroEmptyUpstream struct {
	kiroCompatUpstream
	body io.ReadCloser
}

func (u *kiroEmptyUpstream) Do(req *http.Request, proxy string, id int64, concurrency int) (*http.Response, error) {
	resp, err := u.kiroCompatUpstream.Do(req, proxy, id, concurrency)
	if err == nil && req.URL.Host == "runtime.us-east-1.kiro.dev" {
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(kiroTestFrame("contextUsageEvent", `{"contextUsagePercentage":0.68}`)))
		if u.body != nil {
			resp.Body = u.body
		}
	}
	return resp, err
}

func (u *kiroEmptyUpstream) DoWithTLS(req *http.Request, proxy string, id int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxy, id, concurrency)
}

func TestKiroEmptyAndRefusalAcrossIngress(t *testing.T) {
	for _, filtered := range []bool{false, true} {
		status, code, message := http.StatusBadGateway, kiroEmptyResponseCode, kiroEmptyResponseMessage
		for _, protocol := range []string{"messages", "chat", "responses", "gemini"} {
			for _, stream := range []bool{false, true} {
				for _, flushed := range []bool{false, true} {
					if flushed && !stream {
						continue
					}
					t.Run(fmt.Sprintf("%s/stream=%t/flushed=%t/filtered=%t", protocol, stream, flushed, filtered), func(t *testing.T) {
						upstream := &kiroEmptyUpstream{kiroCompatUpstream: kiroCompatUpstream{t: t}}
						if filtered {
							upstream.body = io.NopCloser(bytes.NewReader(kiroTestFrame("metadataEvent", `{"stopReason":"CONTENT_FILTERED","stopDetails":{"refusal":{"category":"CYBER","explanation":"private explanation"}}}`)))
						}
						cache := &kiroMemoryCache{values: map[string][]byte{}}
						groupID := int64(7)
						group := &Group{ID: groupID, KiroResponseCache: KiroCachePolicy{Enabled: true, TargetPercent: 100, TTLSeconds: 300, MaxEntries: 128}}
						svc := &GatewayService{httpUpstream: upstream, cfg: &config.Config{}, cache: cache, groupRepo: &kiroGroupRepo{group: group}}
						a := fixedKiroAccount()
						a.Credentials["access_token"] = t.Name()
						a.Credentials["expires_at"] = time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
						raw := []byte(fmt.Sprintf(`{"model":"claude-haiku-4.5","max_tokens":1024,"stream":%t,"messages":[{"role":"user","content":"hi"}]}`, stream))
						if protocol == "responses" {
							raw = []byte(fmt.Sprintf(`{"model":"claude-haiku-4.5","stream":%t,"input":"hi"}`, stream))
						}
						rec := httptest.NewRecorder()
						c, _ := gin.CreateTestContext(rec)
						c.Request = httptest.NewRequest("POST", "/test", bytes.NewReader(raw))
						c.Set("api_key", &APIKey{ID: 1, UserID: 2, GroupID: &groupID})
						if flushed {
							c.Header("Content-Type", "text/event-stream")
							_, _ = c.Writer.WriteString(": ping\n\n")
							c.Writer.Flush()
						}
						var result *ForwardResult
						var err error
						switch protocol {
						case "messages":
							parsed, parseErr := ParseGatewayRequest(NewRequestBodyRef(raw), PlatformAnthropic)
							require.NoError(t, parseErr)
							result, err = svc.Forward(context.Background(), c, a, parsed)
						case "chat":
							result, err = svc.ForwardAsChatCompletions(context.Background(), c, a, raw, nil)
						case "responses":
							result, err = svc.ForwardAsResponses(context.Background(), c, a, raw, nil)
						case "gemini":
							result, err = svc.ForwardKiroGemini(context.Background(), c, a, "claude-haiku-4.5", "generateContent", stream, []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`))
						}
						if filtered {
							require.NoError(t, err)
							require.NotNil(t, result)
							require.Equal(t, ClaudeUsage{}, result.Usage, "refusals must not introduce charges")
							require.Equal(t, http.StatusOK, rec.Code)
							require.NotContains(t, rec.Body.String(), "private explanation")
							require.NotContains(t, rec.Body.String(), `"error":`)
							switch protocol {
							case "messages":
								require.Contains(t, rec.Body.String(), `"stop_reason":"refusal"`)
							case "chat":
								require.Contains(t, rec.Body.String(), `"finish_reason":"content_filter"`)
							case "responses":
								require.Contains(t, rec.Body.String(), `"status":"incomplete"`)
								require.Contains(t, rec.Body.String(), `"reason":"content_filter"`)
								require.NotContains(t, rec.Body.String(), "response.completed")
							case "gemini":
								require.Contains(t, rec.Body.String(), `"finishReason":"SAFETY"`)
							}
							require.Empty(t, cache.values)
							require.Equal(t, 2, upstream.requests)
							return
						}
						var empty *kiro.NoContentError
						require.ErrorAs(t, err, &empty)
						require.Nil(t, result, "empty responses must never be billed")
						require.True(t, IsResponseCommitted(c), "handler must not replace the diagnosis with a generic error")
						require.Contains(t, rec.Body.String(), code)
						require.Contains(t, rec.Body.String(), message)
						for _, terminal := range []string{"message_stop", "response.completed", "[DONE]", "finishReason"} {
							require.NotContains(t, rec.Body.String(), terminal)
						}
						if flushed {
							require.Equal(t, http.StatusOK, rec.Code)
							if protocol == "responses" {
								require.Contains(t, rec.Body.String(), "event:response.failed")
							}
						} else {
							require.Equal(t, status, rec.Code)
						}
						require.Empty(t, cache.values, "empty responses must never enter the response cache")
						require.Equal(t, 2, upstream.requests, "discovery plus one generation, without account failover")
					})
				}
			}
		}
	}
}

type kiroWaitingBody struct {
	reading chan struct{}
	closed  chan struct{}
	read    sync.Once
	close   sync.Once
}

func (b *kiroWaitingBody) Read([]byte) (int, error) {
	b.read.Do(func() { close(b.reading) })
	<-b.closed
	return 0, io.ErrClosedPipe
}

func (b *kiroWaitingBody) Close() error {
	b.close.Do(func() { close(b.closed) })
	return nil
}

func TestKiroFirstContentWaitStopsOnClientCancellation(t *testing.T) {
	body := &kiroWaitingBody{reading: make(chan struct{}), closed: make(chan struct{})}
	upstream := &kiroEmptyUpstream{kiroCompatUpstream: kiroCompatUpstream{t: t}, body: body}
	svc := &GatewayService{httpUpstream: upstream, cfg: &config.Config{}}
	a := fixedKiroAccount()
	a.Credentials["access_token"] = t.Name()
	a.Credentials["expires_at"] = time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer func() { _ = body.Close() }()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil).WithContext(ctx)
	c.Set("api_key", &APIKey{ID: 1, UserID: 2})
	done := make(chan error, 1)
	go func() {
		resp, err := svc.kiroAsAnthropic(ctx, c, a, []byte(`{"model":"claude-haiku-4.5","max_tokens":1024,"messages":[{"role":"user","content":"hi"}]}`), nil, "claude-haiku-4.5")
		if resp != nil {
			_ = resp.Body.Close()
			done <- fmt.Errorf("received a response before upstream content")
			return
		}
		done <- err
	}()
	select {
	case <-body.reading:
	case err := <-done:
		t.Fatalf("request failed before first-content wait: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("decoder did not start")
	}
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("first-content wait leaked after cancellation")
	}
	select {
	case <-body.closed:
	default:
		t.Fatal("upstream response was not closed")
	}
}
