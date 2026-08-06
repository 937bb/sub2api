package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type openAIWSPrewarmCaptureConn struct {
	mu       sync.Mutex
	writes   []map[string]any
	events   [][]byte
	closed   bool
	writeErr error
}

func (c *openAIWSPrewarmCaptureConn) WriteJSON(_ context.Context, value any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("closed")
	}
	if c.writeErr != nil {
		return c.writeErr
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		return err
	}
	c.writes = append(c.writes, payload)
	return nil
}

func (c *openAIWSPrewarmCaptureConn) ReadMessage(_ context.Context) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, errors.New("closed")
	}
	if len(c.events) == 0 {
		return nil, errors.New("no queued event")
	}
	event := c.events[0]
	c.events = c.events[1:]
	return event, nil
}

func (c *openAIWSPrewarmCaptureConn) Ping(context.Context) error { return nil }

func (c *openAIWSPrewarmCaptureConn) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	return nil
}

func TestPrewarmOpenAIWSSessionMarksConnectionAfterTerminalEvent(t *testing.T) {
	conn := &openAIWSPrewarmCaptureConn{
		events: [][]byte{
			[]byte(`{"type":"response.created","response":{"id":"resp_warmup"}}`),
			[]byte(`{"type":"response.completed","response":{"id":"resp_warmup"}}`),
		},
	}
	pooled := newOpenAIWSConn("prewarm-test", 1, conn, http.Header{"X-Test": []string{"ok"}})
	require.True(t, pooled.tryAcquire())
	lease := &openAIWSConnLease{conn: pooled, accountID: 1}
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	account := &Account{ID: 1, Platform: PlatformOpenAI}

	err := svc.prewarmOpenAIWSSession(
		context.Background(),
		lease,
		[]byte(`{"type":"response.create","model":"gpt-5.3-codex","input":[{"type":"input_text","text":"hello"}]}`),
		account,
		nil,
	)
	require.NoError(t, err)
	require.True(t, lease.IsPrewarmed())
	require.Len(t, conn.writes, 1)
	require.Equal(t, false, conn.writes[0]["generate"])
}

func TestPrewarmOpenAIWSSessionSkipsContinuationPayload(t *testing.T) {
	conn := &openAIWSPrewarmCaptureConn{
		events: [][]byte{[]byte(`{"type":"response.completed"}`)},
	}
	pooled := newOpenAIWSConn("prewarm-test", 1, conn, nil)
	require.True(t, pooled.tryAcquire())
	lease := &openAIWSConnLease{conn: pooled, accountID: 1}
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	account := &Account{ID: 1, Platform: PlatformOpenAI}

	err := svc.prewarmOpenAIWSSession(
		context.Background(),
		lease,
		[]byte(`{"type":"response.create","previous_response_id":"resp_previous","input":[]}`),
		account,
		nil,
	)
	require.NoError(t, err)
	require.False(t, lease.IsPrewarmed())
	require.Empty(t, conn.writes)
}

func TestPrewarmOpenAIWSSessionReturnsFailoverOnRateLimit(t *testing.T) {
	conn := &openAIWSPrewarmCaptureConn{
		events: [][]byte{
			[]byte(`{"type":"error","error":{"type":"rate_limit_error","code":"rate_limit_exceeded","message":"rate limit exceeded"}}`),
		},
	}
	pooled := newOpenAIWSConn("prewarm-test", 1, conn, nil)
	require.True(t, pooled.tryAcquire())
	lease := &openAIWSConnLease{conn: pooled, accountID: 1}
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	account := &Account{ID: 1, Platform: PlatformOpenAI}

	err := svc.prewarmOpenAIWSSession(
		context.Background(),
		lease,
		[]byte(`{"type":"response.create","model":"gpt-5.3-codex","input":[]}`),
		account,
		nil,
	)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusTooManyRequests, failoverErr.StatusCode)
	require.False(t, lease.IsPrewarmed())
}

func TestPrewarmOpenAIWSPassthroughSessionReturnsFailoverOnRateLimit(t *testing.T) {
	conn := &openAIWSPrewarmCaptureConn{
		events: [][]byte{
			[]byte(`{"type":"error","error":{"type":"rate_limit_error","code":"rate_limit_exceeded","message":"rate limit exceeded"}}`),
		},
	}
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	account := &Account{ID: 1, Platform: PlatformOpenAI}
	hooks := &OpenAIWSIngressHooks{QuotaBypassEnabled: true}
	headers := http.Header{"Retry-After": []string{"10"}}

	err := svc.prewarmOpenAIWSPassthroughSession(
		context.Background(),
		conn,
		[]byte(`{"type":"response.create","model":"gpt-5.3-codex","input":[]}`),
		account,
		hooks,
		headers,
	)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusTooManyRequests, failoverErr.StatusCode)
	require.Equal(t, "10", failoverErr.ResponseHeaders.Get("Retry-After"))
	require.Len(t, conn.writes, 1)
	require.Equal(t, false, conn.writes[0]["generate"])
}

func TestQuotaBypassPrewarmPayloadDoesNotDuplicateExistingToolOutput(t *testing.T) {
	hooks := &OpenAIWSIngressHooks{QuotaBypassEnabled: true}
	payload := []byte(`{"model":"gpt-5.3-codex","input":[{"type":"function_call_output","call_id":"call_existing","output":"ok"}]}`)
	updated := quotaBypassPrewarmPayload(payload, hooks)
	require.Equal(t, string(payload), string(updated))
}

func TestPrewarmOpenAIWSSessionChecksContinuationBeforeQuotaInjection(t *testing.T) {
	conn := &openAIWSPrewarmCaptureConn{
		events: [][]byte{[]byte(`{"type":"response.completed","response":{"id":"resp_warmup"}}`)},
	}
	pooled := newOpenAIWSConn("prewarm-test", 1, conn, nil)
	require.True(t, pooled.tryAcquire())
	lease := &openAIWSConnLease{conn: pooled, accountID: 1}
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	account := &Account{ID: 1, Platform: PlatformOpenAI}
	hooks := &OpenAIWSIngressHooks{QuotaBypassEnabled: true}

	err := svc.prewarmOpenAIWSSession(
		context.Background(),
		lease,
		[]byte(`{"type":"response.create","model":"gpt-5.3-codex","input":[{"type":"message","role":"user","content":"hello"}]}`),
		account,
		hooks,
	)
	require.NoError(t, err)
	require.True(t, lease.IsPrewarmed())
	require.Len(t, conn.writes, 1)
	require.Equal(t, false, conn.writes[0]["generate"])
	encoded, marshalErr := json.Marshal(conn.writes[0])
	require.NoError(t, marshalErr)
	input := gjson.GetBytes(encoded, "input").Array()
	require.Len(t, input, 3)
	require.Equal(t, "function_call", input[1].Get("type").String())
	require.Equal(t, "function_call_output", input[2].Get("type").String())
}
