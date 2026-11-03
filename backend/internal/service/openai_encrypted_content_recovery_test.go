package service

import (
	"context"
	"encoding/json"
	"errors"
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

const encryptedMessageFixture = `{"model":"gpt-5.1","stream":false,"input":[{"type":"message","role":"assistant","id":"amsg_old","content":[{"type":"output_text","text":"keep plaintext"},{"type":"encrypted_content","data":"old-cipher"}]},{"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{\"encrypted_content\":\"user-data\"}"},{"type":"function_call_output","call_id":"call_1","output":"tool result"}],"sequence":900719925474099312345}`
const encryptedRejectionFixture = `{"error":{"type":"invalid_request_error","message":"The encrypted content for item rs_092e7fbbc429802a006ae945b3c3cc819583f698e38f8f962a could not be verified. Reason: Encrypted content could not be decrypted or parsed."}}`

func TestEncryptedContentRecoveryPreservesPlaintextAndTools(t *testing.T) {
	for _, envelope := range []bool{false, true} {
		t.Run(fmt.Sprint(envelope), func(t *testing.T) {
			source := encryptedMessageFixture
			prefix := ""
			if envelope {
				source = `{"type":"response.create","response":` + source + `}`
				prefix = "response."
			}
			original := []byte(source)
			cleaned, changed, err := SanitizeOpenAIEncryptedContent(original)
			require.NoError(t, err)
			require.True(t, changed)
			require.Equal(t, source, string(original))
			require.False(t, OpenAIRequestHasEncryptedContent(cleaned))
			require.Equal(t, "keep plaintext", gjson.GetBytes(cleaned, prefix+"input.0.content.0.text").String())
			require.False(t, gjson.GetBytes(cleaned, prefix+"input.0.id").Exists())
			require.Equal(t, `{"encrypted_content":"user-data"}`, gjson.GetBytes(cleaned, prefix+"input.1.arguments").String())
			require.Equal(t, "call_1", gjson.GetBytes(cleaned, prefix+"input.2.call_id").String())
			require.Equal(t, "900719925474099312345", gjson.GetBytes(cleaned, prefix+"sequence").Raw)
			twice, changed, err := SanitizeOpenAIEncryptedContent(cleaned)
			require.NoError(t, err)
			require.False(t, changed)
			require.Equal(t, cleaned, twice)
		})
	}
	protected := []byte(`{"input":[{"type":"message","role":"user","content":[{"type":"encrypted_content","data":"user-json"}]},{"type":"function_call_output","call_id":"call","output":{"type":"encrypted_content","data":"tool-json"}}]}`)
	cleaned, changed, err := SanitizeOpenAIEncryptedContent(protected)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, protected, cleaned)
}

func TestEncryptedMessageLineageIsSelective(t *testing.T) {
	old := `{"type":"encrypted_content","data":"old-cipher"}`
	invalid := map[string]struct{}{openAIEncryptedContentDigest(openAIEncryptedFragmentKey(old)): {}}
	payload := []byte(`{"type":"response.create","response":{"input":[{"type":"message","role":"assistant","id":"amsg_old","content":[{"data":"old-cipher","type":"encrypted_content"},{"type":"encrypted_content","data":"fresh-cipher"},{"type":"output_text","text":"preserved"}]}]}}`)
	cleaned, count, err := stripOpenAIInvalidEncryptedContentRaw(payload, invalid)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.Equal(t, "fresh-cipher", gjson.GetBytes(cleaned, "response.input.0.content.0.data").String())
	require.Equal(t, "preserved", gjson.GetBytes(cleaned, "response.input.0.content.1.text").String())
	require.NotContains(t, string(cleaned), "old-cipher")
	item := json.RawMessage(gjson.GetBytes(payload, "response.input.0").Raw)
	replay, count := stripOpenAIInvalidEncryptedContentFromReplayItems([]json.RawMessage{item}, invalid)
	require.Equal(t, 1, count)
	require.Len(t, replay, 1)
	require.Contains(t, string(item), "old-cipher")
	require.NotContains(t, string(replay[0]), "old-cipher")
	require.Len(t, collectOpenAIEncryptedContentDigestsRaw(payload), 2)
}

