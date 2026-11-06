package service

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func kiroTestFrame(kind, payload string) []byte {
	var h bytes.Buffer
	for _, p := range [][2]string{{":event-type", kind}, {":message-type", "event"}} {
		h.WriteByte(byte(len(p[0])))
		h.WriteString(p[0])
		h.WriteByte(7)
		_ = binary.Write(&h, binary.BigEndian, uint16(len(p[1])))
		h.WriteString(p[1])
	}
	b := make([]byte, 12)
	binary.BigEndian.PutUint32(b, uint32(16+h.Len()+len(payload)))
	binary.BigEndian.PutUint32(b[4:], uint32(h.Len()))
	binary.BigEndian.PutUint32(b[8:], crc32.ChecksumIEEE(b[:8]))
	b = append(b, h.Bytes()...)
	b = append(b, []byte(payload)...)
	return binary.BigEndian.AppendUint32(b, crc32.ChecksumIEEE(b))
}

type kiroCompatUpstream struct {
	t        *testing.T
	requests int
}

func (u *kiroCompatUpstream) Do(req *http.Request, proxy string, id int64, _ int) (*http.Response, error) {
	require.Equal(u.t, "http://test:secret@192.0.2.17:8000", proxy)
	require.EqualValues(u.t, 19, id)
	u.requests++
	assertKiroTransportContext(u.t, req)
	require.Contains(u.t, req.Header.Get("User-Agent"), "os/darwin#24.6.0 lang/js md/nodejs#22.22.0")
	require.Contains(u.t, req.Header.Get("User-Agent"), strings.Repeat("a", 64))
	require.NotEmpty(u.t, req.Header.Get("amz-sdk-invocation-id"))
	response := `{"models":[{"modelId":"claude-haiku-4.5","modelName":"Haiku"}]}`
	if req.URL.Host == "runtime.us-east-1.kiro.dev" {
		require.Equal(u.t, "/", req.URL.Path)
		require.Equal(u.t, "KiroRuntimeService.GenerateAssistantResponse", req.Header.Get("X-Amz-Target"))
		require.Equal(u.t, "application/x-amz-json-1.0", req.Header.Get("Content-Type"))
		body, _ := io.ReadAll(req.Body)
		require.Contains(u.t, string(body), `"conversationState"`)
		require.NotContains(u.t, string(body), "additionalModelRequestFields")
		response = string(kiroTestFrame("assistantResponseEvent", `{"content":"OK"}`)) + string(kiroTestFrame("meteringEvent", `{"unit":"credit","usage":0.001,"inputTokens":11,"outputTokens":2}`))
	} else {
		require.Equal(u.t, "management.us-east-1.kiro.dev", req.URL.Host)
	}
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(response))}, nil
}
func (u *kiroCompatUpstream) DoWithTLS(r *http.Request, p string, id int64, c int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(r, p, id, c)
}
func TestKiroAllIngressProtocolsUsePinnedProxy(t *testing.T) {
	for _, protocol := range []string{"messages", "chat", "responses"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%v", protocol, stream), func(t *testing.T) {
				upstream := &kiroCompatUpstream{t: t}
				svc := &GatewayService{httpUpstream: upstream, cfg: &config.Config{}}
				a := fixedKiroAccount()
				a.Credentials["access_token"] = t.Name()
				a.Credentials["expires_at"] = time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
				raw := fmt.Sprintf(`{"model":"claude-haiku-4.5","max_tokens":1024,"stream":%t,"messages":[{"role":"user","content":"hi"}]}`, stream)
				if protocol == "responses" {
					raw = fmt.Sprintf(`{"model":"claude-haiku-4.5","stream":%t,"input":"hi"}`, stream)
				}
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest("POST", "/v1/"+protocol, strings.NewReader(raw))
				c.Set("api_key", &APIKey{ID: 1, UserID: 2})
				var result *ForwardResult
				var err error
				switch protocol {
				case "messages":
					parsed, e := ParseGatewayRequest(NewRequestBodyRef([]byte(raw)), PlatformAnthropic)
					require.NoError(t, e)
					result, err = svc.Forward(context.Background(), c, a, parsed)
				case "chat":
					result, err = svc.ForwardAsChatCompletions(context.Background(), c, a, []byte(raw), nil)
				case "responses":
					result, err = svc.ForwardAsResponses(context.Background(), c, a, []byte(raw), nil)
				}
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, 11, result.Usage.InputTokens)
				require.Equal(t, 2, result.Usage.OutputTokens)
				require.Contains(t, rec.Body.String(), "OK")
				require.Equal(t, 2, upstream.requests)
			})
		}
	}
}

type kiroMemoryCache struct {
	GatewayCache
	mu     sync.Mutex
	values map[string][]byte
}

func (c *kiroMemoryCache) GetKiroResponse(_ context.Context, _ int64, key string, _ KiroCachePolicy) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]byte(nil), c.values[key]...), nil
}
func (c *kiroMemoryCache) PutKiroResponse(_ context.Context, _ int64, key string, value []byte, _ KiroCachePolicy) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.values[key] = append([]byte(nil), value...)
	return nil
}
func TestKiroCompatibilityCacheDoesNotCallOrChargeUpstreamTwice(t *testing.T) {
	upstream := &kiroCompatUpstream{t: t}
	cache := &kiroMemoryCache{values: map[string][]byte{}}
	groupID := int64(7)
	group := &Group{ID: groupID, KiroResponseCache: KiroCachePolicy{Enabled: true, TargetPercent: 100, TTLSeconds: 300, MaxEntries: 128}}
	svc := &GatewayService{httpUpstream: upstream, cfg: &config.Config{}, cache: cache, groupRepo: &kiroGroupRepo{group: group}}
	a := fixedKiroAccount()
	a.Credentials["access_token"] = t.Name()
	a.Credentials["expires_at"] = time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	a.Extra = map[string]any{"kiro_response_cache": map[string]any{"enabled": true, "target_percent": 100}}
	raw := []byte(`{"model":"claude-haiku-4.5","messages":[{"role":"user","content":"hello"}],"stream":false}`)
	for i := 0; i < 2; i++ {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(raw))
		c.Set("api_key", &APIKey{ID: 1, UserID: 2, GroupID: &groupID})
		result, err := svc.ForwardAsChatCompletions(context.Background(), c, a, raw, nil)
		require.NoError(t, err)
		require.Contains(t, recorder.Body.String(), "OK")
		if i == 1 {
			require.Zero(t, result.Usage.InputTokens)
			require.Zero(t, result.Usage.OutputTokens)
			require.Equal(t, "hit", recorder.Header().Get("X-Sub2api-Cache"))
		}
	}
	require.Equal(t, 2, upstream.requests, "one model query and one generation only")
}

// This fixture is available with and without the unit build tag.
type kiroGroupRepo struct {
	GroupRepository
	group *Group
}

func (r *kiroGroupRepo) GetByID(_ context.Context, _ int64) (*Group, error) { return r.group, nil }
