package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const encryptedToolOutputMessage = "Encrypted function output content could not be decrypted or decoded."
const encryptedToolOutputRejection = `{"error":{"code":"invalid_encrypted_content","type":"invalid_request_error","message":"` + encryptedToolOutputMessage + `"}}`
const encryptedToolOutputFixture = `{"model":"gpt-5.1","stream":false,"input":[{"type":"reasoning","encrypted_content":"reasoning-cipher","summary":[]},{"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{}"},{"type":"function_call_output","call_id":"call_1","output":[{"type":"input_text","text":"visible output"},{"type":"encrypted_content","encrypted_content":"opaque-tool-result"}]}]}`

func TestEncryptedAgentContextPreservedAndPinned(t *testing.T) {
	for _, item := range []string{
		`{"type":"agent_message","author":"agent-a","recipient":"agent-b","content":[{"type":"input_text","text":"keep message"},{"type":"encrypted_content","encrypted_content":"opaque-agent-result"}]}`,
		`{"type":"function_call","call_id":"call_sealed","name":"send_message","encrypted_content":"opaque-call-context","arguments":"{}"}`,
		`{"type":"custom_tool_call","call_id":"call_sealed","name":"exec","encrypted_content":"opaque-call-context","input":"command"}`,
	} {
		for _, envelope := range []bool{false, true} {
			body := []byte(`{"input":[{"type":"reasoning","encrypted_content":"keep-reasoning","summary":[]},` + item + `]}`)
			if envelope {
				body = []byte(`{"type":"response.create","response":` + string(body) + `}`)
			}
			require.True(t, openAIRequestHasEncryptedToolOutput(body))
			cleaned, changed, err := SanitizeOpenAIEncryptedContent(body)
			require.NoError(t, err)
			require.False(t, changed)
			require.Equal(t, body, cleaned)
			store := NewOpenAIWSStateStore(nil)
			require.NoError(t, store.BindResponseAccount(context.Background(), 9, "resp_owner", 1, time.Hour))
			svc := &OpenAIGatewayService{openaiWSStateStore: store}
			group := int64(9)
			ctx := svc.WithOpenAIEncryptedContentAffinity(context.Background(), &group, "session", "resp_owner", body)
			require.EqualValues(t, 1, openAIEncryptedToolOutputOwner(ctx))
			bindOpenAIEncryptedToolOutputAttempt(ctx, &Account{ID: 2})
			require.EqualValues(t, 1, openAIEncryptedToolOutputOwner(ctx))
			cleaned, err = SanitizeOpenAIEncryptedContentForAccount(ctx, &Account{ID: 2, Platform: PlatformOpenAI}, body)
			require.NoError(t, err)
			require.Equal(t, body, cleaned)
		}
	}
	for _, body := range []string{
		`{"input":[{"type":"agent_message","content":[{"type":"input_text","text":"encrypted_content"}]}]}`,
		`{"input":[{"type":"agent_message","content":{"type":"encrypted_content","encrypted_content":"tool-owned-json"}}]}`,
		`{"input":[{"type":"function_call","arguments":"{\"encrypted_content\":\"user-json\"}"}]}`,
	} {
		require.False(t, openAIRequestHasEncryptedToolOutput([]byte(body)))
	}
}