func TestEncryptedContentAffinitySnapshotsOwner(t *testing.T) {
	store := NewOpenAIWSStateStore(nil)
	svc := &OpenAIGatewayService{openaiWSStateStore: store}
	group := int64(7)
	require.NoError(t, store.BindResponseAccount(context.Background(), group, "resp_owner", 10, time.Hour))
	body := []byte(encryptedMessageFixture)
	ctx := svc.WithOpenAIEncryptedContentAffinity(context.Background(), &group, "", "resp_owner", body)
	require.True(t, openAIEncryptedContentStickyProtected(ctx))
	require.NoError(t, store.BindResponseAccount(context.Background(), group, "resp_owner", 20, time.Hour))
	for _, id := range []int64{10, 20, 20, 10} {
		got, err := SanitizeOpenAIEncryptedContentForAccount(ctx, &Account{ID: id, Platform: PlatformOpenAI}, body)
		require.NoError(t, err)
		require.Equal(t, id == 10, OpenAIRequestHasEncryptedContent(got))
	}
	unknown := svc.WithOpenAIEncryptedContentAffinity(context.Background(), nil, "", "", body)
	first, err := SanitizeOpenAIEncryptedContentForAccount(unknown, &Account{ID: 30, Platform: PlatformOpenAI}, body)
	require.NoError(t, err)
	require.Equal(t, body, first)
	next, err := SanitizeOpenAIEncryptedContentForAccount(unknown, &Account{ID: 40, Platform: PlatformOpenAI}, body)
	require.NoError(t, err)
	require.False(t, OpenAIRequestHasEncryptedContent(next))
	shadow := svc.WithOpenAIEncryptedContentAffinity(context.Background(), nil, "", "", body)
	parent := int64(50)
	for _, account := range []*Account{{ID: 51, ParentAccountID: &parent, Platform: PlatformOpenAI}, {ID: 50, Platform: PlatformOpenAI}, {ID: 52, ParentAccountID: &parent, Platform: PlatformOpenAI}} {
		got, err := SanitizeOpenAIEncryptedContentForAccount(shadow, account, body)
		require.NoError(t, err)
		require.Equal(t, body, got)
	}
}

func TestEncryptedContentRecoveryAcrossHTTPPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, path := range []string{"standard", "passthrough", "bridge"} {
		for _, terminal := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/terminal=%v", path, terminal), func(t *testing.T) {
				response := func(status int, body string) *http.Response {
					return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
				}
				second := response(200, `{"id":"resp_recovered","usage":{"input_tokens":1,"output_tokens":2}}`)
				if path == "bridge" {
					second = response(200, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_recovered\",\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":2}}}\n\n")
					second.Header.Set("Content-Type", "text/event-stream")
				}
				if terminal {
					second = response(400, encryptedRejectionFixture)
				}
				upstream := &httpUpstreamSequenceRecorder{responses: []*http.Response{response(400, encryptedRejectionFixture), second}}
				cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}
				svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
				account := &Account{ID: 2, Status: StatusActive, Schedulable: true, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{"api_key": "test-only"}, Extra: map[string]any{"openai_passthrough": path == "passthrough"}}
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
				SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
				body := []byte(encryptedMessageFixture)
				var err error
				if path == "bridge" {
					_, err = svc.proxyOpenAIWSHTTPBridgeTurn(context.Background(), c, account, "test-only", body, len(body), "gpt-5.1", "", "", "", "", 1, func([]byte) error { return nil })
				} else {
					_, err = svc.Forward(context.Background(), c, account, body)
				}
				if terminal {
					require.ErrorIs(t, err, errOpenAIInvalidEncryptedContent)
				} else {
					require.NoError(t, err)
				}
				require.Equal(t, 2, upstream.callCount)
				require.True(t, OpenAIRequestHasEncryptedContent(upstream.bodies[0]))
				require.False(t, OpenAIRequestHasEncryptedContent(upstream.bodies[1]))
				require.Contains(t, string(upstream.bodies[1]), "keep plaintext")
				require.Contains(t, string(upstream.bodies[1]), "call_1")
			})
		}
	}
}

