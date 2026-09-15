package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func wsTransportHTTPResponse(status int) *http.Response {
	body := `{"error":{"type":"server_error","message":"upstream unavailable"}}`
	if status == 200 {
		body = `data: {"type":"response.completed","response":{"id":"resp_http","status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"http success"}]}],"usage":{"input_tokens":3,"output_tokens":2}}}` + "\n\n"
	}
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body))}
}

func wsTransportHTTPStreamFailure() *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: " + wsRetryFailedEvent + "\n\n"))}
}

func TestForwardWSTransportRecoveryStaysWSAndPreservesReuse(t *testing.T) {
	svc, account, c, rec, dialer := newWSMappedRetryFixture(t, OAuthRetrySettings{Enabled: true, MaxRetries: 5, StatusCodes: []int{502}}, [][]byte{[]byte(`{"type":"codex.rate_limits"}`), []byte(`{"type":"codex.response.metadata"}`)}, wsRetrySuccessfulEvents())
	// Reuse is within one identified conversation. Separate anonymous HTTP
	// requests intentionally receive different conversation identities.
	c.Request.Header.Set("Session_id", "transport-recovery-conversation")
	result, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.6-sol","stream":true,"input":"hello"}`))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.OpenAIWSMode)
	require.Equal(t, 2, dialer.requestCount())
	require.Empty(t, svc.httpUpstream.(*httpUpstreamRecorder).requests)
	require.NotContains(t, rec.Body.String(), "codex.rate_limits")
	require.Equal(t, 1, strings.Count(rec.Body.String(), `"delta":"successful answer"`))
	// The recovered connection remains eligible for reuse by the next request.
	dialer.mu.Lock()
	conn := dialer.conns[len(dialer.conns)-1]
	dialer.mu.Unlock()
	conn.mu.Lock()
	conn.events = append(conn.events, wsRetrySuccessfulEvents()...)
	conn.mu.Unlock()
	rec2 := newOpenAIResponseFlushRecorder()
	c2, _ := gin.CreateTestContext(rec2)
	c2.Request = c.Request.Clone(context.Background())
	result, err = svc.Forward(context.Background(), c2, account, []byte(`{"model":"gpt-5.6-sol","stream":true,"input":"hello"}`))
	require.NoError(t, err)
	require.True(t, result.OpenAIWSMode)
	require.Len(t, dialer.conns, 2)
	require.Equal(t, 3, dialer.requestCount())
}

func TestForwardWSTransportFallbackBudget(t *testing.T) {
	for _, tc := range []struct{ budget, wantWS, wantHTTP int }{{0, 1, 1}, {1, 1, 1}, {5, 2, 4}} {
		t.Run(fmt.Sprint(tc.budget), func(t *testing.T) {
			svc, account, c, _, dialer := newWSMappedRetryFixture(t, OAuthRetrySettings{Enabled: true, MaxRetries: tc.budget, StatusCodes: []int{502}}, nil)
			upstream := svc.httpUpstream.(*httpUpstreamRecorder)
			for i := 0; i < tc.wantHTTP; i++ {
				upstream.responses = append(upstream.responses, wsTransportHTTPStreamFailure())
			}
			upstream.resp = wsTransportHTTPStreamFailure()
			_, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.6-sol","stream":true,"input":"hello"}`))
			require.Error(t, err)
			require.Equal(t, tc.wantWS, dialer.requestCount())
			require.Len(t, upstream.requests, tc.wantHTTP)
		})
	}
}

func TestForwardWSTransportFallsBackToWorkingHTTP(t *testing.T) {
	svc, account, c, rec, dialer := newWSMappedRetryFixture(t, OAuthRetrySettings{Enabled: true, MaxRetries: 5, StatusCodes: []int{502}}, nil)
	upstream := svc.httpUpstream.(*httpUpstreamRecorder)
	upstream.resp = wsTransportHTTPResponse(200)
	result, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.6-sol","stream":true,"input":"hello"}`))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.OpenAIWSMode)
	require.EqualValues(t, 2, result.Usage.OutputTokens)
	require.Equal(t, 2, dialer.requestCount())
	require.Len(t, upstream.requests, 1)
	require.Contains(t, rec.Body.String(), "http success")
}