func TestEncryptedToolOutputDetectionAndPreservation(t *testing.T) {
	for _, envelope := range []bool{false, true} {
		for _, kind := range []string{"function_call_output", "custom_tool_call_output"} {
			body := strings.ReplaceAll(encryptedToolOutputFixture, "function_call_output", kind)
			if envelope {
				body = `{"type":"response.create","response":` + body + `}`
			}
			require.True(t, openAIRequestHasEncryptedToolOutput([]byte(body)))
			cleaned, changed, err := SanitizeOpenAIEncryptedContent([]byte(body))
			require.NoError(t, err)
			require.False(t, changed)
			require.True(t, openAIRequestHasEncryptedToolOutput(cleaned))
			require.Contains(t, string(cleaned), "opaque-tool-result")
			require.Contains(t, string(cleaned), "visible output")
			require.Contains(t, string(cleaned), "reasoning-cipher")
			require.Equal(t, body, string(cleaned))
		}
	}
	for _, body := range []string{
		`{"input":[{"type":"function_call_output","output":{"type":"encrypted_content","encrypted_content":"tool-owned-json"}}]}`,
		`{"input":[{"type":"function_call_output","output":"encrypted_content"}]}`,
		`{"input":[{"role":"user","content":[{"type":"encrypted_content","encrypted_content":"user-json"}]}]}`,
		`{"input":[{"type":"function_call_output","output":[{"type":"input_text","text":"encrypted_content"}]}]}`,
	} {
		require.False(t, openAIRequestHasEncryptedToolOutput([]byte(body)), body)
	}
}

func TestEncryptedToolOutputRejectionIsNotReasoningRecovery(t *testing.T) {
	require.False(t, isOpenAIInvalidEncryptedContentResponse(400, []byte(encryptedToolOutputRejection)))
	require.False(t, isOpenAIInvalidEncryptedContentMessage(encryptedToolOutputMessage))
	require.ErrorIs(t, openAIEncryptedRequestResponseError(400, []byte(encryptedToolOutputRejection)), errOpenAIInvalidEncryptedToolOutput)
	require.Nil(t, openAIEncryptedRequestResponseError(429, []byte(encryptedToolOutputRejection)))
	require.True(t, isOpenAIEncryptedRequestError(fmt.Errorf("after dispatch: %s", encryptedToolOutputMessage)))
	for _, code := range []string{"", "invalid_encrypted_content"} {
		reason, retry := classifyOpenAIWSErrorEventFromRaw(code, "invalid_request_error", encryptedToolOutputMessage)
		require.Equal(t, "encrypted_tool_output_invalid", reason)
		require.False(t, retry)
	}
	event := []byte(`{"type":"response.failed","response":` + encryptedToolOutputRejection + `}`)
	require.ErrorIs(t, openAIEncryptedContentEventError(event), errOpenAIInvalidEncryptedToolOutput)
	require.Equal(t, 400, openAIStreamFailureStatus(event, encryptedToolOutputMessage))
}

func TestEncryptedToolOutputNoReplayAcrossHTTPPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for fixtureName, fixture := range map[string]string{
		"tool_output":   encryptedToolOutputFixture,
		"agent_message": `{"model":"gpt-5.1","input":[{"type":"reasoning","encrypted_content":"reasoning-cipher","summary":[]},{"type":"agent_message","author":"a","recipient":"b","content":[{"type":"encrypted_content","encrypted_content":"opaque-agent-result"}]}]}`,
		"sealed_call":   `{"model":"gpt-5.1","input":[{"type":"reasoning","encrypted_content":"reasoning-cipher","summary":[]},{"type":"function_call","call_id":"sealed","name":"lookup","arguments":"{}","encrypted_content":"opaque-call"}]}`,
	} {
		for _, generic := range []bool{false, true} {
			rejection := encryptedToolOutputRejection
			wantErr := errOpenAIInvalidEncryptedToolOutput
			message := encryptedToolOutputMessage
			if generic {
				rejection = encryptedRejectionFixture
				wantErr = errOpenAIInvalidEncryptedContent
				message = "could not be verified"
			}
			for _, path := range []string{"standard", "passthrough", "bridge"} {
				t.Run(fmt.Sprintf("%s/%s/generic=%v", fixtureName, path, generic), func(t *testing.T) {
					upstream := &httpUpstreamSequenceRecorder{responses: []*http.Response{{StatusCode: 400, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(rejection))}}}
					svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}, httpUpstream: upstream}
					account := &Account{ID: 2, Status: StatusActive, Schedulable: true, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{"api_key": "test-only", "pool_mode": true, "pool_mode_retry_status_codes": []any{400}}, Extra: map[string]any{"openai_passthrough": path == "passthrough"}}
					recorder := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(recorder)
					c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
					SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
					body := []byte(fixture)
					var err error
					var events [][]byte
					if path == "bridge" {
						_, err = svc.proxyOpenAIWSHTTPBridgeTurn(context.Background(), c, account, "test-only", body, len(body), "gpt-5.1", "", "", "", "", 1, func(b []byte) error { events = append(events, b); return nil })
						require.Len(t, events, 1)
						require.Contains(t, string(events[0]), message)
					} else {
						_, err = svc.Forward(context.Background(), c, account, body)
						require.Equal(t, 400, recorder.Code)
						require.Contains(t, recorder.Body.String(), message)
					}
					require.ErrorIs(t, err, wantErr)
					require.Equal(t, 1, upstream.callCount)
					var before, after []string
					visitOpenAIRequiredEncryptedContext(body, func(v string) { before = append(before, v) })
					visitOpenAIRequiredEncryptedContext(upstream.bodies[0], func(v string) { after = append(after, v) })
					require.NotEmpty(t, before)
					require.Equal(t, before, after)
					require.Contains(t, string(upstream.bodies[0]), "reasoning-cipher")
					require.False(t, svc.shouldFailoverOpenAIUpstreamResponse(account, 400, encryptedToolOutputMessage, []byte(rejection)))
					require.False(t, shouldFailoverOpenAIPassthroughResponse(account, 400, []byte(rejection)))
				})
			}
		}
	}
}

