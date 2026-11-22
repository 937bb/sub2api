package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestReadOpenAIWSWithKeepaliveDoesNotRestartRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ticks := make(chan time.Time, 3)
	for range 3 {
		ticks <- time.Now()
	}
	release := make(chan struct{})
	heartbeats := 0
	message, err := readOpenAIWSWithKeepalive(ctx, func(ctx context.Context) ([]byte, error) {
		select {
		case <-release:
			return []byte("complete"), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}, ticks, func() {
		heartbeats++
		if heartbeats == 3 {
			close(release)
		}
	})
	require.NoError(t, err)
	require.Equal(t, "complete", string(message))
	require.Equal(t, 3, heartbeats)
}

func TestReadOpenAIWSWithKeepaliveCancellationJoinsRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	ticks := make(chan time.Time, 1)
	ticks <- time.Now()
	_, err := readOpenAIWSWithKeepalive(ctx, func(ctx context.Context) ([]byte, error) {
		<-ctx.Done()
		close(finished)
		return nil, ctx.Err()
	}, ticks, cancel)
	require.ErrorIs(t, err, context.Canceled)
	select {
	case <-finished:
	default:
		t.Fatal("read must exit before returning its connection lease")
	}
}

func TestReadOpenAIWSWithKeepaliveDoesNotExtendReadDeadline(t *testing.T) {
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	started := time.Now()
	beats := 0
	_, err := readOpenAIWSWithKeepalive(context.Background(), func(ctx context.Context) ([]byte, error) {
		readCtx, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
		defer cancel()
		<-readCtx.Done()
		return nil, readCtx.Err()
	}, ticker.C, func() { beats++ })
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Positive(t, beats)
	require.Less(t, time.Since(started), time.Second)
}

type failedKeepaliveWriter struct {
	*httptest.ResponseRecorder
	writes int
}

func (w *failedKeepaliveWriter) Write([]byte) (int, error) {
	w.writes++
	return 0, io.ErrClosedPipe
}

func TestForwardOpenAIWSV2_FailedKeepaliveDrainsUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := newOpenAIWSV2TestConfig()
	cfg.Gateway.StreamKeepaliveInterval = 1
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 5
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	conn := &openAIWSCaptureConn{
		events:     [][]byte{[]byte(`{"type":"response.completed","response":{"id":"resp_drain","model":"gpt-5.5","status":"completed","usage":{"input_tokens":3,"output_tokens":5}}}`)},
		readDelays: []time.Duration{2200 * time.Millisecond},
	}
	pool := newOpenAIWSConnPool(cfg)
	defer pool.Close()
	pool.setClientDialerForTest(&openAIWSQueueDialer{conns: []openAIWSClientConn{conn}})
	svc := &OpenAIGatewayService{cfg: cfg, cache: &stubGatewayCache{}, toolCorrector: NewCodexToolCorrector(), openaiWSPool: pool, openaiWSResolver: NewOpenAIWSProtocolResolver(cfg)}
	account := &Account{ID: 91732, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, Credentials: map[string]any{"api_key": "sk-test"}, Extra: map[string]any{"responses_websockets_v2_enabled": true}}
	writer := &failedKeepaliveWriter{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(writer)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	result, err := svc.Forward(c.Request.Context(), c, account, []byte(`{"model":"gpt-5.5","stream":true,"input":[{"role":"user","content":"hello"}]}`))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.ClientDisconnect)
	require.Equal(t, 3, result.Usage.InputTokens)
	require.Equal(t, 5, result.Usage.OutputTokens)
	require.Equal(t, 1, writer.writes, "a failed heartbeat must stop all subsequent downstream writes")
}

func TestForwardOpenAIWSV2_SSEKeepaliveAndRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, retry := range []bool{false, true} {
		name := "delayed_output"
		if retry {
			name = "retry_after_heartbeat"
		}
		t.Run(name, func(t *testing.T) {
			cfg := newOpenAIWSV2TestConfig()
			cfg.Gateway.StreamKeepaliveInterval = 1
			cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 5
			cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
			cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
			cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
			created := []byte(`{"type":"response.created","response":{"id":"resp_old","model":"gpt-5.5"}}`)
			complete := []byte(`{"type":"response.completed","response":{"id":"resp_ok","model":"gpt-5.5","status":"completed","usage":{"input_tokens":2,"output_tokens":1}}}`)
			delta := []byte(`{"type":"response.output_text.delta","delta":"hello"}`)
			first := &openAIWSCaptureConn{events: [][]byte{created, delta, complete}, readDelays: []time.Duration{0, 1200 * time.Millisecond, 1500 * time.Millisecond}}
			conns := []openAIWSClientConn{first}
			if retry {
				first.events = [][]byte{created, []byte(`{"type":"error","error":{"code":"server_error","message":"temporary upstream failure"}}`)}
				conns = append(conns, &openAIWSCaptureConn{events: [][]byte{complete}})
			}
			dialer := &openAIWSQueueDialer{conns: conns}
			pool := newOpenAIWSConnPool(cfg)
			defer pool.Close()
			pool.setClientDialerForTest(dialer)
			svc := &OpenAIGatewayService{cfg: cfg, cache: &stubGatewayCache{}, toolCorrector: NewCodexToolCorrector(), openaiWSPool: pool, openaiWSResolver: NewOpenAIWSProtocolResolver(cfg)}
			account := &Account{ID: 91731, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, Credentials: map[string]any{"api_key": "sk-test"}, Extra: map[string]any{"responses_websockets_v2_enabled": true}}
			errs := make(chan error, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				c, _ := gin.CreateTestContext(w)
				c.Request = r
				_, err := svc.Forward(r.Context(), c, account, []byte(`{"model":"gpt-5.5","stream":true,"input":[{"role":"user","content":"hello"}]}`))
				errs <- err
			}))
			defer server.Close()
			client := server.Client()
			client.Timeout = 8 * time.Second
			resp, err := client.Post(server.URL+"/v1/responses", "application/json", strings.NewReader("{}"))
			require.NoError(t, err)
			defer resp.Body.Close()
			prefix := make([]byte, 3)
			_, err = io.ReadFull(resp.Body, prefix)
			require.NoError(t, err)
			require.Equal(t, ":\n\n", string(prefix), "heartbeat must flush before buffered lifecycle events")
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.NoError(t, <-errs)
			require.Contains(t, string(body), `"type":"response.completed"`)
			require.Equal(t, 1, strings.Count(string(body), `"type":"response.completed"`))
			if retry {
				require.NotContains(t, string(body), "resp_old", "failed attempt metadata must stay private")
				require.NotContains(t, string(body), "temporary upstream failure")
				require.Equal(t, 2, dialer.DialCount())
			} else {
				require.Contains(t, string(body), ":\n\n", "heartbeat must also cover gaps after output begins")
				require.Less(t, strings.Index(string(body), `"delta":"hello"`), strings.LastIndex(string(body), ":\n\n"))
			}
		})
	}
}