func TestEncryptedContentErrorDoesNotPenalizeAccount(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)
	openAIAdvancedSchedulerSettingCache.Store(&cachedOpenAIAdvancedSchedulerSetting{enabled: true, expiresAt: time.Now().Add(time.Hour).UnixNano()})
	svc := &OpenAIGatewayService{}
	account := &Account{ID: 10, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	scheduler := svc.getOpenAIAccountScheduler(context.Background()).(*defaultOpenAIAccountScheduler)
	require.False(t, svc.shouldFailoverOpenAIUpstreamResponse(account, 400, "", []byte(encryptedRejectionFixture)))
	account.Credentials = map[string]any{"pool_mode": true, "pool_mode_retry_status_codes": []any{400}}
	require.True(t, account.IsPoolMode())
	require.True(t, account.IsPoolModeRetryableStatus(400))
	require.False(t, shouldFailoverOpenAIPassthroughResponse(account, 400, []byte(encryptedRejectionFixture)))
	for _, err := range []error{errOpenAIInvalidEncryptedContent, fmt.Errorf("wrapped: %w", errOpenAIInvalidEncryptedContent), &openAIWSFallbackError{Reason: openAIWSFallbackReasonInvalidEncryptedContent}} {
		svc.ReportOpenAIAccountScheduleResult(account, "gpt-5.1", false, nil, err)
	}
	rate, _, _ := scheduler.stats.snapshot(account.ID)
	require.Zero(t, rate)
	svc.ReportOpenAIAccountScheduleResult(account, "gpt-5.1", false, nil, errors.New("connection failed"))
	rate, _, _ = scheduler.stats.snapshot(account.ID)
	require.Greater(t, rate, 0.0)
	require.True(t, isOpenAIInvalidEncryptedContentMessage("encrypted content hydration failed"))
	require.False(t, isOpenAIInvalidEncryptedContentResponse(500, []byte(encryptedRejectionFixture)))
}

func TestEncryptedContentStickyWaitOverridesWeightedEscape(t *testing.T) {
	group := int64(9)
	owner := Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, GroupIDs: []int64{group}}
	other := owner
	other.ID = 2
	cfg := &config.Config{}
	cfg.Gateway.OpenAIScheduler.StickyEscapeEnabled = true
	cfg.Gateway.Scheduling.StickySessionWaitTimeout = time.Second
	cfg.Gateway.Scheduling.StickySessionMaxWaiting = 3
	svc := &OpenAIGatewayService{cfg: cfg, cache: &schedulerTestGatewayCache{}, accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{owner, other}}, concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{acquireResults: map[int64]bool{1: false, 2: true}})}
	stats := newOpenAIAccountRuntimeStats()
	for range 20 {
		stats.report(1, false, nil)
	}
	scheduler := &defaultOpenAIAccountScheduler{service: svc, stats: stats}
	ctx := context.WithValue(context.Background(), openAIEncryptedAffinityKey{}, &openAIEncryptedAffinity{})
	selection, decision, err := scheduler.Select(ctx, OpenAIAccountScheduleRequest{GroupID: &group, Platform: PlatformOpenAI, SessionHash: "encrypted-session", StickyAccountID: 1, StickyWeighted: true})
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, int64(1), selection.Account.ID)
	require.True(t, decision.StickySessionHit)
	require.False(t, selection.Acquired)
	require.NotNil(t, selection.WaitPlan)
	require.Equal(t, int64(1), selection.WaitPlan.AccountID)
}

