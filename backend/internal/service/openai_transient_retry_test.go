package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
	openaiwsv2 "github.com/Wei-Shaw/sub2api/internal/service/openai_ws_v2"
	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

const transientFailureJSON = `{"type":"response.failed","response":{"output":[],"error":{"type":"server_error","code":"internal_error","message":"Upstream service temporarily unavailable"}}}`

func TestTransientRetryHTTPStatusAndExclusions(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     int
		body       string
		retryAfter string
		retry      bool
	}{
		{"502", 502, `{"error":{"type":"server_error","message":"upstream unavailable"}}`, "", true},
		{"503_html", 503, "<html>Service unavailable</html>", "", true},
		{"504", 504, "Gateway timeout", "", true},
		{"429", 429, processingErrorJSON, "", false},
		{"401", 401, processingErrorJSON, "", false},
		{"403", 403, processingErrorJSON, "", false},
		{"long_retry_after", 503, transientFailureJSON, "30", false},
		{"image_capacity", 503, `{"error":{"code":"basispoints_image_request_busy","message":"busy"}}`, "", false},
		{"concurrency", 503, `{"error":{"code":"gateway_concurrency_limit","message":"busy"}}`, "", false},
		{"overload", 503, `{"error":{"type":"server_error","code":"server_is_overloaded","message":"busy"}}`, "", false},
		{"ws_connection_limit", 503, `{"error":{"type":"server_error","code":"websocket_connection_limit_reached","message":"busy"}}`, "", false},
		{"quota", 503, `{"error":{"code":"insufficient_quota","message":"quota exceeded"}}`, "", false},
		{"ineligible", 503, `{"error":{"code":"account_ineligible","message":"admission denied"}}`, "", false},
		{"billable", 503, `{"response":{"usage":{"input_tokens":10},"error":{"type":"server_error"}}}`, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failed := &passthroughCloseTrackingReadCloser{Reader: strings.NewReader(tc.body)}
			upstream := &httpUpstreamRecorder{responses: []*http.Response{
				{StatusCode: tc.status, Header: http.Header{"Content-Type": {"application/json"}, "Retry-After": {tc.retryAfter}}, Body: failed},
				{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("ok"))},
			}}
			svc := newOpenAIRejectedFieldTestService(upstream)
			req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://example.com/v1/responses", strings.NewReader(`{"model":"gpt-6-astra","input":"hi"}`))
			require.NoError(t, err)
			resp, err := svc.doOpenAIUpstream(req, "", newOpenAIOAuthNamespaceTestAccount())
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()
			if tc.retry {
				require.Len(t, upstream.requests, 2)
				require.True(t, failed.closed)
				require.Equal(t, upstream.bodies[0], upstream.bodies[1])
				require.True(t, httpclient.RequiresFreshConnection(upstream.requests[1].Context()))
			} else {
				require.Len(t, upstream.requests, 1)
				got, readErr := io.ReadAll(resp.Body)
				require.NoError(t, readErr)
				require.Equal(t, tc.body, string(got))
			}
		})
	}
}

func TestTransientRetrySSEAndSharedBudget(t *testing.T) {
	for _, mode := range []string{"503_event", "early_eof", "http_then_sse", "after_text", "after_tool"} {
		t.Run(mode, func(t *testing.T) {
			body := processingSSE(processingCreatedJSON, transientFailureJSON)
			if mode == "early_eof" {
				body = processingSSE(processingCreatedJSON)
			}
			if mode == "after_text" {
				body = processingSSE(`{"type":"response.output_text.delta","delta":"hello"}`, transientFailureJSON)
			}
			if mode == "after_tool" {
				body = processingSSE(`{"type":"response.output_item.added","item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"charge","arguments":"{}"}}`, transientFailureJSON)
			}
			first := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body))}
			second := processingSSE(processingCompletedJSON)
			if mode == "http_then_sse" {
				first.StatusCode = 503
				first.Header.Set("Content-Type", "application/json")
				first.Body = io.NopCloser(strings.NewReader(transientFailureJSON))
				second = processingSSE(processingCreatedJSON, transientFailureJSON)
			}
			upstream := &httpUpstreamRecorder{responses: []*http.Response{first, {StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(second))}}}
			svc := newOpenAIRejectedFieldTestService(upstream)
			payload := []byte(`{"model":"gpt-6-astra","stream":true,"input":"hi","prompt_cache_key":"stable"}`)
			c := newOpenAIRejectedFieldTestContext(payload)
			account := newOpenAIOAuthNamespaceTestAccount()
			result, err := svc.Forward(context.Background(), c, account, payload)
			if mode == "503_event" || mode == "early_eof" {
				require.NoError(t, err)
				require.Equal(t, 10, result.Usage.InputTokens)
			} else {
				require.Error(t, err)
			}
			if strings.HasPrefix(mode, "after_") {
				require.Len(t, upstream.requests, 1)
			} else {
				require.Len(t, upstream.requests, 2)
				require.True(t, httpclient.RequiresFreshConnection(upstream.requests[1].Context()))
				require.Equal(t, upstream.bodies[0], upstream.bodies[1])
			}
		})
	}
}

func TestTransientRetryDoesNotDiscardObservedUsage(t *testing.T) {
	svc := &OpenAIGatewayService{}
	calls := 0
	result, err := svc.retryOpenAIProcessing(context.Background(), newOpenAIRejectedFieldTestAccount(), func(context.Context) (*OpenAIForwardResult, error) {
		calls++
		return &OpenAIForwardResult{Usage: OpenAIUsage{InputTokens: 5}}, newOpenAIProcessingFailover(nil, []byte(transientFailureJSON))
	})
	require.Error(t, err)
	require.Equal(t, 1, calls)
	require.Equal(t, 5, result.Usage.InputTokens)
}

type transientReadFailureConn struct{ openaiwsv2.FrameConn }

func (c *transientReadFailureConn) ReadFrame(context.Context) (coderws.MessageType, []byte, error) {
	return coderws.MessageText, nil, io.ErrUnexpectedEOF
}

func TestTransientRetryWSFrameNewConnection(t *testing.T) {
	for _, eof := range []bool{false, true} {
		first, second := newStagedPassthroughConn(), newStagedPassthroughConn()
		var source openaiwsv2.FrameConn = first
		if eof {
			source = &transientReadFailureConn{FrameConn: first}
		} else {
			first.Send(transientFailureJSON)
		}
		second.Send(processingCompletedJSON)
		calls := 0
		conn := &openAIProcessingRetryFrameConn{inner: source, allowTemporary: true, reconnect: func(context.Context, []byte) (openaiwsv2.FrameConn, error) { calls++; return second, nil }}
		payload := []byte(`{"type":"response.create","model":"gpt-6-astra","input":"hi"}`)
		require.NoError(t, conn.WriteFrame(context.Background(), coderws.MessageText, payload))
		_, frame, err := conn.ReadFrame(context.Background())
		require.NoError(t, err)
		require.Equal(t, processingCompletedJSON, string(frame))
		require.Equal(t, 1, calls)
		require.Equal(t, payload, <-second.writes)
		require.NoError(t, conn.Close())
	}
}
