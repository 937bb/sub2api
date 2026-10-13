package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
	openaiwsv2 "github.com/Wei-Shaw/sub2api/internal/service/openai_ws_v2"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const processingErrorJSON = `{"type":"response.failed","response":{"id":"resp_failed","output":[],"status":"failed","error":{"code":"server_error","message":"An error occurred while processing your request. You can retry your request, or contact us through our help center at help.openai.com if the error persists. Please include the request ID 23915b85-faad-4cfc-bb4b-3fa8b7e7d462 in your message."}}}`
const processingCreatedJSON = `{"type":"response.created","response":{"id":"resp_failed","output":[]}}`
const processingCompletedJSON = `{"type":"response.completed","response":{"id":"resp_success","status":"completed","model":"gpt-6-astra","output":[],"usage":{"input_tokens":10,"output_tokens":2}}}`

func processingSSE(events ...string) string {
	var body strings.Builder
	for _, event := range events {
		fmt.Fprintf(&body, "data: %s\n\n", event)
	}
	return body.String()
}

func TestProcessingRetryHTTPPreservesRequestAndClosesFailedBody(t *testing.T) {
	for _, kind := range []string{"json400", "json500", "persistent", "after_text", "after_tool", "echo", "non_replayable"} {
		t.Run(kind, func(t *testing.T) {
			status, contentType, body := 200, "text/event-stream", processingSSE(processingCreatedJSON, processingErrorJSON)
			wantRetry := true
			switch kind {
			case "persistent":
				status, contentType, body = 500, "application/json", processingErrorJSON
			case "json400":
				status, contentType, body = 400, "application/json", processingErrorJSON
			case "json500":
				status, contentType, body = 500, "application/json", processingErrorJSON
			case "after_text":
				body, wantRetry = processingSSE(`{"type":"response.output_text.delta","delta":"hello"}`, processingErrorJSON), false
			case "after_tool":
				body, wantRetry = processingSSE(`{"type":"response.output_item.added","item":{"type":"function_call","name":"charge"}}`, processingErrorJSON), false
			case "echo":
				body, wantRetry = processingSSE(`{"type":"response.output_text.delta","delta":"An error occurred while processing your request"}`), false
			case "non_replayable":
				wantRetry = false
			}
			firstBody := &passthroughCloseTrackingReadCloser{Reader: strings.NewReader(body)}
			secondBody := processingSSE(processingCompletedJSON)
			if kind == "persistent" {
				secondBody = processingSSE(processingErrorJSON)
			}
			upstream := &httpUpstreamRecorder{responses: []*http.Response{
				{StatusCode: status, Header: http.Header{"Content-Type": {contentType}}, Body: firstBody},
				{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(secondBody))},
			}}
			svc := newOpenAIRejectedFieldTestService(upstream)
			payload := `{"model":"gpt-6-astra","input":"hi","prompt_cache_key":"stable","tools":[]}`
			req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://example.com/v1/responses", strings.NewReader(payload))
			require.NoError(t, err)
			req.Header.Set("Authorization", "Bearer test-only")
			if kind == "non_replayable" {
				req.GetBody = nil
			}
			resp, err := svc.doOpenAIUpstream(req, "", newOpenAIRejectedFieldTestAccount())
			require.NoError(t, err)
			got, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			if wantRetry {
				require.Len(t, upstream.requests, 2)
				require.True(t, firstBody.closed)
				require.True(t, httpclient.RequiresFreshConnection(upstream.requests[1].Context()))
				require.Equal(t, upstream.bodies[0], upstream.bodies[1])
				require.Equal(t, "Bearer test-only", upstream.requests[1].Header.Get("Authorization"))
				require.Equal(t, secondBody, string(got))
			} else {
				require.Len(t, upstream.requests, 1)
				require.Equal(t, body, string(got))
			}
		})
	}
}