func TestEncryptedToolOutputPinsSnapshotDespiteStickyChanges(t *testing.T) {
	group := int64(9)
	owner := Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, GroupIDs: []int64{group}}
	other := owner
	other.ID = 2
	store := NewOpenAIWSStateStore(nil)
	require.NoError(t, store.BindResponseAccount(context.Background(), group, "resp_owner", owner.ID, time.Hour))
	svc := &OpenAIGatewayService{cfg: &config.Config{}, openaiWSStateStore: store, cache: &schedulerTestGatewayCache{}, accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{owner, other}}, concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{acquireResults: map[int64]bool{1: false, 2: true}})}
	svc.cfg.Gateway.Scheduling.StickySessionWaitTimeout = time.Second
	ctx := svc.WithOpenAIEncryptedContentAffinity(context.Background(), &group, "session", "resp_owner", []byte(encryptedToolOutputFixture))
	require.True(t, openAIEncryptedContentStickyProtected(ctx))
	scheduler := &defaultOpenAIAccountScheduler{service: svc, stats: newOpenAIAccountRuntimeStats()}
	req := OpenAIAccountScheduleRequest{GroupID: &group, Platform: PlatformOpenAI, SessionHash: "session", StickyAccountID: other.ID, StickyWeighted: true}
	selection, _, err := scheduler.Select(ctx, req)
	require.NoError(t, err)
	require.Equal(t, owner.ID, selection.Account.ID)
	require.NotNil(t, selection.WaitPlan)
	// An unavailable owner must not turn required ciphertext into a cross-account retry.
	req.ExcludedIDs = map[int64]struct{}{owner.ID: {}}
	selection, _, err = scheduler.Select(ctx, req)
	require.ErrorIs(t, err, ErrNoAvailableAccounts)
	require.Nil(t, selection)
}

func TestEncryptedToolOutputDoesNotPenalizeAccount(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)
	openAIAdvancedSchedulerSettingCache.Store(&cachedOpenAIAdvancedSchedulerSetting{enabled: true, expiresAt: time.Now().Add(time.Hour).UnixNano()})
	svc := &OpenAIGatewayService{}
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	scheduler := svc.getOpenAIAccountScheduler(context.Background()).(*defaultOpenAIAccountScheduler)
	for _, err := range []error{errOpenAIInvalidEncryptedToolOutput, fmt.Errorf("wrapped: %w", errOpenAIInvalidEncryptedToolOutput), fmt.Errorf("stream failed: %s", encryptedToolOutputMessage)} {
		require.False(t, svc.ReportOpenAIAccountScheduleResult(account, "gpt-5.1", false, nil, err))
	}
	rate, _, _ := scheduler.stats.snapshot(account.ID)
	require.Zero(t, rate)
}