func TestForwardWSTransportDoesNotReplayObservedWork(t *testing.T) {
	for _, prefix := range []string{`{"type":"response.output_text.delta","delta":"partial"}`, `{"type":"response.output_item.added","item":{"type":"function_call","name":"tool","call_id":"call_1"}}`, `{"type":"response.in_progress","response":{"usage":{"input_tokens":3,"output_tokens":0}}}`} {
		for _, malformed := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s_malformed_%v", prefix, malformed), func(t *testing.T) {
				events := [][]byte{[]byte(prefix)}
				if malformed {
					events = append(events, []byte(`{"broken"`))
				}
				svc, account, c, _, dialer := newWSMappedRetryFixture(t, OAuthRetrySettings{Enabled: true, MaxRetries: 5, StatusCodes: []int{502}}, events, wsRetrySuccessfulEvents())
				result, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.6-sol","stream":false,"input":"hello"}`))
				require.Error(t, err)
				require.NotNil(t, result)
				require.Equal(t, 1, dialer.requestCount())
				require.Empty(t, svc.httpUpstream.(*httpUpstreamRecorder).requests)
			})
		}
	}
}

func TestForwardWSTransportCancellationDoesNotFallback(t *testing.T) {
	svc, account, c, _, dialer := newWSMappedRetryFixture(t, OAuthRetrySettings{Enabled: true, MaxRetries: 5, StatusCodes: []int{502}}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dialer.onRead = cancel
	c.Request = c.Request.WithContext(ctx)
	_, err := svc.Forward(ctx, c, account, []byte(`{"model":"gpt-5.6-sol","stream":true,"input":"hello"}`))
	require.Error(t, err)
	require.Equal(t, 1, dialer.requestCount())
	require.Empty(t, svc.httpUpstream.(*httpUpstreamRecorder).requests)
}

func TestForwardWSTransportFallbackDoesNotDisableNextAccountWS(t *testing.T) {
	svc, account, c, _, dialer := newWSMappedRetryFixture(t, OAuthRetrySettings{Enabled: true, MaxRetries: 1, StatusCodes: []int{502}}, nil, wsRetrySuccessfulEvents())
	svc.httpUpstream.(*httpUpstreamRecorder).resp = wsTransportHTTPResponse(502)
	result, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.6-sol","stream":true,"input":"hello"}`))
	require.Error(t, err)
	require.Nil(t, result)
	require.False(t, c.Writer.Written())
	nextAccount := *account
	nextAccount.ID++
	result, err = svc.Forward(context.Background(), c, &nextAccount, []byte(`{"model":"gpt-5.6-sol","stream":true,"input":"hello"}`))
	require.NoError(t, err)
	require.True(t, result.OpenAIWSMode)
	require.Equal(t, 2, dialer.requestCount())
	require.Len(t, svc.httpUpstream.(*httpUpstreamRecorder).requests, 1)
}

func TestForwardWSTransportAtExhaustionKeepsOneHTTPFallback(t *testing.T) {
	svc, account, c, _, dialer := newWSMappedRetryFixture(t, OAuthRetrySettings{Enabled: true, MaxRetries: 1, StatusCodes: []int{502}}, [][]byte{[]byte(wsRetryOverloadEvent)}, nil)
	svc.httpUpstream.(*httpUpstreamRecorder).resp = wsTransportHTTPStreamFailure()
	_, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.6-sol","stream":true,"input":"hello"}`))
	require.Error(t, err)
	require.Equal(t, 2, dialer.requestCount())
	require.Len(t, svc.httpUpstream.(*httpUpstreamRecorder).requests, 1)
}
