package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type anthropicCompatRetryUpstream struct {
	requestBodies [][]byte
}

func (u *anthropicCompatRetryUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	body, _ := io.ReadAll(req.Body)
	u.requestBodies = append(u.requestBodies, append([]byte(nil), body...))
	if len(u.requestBodies) == 1 {
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(bytes.NewBufferString(
				`{"type":"error","error":{"type":"invalid_request_error","message":"thinking.enabled is not supported; use thinking.adaptive and output_config.effort"}}`,
			)),
		}, nil
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(`{"id":"msg_ok","type":"message","usage":{"input_tokens":3,"output_tokens":1}}`)),
	}, nil
}

func (u *anthropicCompatRetryUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

func TestRewriteAnthropicHistoricalToolUseIDsKeepsPairsStable(t *testing.T) {
	body := []byte(`{"model":"claude","messages":[{"role":"assistant","content":[{"type":"tool_use","id":"call_same","name":"lookup","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_same","content":"ok"}]}]}`)

	rewritten, changed := rewriteAnthropicHistoricalToolUseIDs(body)
	require.True(t, changed)
	toolUseID := gjson.GetBytes(rewritten, "messages.0.content.0.id").String()
	require.Contains(t, toolUseID, "toolu_")
	require.Equal(t, toolUseID, gjson.GetBytes(rewritten, "messages.1.content.0.tool_use_id").String())

	rewrittenAgain, changedAgain := rewriteAnthropicHistoricalToolUseIDs(body)
	require.True(t, changedAgain)
	require.JSONEq(t, string(rewritten), string(rewrittenAgain))
}

func TestRewriteAnthropicAdaptiveThinking(t *testing.T) {
	body := []byte(`{"thinking":{"type":"enabled","budget_tokens":8192},"messages":[]}`)

	rewritten, changed := rewriteAnthropicAdaptiveThinking(body)
	require.True(t, changed)
	require.Equal(t, "adaptive", gjson.GetBytes(rewritten, "thinking.type").String())
	require.False(t, gjson.GetBytes(rewritten, "thinking.budget_tokens").Exists())
	require.Equal(t, "high", gjson.GetBytes(rewritten, "output_config.effort").String())
}

func TestAnthropicPassthroughRetriesExactAdaptiveThinkingCompatibilityError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	upstream := &anthropicCompatRetryUpstream{}
	svc := &GatewayService{
		cfg:              &config.Config{},
		httpUpstream:     upstream,
		rateLimitService: &RateLimitService{},
	}
	body := []byte(`{"model":"claude-3-5-sonnet-latest","thinking":{"type":"enabled","budget_tokens":4096},"messages":[{"role":"user","content":"hello"}]}`)

	result, err := svc.forwardAnthropicAPIKeyPassthrough(context.Background(), c, newAnthropicAPIKeyAccountForTest(), body, "claude-3-5-sonnet-latest", "claude-3-5-sonnet-latest", false, time.Now())

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.requestBodies, 2)
	require.Equal(t, "enabled", gjson.GetBytes(upstream.requestBodies[0], "thinking.type").String())
	require.Equal(t, "adaptive", gjson.GetBytes(upstream.requestBodies[1], "thinking.type").String())
	require.False(t, gjson.GetBytes(upstream.requestBodies[1], "thinking.budget_tokens").Exists())
	require.Equal(t, "high", gjson.GetBytes(upstream.requestBodies[1], "output_config.effort").String())
	require.Equal(t, http.StatusOK, recorder.Code)
}