func TestEncryptedContentBridgeTerminalCarriesSchedulingError(t *testing.T) {
	event := `{"type":"response.failed","response":{"id":"resp_failed","error":{"code":"invalid_encrypted_content","message":"encrypted content hydration failed"}}}`
	upstream := &httpUpstreamSequenceRecorder{responses: []*http.Response{{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: " + event + "\n\n"))}}}
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}, httpUpstream: upstream}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/v1/responses", nil)
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true}
	body := []byte(encryptedMessageFixture)
	result, err := svc.proxyOpenAIWSHTTPBridgeTurn(context.Background(), c, account, "test-only", body, len(body), "gpt-5.1", "", "", "", "", 1, func([]byte) error { return nil })
	require.NoError(t, err)
	require.NotNil(t, result)
	require.ErrorIs(t, result.SchedulingError(), errOpenAIInvalidEncryptedContent)
	require.False(t, result.SucceededForScheduling())
}

func TestEncryptedContentNativeWSRetriesSelfContainedTurnOnce(t *testing.T) {
	for _, terminal := range []bool{false, true} {
		t.Run(fmt.Sprint(terminal), func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Gateway.OpenAIWS.Enabled = true
			cfg.Gateway.OpenAIWS.APIKeyEnabled = true
			cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
			cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
			cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
			rejection := []byte(`{"type":"error","error":{"code":"invalid_encrypted_content","message":"encrypted content hydration failed"}}`)
			first := &openAIWSCaptureConn{events: [][]byte{rejection}}
			second := &openAIWSCaptureConn{events: [][]byte{[]byte(`{"type":"response.completed","response":{"id":"resp_recovered","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`)}}
			if terminal {
				second.events = [][]byte{rejection, []byte(`{"type":"response.failed","response":{"id":"resp_failed","error":{"code":"invalid_encrypted_content","message":"encrypted content hydration failed"}}}`)}
			}
			dialer := &openAIWSQueueDialer{conns: []openAIWSClientConn{first, second}}
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
				payload := []byte(strings.Replace(encryptedMessageFixture, `{"model":`, `{"type":"response.create","model":`, 1))
				done <- svc.ProxyResponsesWebSocketFromClient(ctx, c, conn, account, "test-only", payload, &OpenAIWSIngressHooks{AfterTurn: func(_ int, result *OpenAIForwardResult, _ error) { results <- result }})
			}))
			defer server.Close()
			conn, _, err := coderws.Dial(ctx, server.URL, nil)
			require.NoError(t, err)
			defer conn.CloseNow()
			_, event, err := conn.Read(ctx)
			require.NoError(t, err)
			if terminal {
				require.Equal(t, "error", gjson.GetBytes(event, "type").String())
				_, event, err = conn.Read(ctx)
				require.NoError(t, err)
				require.Equal(t, "response.failed", gjson.GetBytes(event, "type").String())
			} else {
				require.Equal(t, "response.completed", gjson.GetBytes(event, "type").String())
			}
			var result *OpenAIForwardResult
			select {
			case result = <-results:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			require.NotNil(t, result)
			if terminal {
				require.ErrorIs(t, result.SchedulingError(), errOpenAIInvalidEncryptedContent)
			} else {
				require.NoError(t, result.SchedulingError())
			}
			require.NoError(t, conn.Close(coderws.StatusNormalClosure, "done"))
			select {
			case err := <-done:
				require.NoError(t, err)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			require.Equal(t, 2, dialer.DialCount())
			first.mu.Lock()
			firstBody, _ := json.Marshal(first.lastWrite)
			first.mu.Unlock()
			second.mu.Lock()
			secondBody, _ := json.Marshal(second.lastWrite)
			writeCount := len(second.writes)
			second.mu.Unlock()
			require.True(t, OpenAIRequestHasEncryptedContent(firstBody))
			require.False(t, OpenAIRequestHasEncryptedContent(secondBody))
			require.Equal(t, 1, writeCount)
		})
	}
}