func TestForwardProcessingRetrySSE(t *testing.T) {
	for _, mode := range []string{"stream", "nonstream", "passthrough", "persistent", "mixed", "after_output"} {
		t.Run(mode, func(t *testing.T) {
			first := processingSSE(processingCreatedJSON, processingErrorJSON)
			if mode == "after_output" {
				first = processingSSE(`{"type":"response.output_text.delta","delta":"hello"}`, processingErrorJSON)
			}
			second := processingSSE(processingCompletedJSON)
			if mode == "persistent" || mode == "mixed" {
				second = processingSSE(processingErrorJSON)
			}
			initial := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(first))}
			if mode == "mixed" {
				initial.StatusCode = 400
				initial.Header.Set("Content-Type", "application/json")
				initial.Body = io.NopCloser(strings.NewReader(processingErrorJSON))
			}
			upstream := &httpUpstreamRecorder{responses: []*http.Response{initial, {StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(second))}}}
			svc := newOpenAIRejectedFieldTestService(upstream)
			account := newOpenAIRejectedFieldTestAccount()
			if mode == "passthrough" {
				account.Extra["openai_passthrough"] = true
			}
			payload := []byte(fmt.Sprintf(`{"model":"gpt-6-astra","stream":%t,"input":"hi"}`, mode != "nonstream"))
			c := newOpenAIRejectedFieldTestContext(payload)
			result, err := svc.Forward(context.Background(), c, account, payload)
			if mode == "stream" || mode == "nonstream" || mode == "passthrough" {
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, 10, result.Usage.InputTokens)
			} else {
				require.Error(t, err)
			}
			if mode == "after_output" {
				require.Len(t, upstream.requests, 1)
			} else {
				require.Len(t, upstream.requests, 2)
				require.True(t, httpclient.RequiresFreshConnection(upstream.requests[1].Context()))
				require.Equal(t, upstream.bodies[0], upstream.bodies[1])
			}
		})
	}
}

func TestProcessingPeekBoundAndErrorFieldSafety(t *testing.T) {
	for _, body := range []string{
		`{"error":{"message":"bad request"},"input":"An error occurred while processing your request"}`,
		`{"response":{"output":[{"type":"function_call"}],"error":{"message":"An error occurred while processing your request"}}}`,
		strings.Repeat(": keepalive\n\n", openAIProcessingPeekLimit),
	} {
		resp := &http.Response{StatusCode: 400, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body))}
		require.False(t, peekOpenAIProcessingFailure(resp))
		got, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, body, string(got))
		require.NoError(t, resp.Body.Close())
	}
	require.False(t, isOpenAIProcessingFailure([]byte(`{"error":{"message":"bad request"},"input":"An error occurred while processing your request"}`)))
}

func TestProcessingRetryWSV2ReplacesConnection(t *testing.T) {
	for _, mode := range []string{"success", "persistent", "after_output", "bare_error", "retry_eof"} {
		t.Run(mode, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			cfg := newOpenAIWSV2TestConfig()
			cfg.Security.URLAllowlist.Enabled = false
			cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
			failure := processingErrorJSON
			if mode == "bare_error" {
				failure = `{"type":"error","error":{"code":"server_error","message":"An error occurred while processing your request."}}`
			}
			first := &openAIWSCaptureConn{events: [][]byte{[]byte(processingCreatedJSON), []byte(failure)}}
			if mode == "after_output" {
				first.events = append([][]byte{[]byte(`{"type":"response.output_text.delta","delta":"hello"}`)}, first.events...)
			}
			secondEvent := processingCompletedJSON
			if mode == "persistent" {
				secondEvent = processingErrorJSON
			}
			second := &openAIWSCaptureConn{events: [][]byte{[]byte(secondEvent)}}
			if mode == "retry_eof" {
				second.events = nil
			}
			dialer := &openAIWSQueueDialer{conns: []openAIWSClientConn{first, second}}
			pool := newOpenAIWSConnPool(cfg)
			defer pool.Close()
			pool.setClientDialerForTest(dialer)
			svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: &httpUpstreamRecorder{}, cache: &stubGatewayCache{}, openaiWSResolver: NewOpenAIWSProtocolResolver(cfg), toolCorrector: NewCodexToolCorrector(), openaiWSPool: pool}
			account := &Account{ID: 5884, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, Credentials: map[string]any{"api_key": "sk-test"}, Extra: map[string]any{"responses_websockets_v2_enabled": true}}
			_, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-6-astra","stream":true,"input":"hi"}`))
			if mode == "retry_eof" {
				require.Error(t, err)
			}
			if mode == "success" || mode == "bare_error" {
				require.NoError(t, err)
				require.Contains(t, rec.Body.String(), "resp_success")
				require.NotContains(t, rec.Body.String(), "resp_failed")
			}
			if mode == "after_output" {
				require.Equal(t, 1, dialer.DialCount())
			} else {
				require.Equal(t, 2, dialer.DialCount())
			}
		})
	}
}

