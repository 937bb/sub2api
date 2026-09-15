package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type heartbeatDelayDialer struct {
	base   *wsMappedRetryDialer
	delay  time.Duration
	active atomic.Int32
}

type heartbeatDelayConn struct {
	openAIWSClientConn
	owner   *heartbeatDelayDialer
	delayed bool
}

func (d *heartbeatDelayDialer) Dial(ctx context.Context, url string, headers http.Header, proxy string) (openAIWSClientConn, int, http.Header, error) {
	conn, status, h, err := d.base.Dial(ctx, url, headers, proxy)
	if err != nil {
		return conn, status, h, err
	}
	return &heartbeatDelayConn{openAIWSClientConn: conn, owner: d}, status, h, nil
}

func (c *heartbeatDelayConn) ReadMessage(ctx context.Context) ([]byte, error) {
	c.owner.active.Add(1)
	defer c.owner.active.Add(-1)
	if !c.delayed {
		c.delayed = true
		timer := time.NewTimer(c.owner.delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return c.openAIWSClientConn.ReadMessage(ctx)
}

func withDelayedWSHeartbeat(svc *OpenAIGatewayService, base *wsMappedRetryDialer, delay time.Duration) *heartbeatDelayDialer {
	svc.cfg.Gateway.StreamKeepaliveInterval = 1
	d := &heartbeatDelayDialer{base: base, delay: delay}
	svc.openaiWSPool.setClientDialerForTest(d)
	return d
}

func TestWSHeartbeatPreservesSlowResponseAndTimeToFirstToken(t *testing.T) {
	svc, account, c, rec, base := newWSMappedRetryFixture(t, OAuthRetrySettings{Enabled: true, MaxRetries: 5, StatusCodes: []int{502}}, wsRetrySuccessfulEvents())
	d := withDelayedWSHeartbeat(svc, base, 2200*time.Millisecond)
	result, err := svc.Forward(c.Request.Context(), c, account, []byte(`{"model":"gpt-5.6-sol","stream":true,"input":"hello"}`))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.GreaterOrEqual(t, strings.Count(rec.Body.String(), ":\n\n"), 2)
	require.True(t, strings.HasPrefix(rec.Body.String(), ":\n\n"))
	require.NotNil(t, result.FirstTokenMs)
	require.GreaterOrEqual(t, *result.FirstTokenMs, 2100, "heartbeat is not the model's first token")
	require.Equal(t, 1, base.requestCount())
	require.EqualValues(t, 2, result.Usage.OutputTokens)
	require.Zero(t, d.active.Load(), "reader must be joined before releasing the lease")
}

func TestWSHeartbeatKeepsMappedRetryPrivate(t *testing.T) {
	for _, event := range []string{wsRetryOverloadEvent, wsRetryFailedEvent} {
		t.Run(fmt.Sprint(event == wsRetryFailedEvent), func(t *testing.T) {
			svc, account, c, rec, base := newWSMappedRetryFixture(t, OAuthRetrySettings{Enabled: true, MaxRetries: 1, StatusCodes: []int{502}}, [][]byte{[]byte(event)}, wsRetrySuccessfulEvents())
			d := withDelayedWSHeartbeat(svc, base, 1100*time.Millisecond)
			result, err := svc.Forward(c.Request.Context(), c, account, []byte(`{"model":"gpt-5.6-sol","stream":true,"input":"hello"}`))
			require.NoError(t, err)
			require.True(t, result.OpenAIWSMode)
			require.Equal(t, 2, base.requestCount())
			require.GreaterOrEqual(t, strings.Count(rec.Body.String(), ":\n\n"), 2)
			require.NotContains(t, rec.Body.String(), "overloaded")
			require.NotContains(t, rec.Body.String(), "response.failed")
			require.Equal(t, 1, strings.Count(rec.Body.String(), `"type":"response.completed"`))
			require.Zero(t, d.active.Load())
		})
	}
}

func TestWSHeartbeatExhaustionStillAllowsNextAccount(t *testing.T) {
	svc, account, c, rec, base := newWSMappedRetryFixture(t, OAuthRetrySettings{Enabled: true, MaxRetries: 0, StatusCodes: []int{502}}, [][]byte{[]byte(wsRetryOverloadEvent)}, wsRetrySuccessfulEvents())
	withDelayedWSHeartbeat(svc, base, 1100*time.Millisecond)
	_, err := svc.Forward(c.Request.Context(), c, account, []byte(`{"model":"gpt-5.6-sol","stream":true,"input":"hello"}`))
	var failover *UpstreamFailoverError
	require.ErrorAs(t, err, &failover)
	require.True(t, failover.ShouldRetryNextAccount())
	require.False(t, failover.RetryableOnSameAccount)
	require.Equal(t, ":\n\n", rec.Body.String())
	require.Equal(t, -1, OpenAIStreamAdjustedWrittenSize(c))
	require.False(t, OpenAIStreamHasCommittedOutput(c))
	next := *account
	next.ID++
	result, err := svc.Forward(c.Request.Context(), c, &next, []byte(`{"model":"gpt-5.6-sol","stream":true,"input":"hello"}`))
	require.NoError(t, err)
	require.True(t, result.OpenAIWSMode)
	require.Equal(t, 2, base.requestCount())
	require.NotContains(t, rec.Body.String(), "overloaded")
}

func TestWSHeartbeatTransportRecoveryAndFallback(t *testing.T) {
	for _, recovery := range []bool{false, true} {
		t.Run(fmt.Sprint(recovery), func(t *testing.T) {
			second := [][]byte(nil)
			if recovery {
				second = wsRetrySuccessfulEvents()
			}
			svc, account, c, rec, base := newWSMappedRetryFixture(t, OAuthRetrySettings{Enabled: true, MaxRetries: 5, StatusCodes: []int{502}}, nil, second)
			d := withDelayedWSHeartbeat(svc, base, 1100*time.Millisecond)
			upstream := svc.httpUpstream.(*httpUpstreamRecorder)
			upstream.resp = wsTransportHTTPResponse(200)
			result, err := svc.Forward(c.Request.Context(), c, account, []byte(`{"model":"gpt-5.6-sol","stream":true,"input":"hello"}`))
			require.NoError(t, err)
			require.Equal(t, recovery, result.OpenAIWSMode)
			require.Equal(t, 2, base.requestCount())
			require.Contains(t, rec.Body.String(), ":\n\n")
			require.NotContains(t, rec.Body.String(), "response.failed")
			if recovery {
				require.Empty(t, upstream.requests)
			} else {
				require.Len(t, upstream.requests, 1)
			}
			require.Zero(t, d.active.Load())
		})
	}
}

func TestWSHeartbeatFinalHTTPErrorIsSSETerminal(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			svc, account, c, rec, base := newWSMappedRetryFixture(t, OAuthRetrySettings{Enabled: enabled, MaxRetries: 1, StatusCodes: []int{502}}, nil)
			withDelayedWSHeartbeat(svc, base, 1100*time.Millisecond)
			svc.httpUpstream.(*httpUpstreamRecorder).resp = wsTransportHTTPResponse(http.StatusBadRequest)
			_, err := svc.Forward(c.Request.Context(), c, account, []byte(`{"model":"gpt-5.6-sol","stream":true,"input":"hello"}`))
			require.Error(t, err)
			require.Equal(t, http.StatusOK, rec.Code)
			require.True(t, strings.HasPrefix(rec.Body.String(), ":\n\n"))
			require.Equal(t, 1, strings.Count(rec.Body.String(), "event: response.failed"))
			require.Contains(t, rec.Body.String(), "upstream unavailable")
			require.True(t, IsResponseCommitted(c))
		})
	}
}

