package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBuildOpenAICompactSSEPayload_PreservesItemsAndSanitizesCodexFields(t *testing.T) {
	payload, ok := buildOpenAICompactSSEPayload([]byte(`{"id":"","output":[{"type":"compaction","encrypted_content":"opaque"},{"type":"message","content":[]}],"usage":{"input_tokens":1}}`))
	require.True(t, ok)
	frames := strings.Split(strings.TrimSpace(string(payload)), "\n\n")
	require.Len(t, frames, 3)
	require.Contains(t, frames[0], `"output_index":0`)
	require.Contains(t, frames[0], `"encrypted_content":"opaque"`)
	completedData := strings.SplitN(frames[2], "\ndata: ", 2)[1]
	var completed map[string]any
	require.NoError(t, json.Unmarshal([]byte(completedData), &completed))
	response := completed["response"].(map[string]any)
	require.Regexp(t, `^resp_[0-9a-f]+$`, response["id"])
	require.NotContains(t, response, "usage")
}

func TestOpenAIGatewayService_OAuthBodySignalStreamBridgesUnaryJSONToSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{}`))
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.136.0")
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"resp_compact","output":[{"type":"compaction","encrypted_content":"opaque"}],"usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}`))}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{ID: 79, Name: "oauth-codex", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1, Credentials: map[string]any{"access_token": "oauth-token"}}
	body := []byte(`{"model":"gpt-5.5","stream":true,"input":[{"type":"compaction_trigger"}]}`)
	_, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
	require.Contains(t, rec.Body.String(), "event: response.output_item.done")
	require.Contains(t, rec.Body.String(), `"encrypted_content":"opaque"`)
	require.Contains(t, rec.Body.String(), "event: response.completed")
}

func TestOpenAICompactKeepalive_AdjustedSizeIgnoresHeartbeatAndSerializesWrites(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set(openAICompactClientStreamKey, true)
	stop := startOpenAICompactSSEKeepalive(c, time.Millisecond)
	t.Cleanup(stop)
	require.Eventually(t, func() bool { return strings.Contains(rec.Body.String(), ": keepalive") }, time.Second, time.Millisecond)
	require.Equal(t, -1, openAICompactKeepaliveAdjustedWrittenSize(c))
	_, err := c.Writer.Write([]byte("semantic"))
	require.NoError(t, err)
	require.Greater(t, openAICompactKeepaliveAdjustedWrittenSize(c), 0)
	before := rec.Body.String()
	time.Sleep(5 * time.Millisecond)
	require.Equal(t, before, rec.Body.String())
}

func TestOpenAIGatewayService_APIKeyBodySignalDoesNotBridge(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{}`))
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"resp_native","output":[],"usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}`))}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{ID: 80, Name: "apikey", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{"api_key": "sk-test"}}
	body := []byte(`{"model":"gpt-5.5","stream":false,"input":[{"type":"compaction_trigger"}]}`)
	_, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.Equal(t, "/v1/responses", c.Request.URL.Path)
	require.NotContains(t, rec.Body.String(), "event: response.completed")
}