func TestProcessingRetryWSClientModes(t *testing.T) {
	for _, mode := range []string{"ctx_pool", "passthrough"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cfg := passthroughLifecycleConfig()
			cfg.Gateway.OpenAIFirstOutputTimeoutSeconds = 5
			cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 5
			account := passthroughLifecycleAccount()
			account.Extra["openai_apikey_responses_websockets_v2_mode"] = mode
			first, second := newStagedPassthroughConn(), newStagedPassthroughConn()
			first.Send(processingErrorJSON)
			second.Send(processingCompletedJSON)
			dialer := &openAIWSQueueDialer{conns: []openAIWSClientConn{first, second}}
			svc := newPassthroughLifecycleService(cfg, first)
			svc.openaiWSPassthroughDialer = dialer
			pool := newOpenAIWSConnPool(cfg)
			defer pool.Close()
			pool.setClientDialerForTest(dialer)
			svc.openaiWSPool = pool
			server, done := startPassthroughLifecycleServer(t, ctx, svc, account)
			defer server.Close()
			client, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
			require.NoError(t, err)
			defer func() { _ = client.CloseNow() }()
			require.NoError(t, client.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-6-astra","input":"hi","stream":true}`)))
			_, data, err := client.Read(ctx)
			require.NoError(t, err)
			require.Contains(t, string(data), "resp_success")
			require.NotContains(t, string(data), "resp_failed")
			require.Equal(t, 2, dialer.DialCount())
			select {
			case <-first.closed:
			default:
				t.Fatal("failed upstream connection remained open")
			}
			_ = client.Close(coderws.StatusNormalClosure, "done")
			select {
			case <-done:
			case <-ctx.Done():
				t.Fatal("websocket handler did not exit")
			}
		})
	}
}

func TestProcessingRetryFrameSafetyAndPerTurnBudget(t *testing.T) {
	for _, mode := range []string{"persistent", "after_tool", "continuation", "cancelled", "second_turn"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			first, second := newStagedPassthroughConn(), newStagedPassthroughConn()
			calls := 0
			conn := &openAIProcessingRetryFrameConn{inner: first, reconnect: func(context.Context, []byte) (openaiwsv2.FrameConn, error) { calls++; return second, nil }}
			defer func() { _ = conn.Close() }()
			payload := `{"type":"response.create","model":"gpt-6-astra","input":"hi"}`
			if mode == "continuation" {
				payload = `{"type":"response.create","previous_response_id":"resp_prior","input":[]}`
			}
			require.NoError(t, conn.WriteFrame(ctx, coderws.MessageText, []byte(payload)))
			if mode == "second_turn" {
				first.Send(processingCompletedJSON)
				_, _, err := conn.ReadFrame(ctx)
				require.NoError(t, err)
				require.NoError(t, conn.WriteFrame(ctx, coderws.MessageText, []byte(payload)))
			}
			if mode == "after_tool" {
				first.Send(`{"type":"response.output_item.added","item":{"type":"function_call","name":"charge"}}`)
				_, _, err := conn.ReadFrame(ctx)
				require.NoError(t, err)
			}
			first.Send(processingErrorJSON)
			second.Send(processingErrorJSON)
			if mode == "cancelled" {
				cancel()
			}
			_, data, err := conn.ReadFrame(ctx)
			if mode != "cancelled" {
				require.NoError(t, err)
				require.Equal(t, processingErrorJSON, string(data))
			}
			if mode == "persistent" || mode == "second_turn" {
				require.Equal(t, 1, calls)
				require.Equal(t, payload, string(<-second.writes))
			} else {
				require.Zero(t, calls)
			}
		})
	}
}

func TestProcessingRetryWSHTTPBridgeKeepsOneClientTurn(t *testing.T) {
	for _, turn := range []int{1, 2} {
		t.Run(fmt.Sprint(turn), func(t *testing.T) {
			failedBody := processingSSE(processingCreatedJSON, processingErrorJSON)
			// Later bridge turns intentionally flush lifecycle events immediately.
			// Exercise recovery before those events commit the client turn.
			if turn > 1 {
				failedBody = processingSSE(processingErrorJSON)
			}
			upstream := &httpUpstreamRecorder{responses: []*http.Response{
				{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(failedBody))},
				{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(processingSSE(processingCompletedJSON)))},
			}}
			svc := newOpenAIRejectedFieldTestService(upstream)
			account := newOpenAIRejectedFieldTestAccount()
			payload := []byte(`{"type":"response.create","model":"gpt-6-astra","stream":true,"input":"hi"}`)
			c := newOpenAIRejectedFieldTestContext(payload)
			var frames []string
			result, err := svc.proxyOpenAIWSHTTPBridgeTurn(context.Background(), c, account, "sk-test", payload, len(payload), "gpt-6-astra", "", "", "", "", turn, func(frame []byte) error { frames = append(frames, string(frame)); return nil })
			require.NoError(t, err)
			require.Equal(t, 10, result.Usage.InputTokens)
			require.Len(t, upstream.requests, 2)
			require.True(t, httpclient.RequiresFreshConnection(upstream.requests[1].Context()))
			require.Equal(t, upstream.bodies[0], upstream.bodies[1])
			require.NotContains(t, strings.Join(frames, "\n"), "resp_failed")
			require.Contains(t, strings.Join(frames, "\n"), "resp_success")
		})
	}
}