func TestWSHeartbeatCancellationJoinsReaderWithoutRetry(t *testing.T) {
	svc, account, c, rec, base := newWSMappedRetryFixture(t, OAuthRetrySettings{Enabled: true, MaxRetries: 10, StatusCodes: []int{502}}, wsRetrySuccessfulEvents())
	d := withDelayedWSHeartbeat(svc, base, time.Minute)
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()
	timer := time.AfterFunc(1200*time.Millisecond, cancel)
	defer timer.Stop()
	c.Request = c.Request.WithContext(ctx)
	_, err := svc.Forward(ctx, c, account, []byte(`{"model":"gpt-5.6-sol","stream":true,"input":"hello"}`))
	require.Error(t, err)
	require.Equal(t, 1, base.requestCount())
	require.Equal(t, ":\n\n", rec.Body.String())
	require.Empty(t, svc.httpUpstream.(*httpUpstreamRecorder).requests)
	require.Zero(t, d.active.Load())
}

func TestWSStreamReadErrorPreservesCauseAndSanitizesClientMessage(t *testing.T) {
	svc, account, c, _, base := newWSMappedRetryFixture(t, OAuthRetrySettings{Enabled: true, MaxRetries: 5, StatusCodes: []int{502}}, [][]byte{[]byte(`{"type":"response.output_text.delta","delta":"partial answer"}`)})
	result, err := svc.Forward(c.Request.Context(), c, account, []byte(`{"model":"gpt-5.6-sol","stream":true,"input":"hello"}`))
	require.Error(t, err)
	require.NotNil(t, result)
	code, message, ok := OpenAIUpstreamStreamReadErrorDetails(err)
	require.True(t, ok)
	require.Equal(t, OpenAIUpstreamWSStreamErrorCode, code)
	require.Equal(t, "Upstream WebSocket stream ended before completion", message)
	require.Equal(t, 1, base.requestCount())
}