func TestEncryptedToolOutputNativeWSNeverReplays(t *testing.T) {
	for _, generic := range []bool{false, true} {
		t.Run(fmt.Sprint(generic), func(t *testing.T) {
			message := encryptedToolOutputMessage
			wantErr := errOpenAIInvalidEncryptedToolOutput
			if generic {
				message = "encrypted content hydration failed"
				wantErr = errOpenAIInvalidEncryptedContent
			}
			cfg := &config.Config{}
			cfg.Gateway.OpenAIWS.Enabled = true
			cfg.Gateway.OpenAIWS.APIKeyEnabled = true
			cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
			cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
			cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
			rejection := []byte(`{"type":"error","error":{"code":"invalid_encrypted_content","message":"` + message + `"}}`)
			terminal := []byte(`{"type":"response.failed","response":{"id":"resp_failed","error":{"message":"` + message + `"}}}`)
			first := &openAIWSCaptureConn{events: [][]byte{rejection, terminal}}
			unused := &openAIWSCaptureConn{}
			dialer := &openAIWSQueueDialer{conns: []openAIWSClientConn{first, unused}}
			pool := newOpenAIWSConnPool(cfg)
			pool.setClientDialerForTest(dialer)
			defer pool.Close()
			svc := &OpenAIGatewayService{cfg: cfg, cache: &stubGatewayCache{}, openaiWSResolver: NewOpenAIWSProtocolResolver(cfg), toolCorrector: NewCodexToolCorrector(), openaiWSPool: pool}
			account := &Account{ID: 12, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, Credentials: map[string]any{"api_key": "test-only"}, Extra: map[string]any{"responses_websockets_v2_enabled": true}}
			results := make(chan *OpenAIForwardResult, 1)
			done := make(chan error, 1)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := coderws.Accept(w, r, nil)
				if err != nil {
					done <- err
					return
				}
				defer conn.CloseNow()
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = r
				payload := []byte(strings.Replace(encryptedToolOutputFixture, `{"model":`, `{"type":"response.create","model":`, 1))
				done <- svc.ProxyResponsesWebSocketFromClient(ctx, c, conn, account, "test-only", payload, &OpenAIWSIngressHooks{AfterTurn: func(_ int, result *OpenAIForwardResult, _ error) { results <- result }})
			}))
			defer server.Close()
			conn, _, err := coderws.Dial(ctx, server.URL, nil)
			require.NoError(t, err)
			defer conn.CloseNow()
			for _, expected := range []string{"error", "response.failed"} {
				_, event, readErr := conn.Read(ctx)
				require.NoError(t, readErr)
				require.Equal(t, expected, gjson.GetBytes(event, "type").String())
				require.Contains(t, string(event), message)
			}
			select {
			case result := <-results:
				require.NotNil(t, result)
				require.ErrorIs(t, result.SchedulingError(), wantErr)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			require.NoError(t, conn.Close(coderws.StatusNormalClosure, "done"))
			select {
			case err := <-done:
				require.NoError(t, err)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			require.Equal(t, 1, dialer.DialCount())
			first.mu.Lock()
			firstBody, _ := json.Marshal(first.lastWrite)
			writeCount := len(first.writes)
			first.mu.Unlock()
			require.True(t, openAIRequestHasEncryptedToolOutput(firstBody))
			require.Contains(t, string(firstBody), "reasoning-cipher")
			require.Equal(t, 1, writeCount)
			unused.mu.Lock()
			unusedWrites := len(unused.writes)
			unused.mu.Unlock()
			require.Zero(t, unusedWrites)
		})
	}
}