func TestWSCapacityNeverReplaysObservedNonstreamOutputWithoutMappedRetry(t *testing.T) {
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeSetupToken} {
		for _, prefix := range []string{
			`{"type":"response.output_text.delta","delta":"partial answer"}`,
			`{"type":"response.output_item.added","item":{"type":"function_call","call_id":"call_1","name":"tool","arguments":""}}`,
			`{"type":"response.in_progress","response":{"usage":{"input_tokens":5,"output_tokens":0}}}`,
		} {
			t.Run(accountType+prefix, func(t *testing.T) {
				svc, account, conn, c, rec, _ := codexWSCapacityFixture(t, accountType, [][]byte{[]byte(prefix), []byte(wsRetryOverloadEvent)})
				svc.cfg.Gateway.StreamKeepaliveInterval = 1
				_, err := svc.Forward(c.Request.Context(), c, account, []byte(`{"model":"gpt-5.6-sol","stream":false,"input":"hello"}`))
				require.Error(t, err)
				var failover *UpstreamFailoverError
				require.False(t, errors.As(err, &failover))
				require.Len(t, conn.writes, 1)
				require.False(t, OpenAIStreamHeartbeatPresent(c))
				require.NotContains(t, rec.Body.String(), ":\n\n")
			})
		}
	}
}

func TestWSHeartbeatReaderJoinsOnWriteFailureAndPanic(t *testing.T) {
	for _, panicWriter := range []bool{false, true} {
		t.Run(fmt.Sprint(panicWriter), func(t *testing.T) {
			d := &heartbeatDelayDialer{delay: time.Minute}
			client := &heartbeatDelayConn{owner: d, openAIWSClientConn: &openAIWSCaptureConn{events: wsRetrySuccessfulEvents()}}
			lease := &openAIWSConnLease{conn: newOpenAIWSConn("heartbeat-join", 1, client, nil)}
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
			start := time.Now()
			call := func() {
				_, err := readOpenAIWSMessageWithHeartbeat(context.Background(), lease, time.Minute, ticker.C, func() bool {
					if panicWriter {
						panic("writer panic")
					}
					return false
				})
				require.ErrorIs(t, err, context.Canceled)
			}
			if panicWriter {
				require.Panics(t, call)
			} else {
				call()
			}
			require.Less(t, time.Since(start), time.Second)
			require.Zero(t, d.active.Load())
		})
	}
}

func TestWSHeartbeatCapacity503UsesConfiguredBudget(t *testing.T) {
	svc, account, c, rec, base := newWSMappedRetryFixture(t, OAuthRetrySettings{Enabled: true, MaxRetries: 1, StatusCodes: []int{503}}, [][]byte{[]byte(wsRetryOverloadEvent)}, wsRetrySuccessfulEvents())
	withDelayedWSHeartbeat(svc, base, 1100*time.Millisecond)
	result, err := svc.Forward(c.Request.Context(), c, account, []byte(`{"model":"gpt-5.6-sol","stream":true,"input":"hello"}`))
	require.NoError(t, err)
	require.True(t, result.OpenAIWSMode)
	require.Equal(t, 2, base.requestCount())
	require.NotContains(t, rec.Body.String(), "overloaded")
	require.Contains(t, rec.Body.String(), ":\n\n")
}

func TestWSHeartbeatRetryGuardPreservesExistingCompactKeepalive(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", nil)
	MarkOpenAICompactClientStream(c)
	stop := StartOpenAICompactSSEKeepalive(c, 5*time.Millisecond)
	defer stop()
	snapshotOAuthRetryResponse(c, c.Writer)
	require.Eventually(t, c.Writer.Written, time.Second, time.Millisecond, "read-only retry bookkeeping must not stop compact keepalive")
	stop()
	require.Contains(t, rec.Body.String(), ": keepalive")
}
