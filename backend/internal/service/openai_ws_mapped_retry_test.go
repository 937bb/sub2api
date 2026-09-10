package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const wsRetryOverloadMessage = "Our servers are currently overloaded. Please try again later."
const wsRetryOverloadEvent = `{"type":"error","error":{"code":"server_is_overloaded","type":"service_unavailable_error","message":"Our servers are currently overloaded. Please try again later."}}`
const wsRetryFailedEvent = `{"type":"response.failed","response":{"id":"resp_failed","status":"failed","output":[],"error":{"code":"server_is_overloaded","type":"service_unavailable_error","message":"Our servers are currently overloaded. Please try again later."}}}`

// Each attempt uses a fresh connection, as production evicts errored WS leases.
type wsMappedRetryDialer struct {
	mu       sync.Mutex
	attempts [][][]byte
	conns    []*openAIWSCaptureConn
	onRead   func()
}

type wsMappedRetryConn struct {
	*openAIWSCaptureConn
	onRead func()
}

func (c *wsMappedRetryConn) ReadMessage(ctx context.Context) ([]byte, error) {
	if c.onRead != nil {
		c.onRead()
	}
	return c.openAIWSCaptureConn.ReadMessage(ctx)
}

func (d *wsMappedRetryDialer) Dial(_ context.Context, _ string, _ http.Header, _ string) (openAIWSClientConn, int, http.Header, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	i := len(d.conns)
	if i >= len(d.attempts) {
		i = len(d.attempts) - 1
	}
	conn := &openAIWSCaptureConn{events: append([][]byte(nil), d.attempts[i]...)}
	d.conns = append(d.conns, conn)
	return &wsMappedRetryConn{openAIWSCaptureConn: conn, onRead: d.onRead}, 101, http.Header{"X-Request-Id": []string{fmt.Sprintf("ws-attempt-%d", len(d.conns))}}, nil
}

func (d *wsMappedRetryDialer) requestCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	count := 0
	for _, conn := range d.conns {
		conn.mu.Lock()
		count += len(conn.writes)
		conn.mu.Unlock()
	}
	return count
}

func newWSMappedRetryFixture(t *testing.T, settings OAuthRetrySettings, attempts ...[][]byte) (*OpenAIGatewayService, *Account, *gin.Context, *httptest.ResponseRecorder, *wsMappedRetryDialer) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := newOpenAIWSV2TestConfig()
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	cfg.Gateway.LogUpstreamErrorBody = true
	raw, err := json.Marshal(settings)
	require.NoError(t, err)
	dialer := &wsMappedRetryDialer{attempts: attempts}
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(dialer)
	t.Cleanup(pool.Close)
	svc := &OpenAIGatewayService{
		cfg: cfg, httpUpstream: &httpUpstreamRecorder{}, cache: &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg), toolCorrector: NewCodexToolCorrector(),
		openaiWSPool:   pool,
		settingService: NewSettingService(&runtimeDefaultsSettingRepoStub{values: map[string]string{oauthRetrySettingKey: string(raw)}}, cfg),
	}
	account := &Account{ID: 9023, Name: "ws-mapped-retry", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"access_token": "test-token"},
		Extra:       map[string]any{"responses_websockets_v2_enabled": true},
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.144.1")
	return svc, account, c, rec, dialer
}

func wsRetrySuccessfulEvents() [][]byte {
	return [][]byte{
		[]byte(`{"type":"response.output_text.delta","delta":"successful answer"}`),
		[]byte(`{"type":"response.completed","response":{"id":"resp_success","model":"gpt-5.6-sol","status":"completed","usage":{"input_tokens":3,"output_tokens":2}}}`),
	}
}

func TestForwardWSMappedRetryRecoversBeforeOutput(t *testing.T) {
	for _, event := range []string{wsRetryOverloadEvent, wsRetryFailedEvent} {
		for _, buffered := range []bool{false, true} {
			t.Run(fmt.Sprintf("failed_terminal_%v_buffered_%v", event == wsRetryFailedEvent, buffered), func(t *testing.T) {
				events := [][]byte{}
				if buffered {
					events = append(events, []byte(`{"type":"response.created","response":{"id":"resp_discarded","output":[]}}`))
				}
				events = append(events, []byte(event))
				svc, account, c, rec, dialer := newWSMappedRetryFixture(t, OAuthRetrySettings{Enabled: true, MaxRetries: 5, StatusCodes: []int{502}}, events, wsRetrySuccessfulEvents())
				result, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.6-sol","stream":true,"input":"hello"}`))
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, 2, dialer.requestCount())
				require.Equal(t, "resp_success", result.RequestID)
				require.EqualValues(t, 2, result.Usage.OutputTokens)
				require.Equal(t, 1, strings.Count(rec.Body.String(), `"delta":"successful answer"`))
				require.Equal(t, 1, strings.Count(rec.Body.String(), `"type":"response.completed"`))
				require.NotContains(t, rec.Body.String(), "server_is_overloaded")
				require.NotContains(t, rec.Body.String(), "resp_discarded")
				require.NotContains(t, rec.Body.String(), "response.failed")
				require.Nil(t, svc.httpUpstream.(*httpUpstreamRecorder).lastReq)
				v, ok := c.Get(OpsUpstreamErrorsKey)
				require.True(t, ok)
				attempts := v.([]*OpsUpstreamErrorEvent)
				require.Len(t, attempts, 1)
				require.Equal(t, "ws_error", attempts[0].Kind)
				require.Equal(t, "ws-attempt-1", attempts[0].UpstreamRequestID)
				require.Equal(t, wsRetryOverloadMessage, attempts[0].Message)
			})
		}
	}
}

func TestForwardWSMappedRetryBudgetExhaustionAllowsAccountSwitch(t *testing.T) {
	for _, budget := range []int{0, 1, 5} {
		t.Run(fmt.Sprint(budget), func(t *testing.T) {
			svc, account, c, rec, dialer := newWSMappedRetryFixture(t, OAuthRetrySettings{Enabled: true, MaxRetries: budget, StatusCodes: []int{502}}, [][]byte{[]byte(wsRetryOverloadEvent)})
			result, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.6-sol","stream":true,"input":"hello"}`))
			require.Nil(t, result)
			var failover *UpstreamFailoverError
			require.ErrorAs(t, err, &failover)
			require.Equal(t, budget+1, dialer.requestCount(), "one budget, no nested reconnect or same-account replay")
			require.False(t, failover.RetryableOnSameAccount)
			require.True(t, failover.ShouldRetryNextAccount())
			require.True(t, failover.RequestScopedTransient)
			require.Equal(t, wsRetryOverloadMessage, failover.ClientMessage)
			require.Equal(t, OpenAIWSMappedRetryReason, failover.Reason)
			require.Empty(t, rec.Body.String())
			require.False(t, c.Writer.Written())
			require.False(t, IsResponseCommitted(c))
			require.Nil(t, svc.httpUpstream.(*httpUpstreamRecorder).lastReq)
		})
	}
}

func TestForwardWSMappedRetryDoesNotReplayVisibleOrBillableOutput(t *testing.T) {
	for _, prefix := range []string{
		`{"type":"response.output_text.delta","delta":"partial answer"}`,
		`{"type":"response.output_item.added","item":{"type":"function_call","call_id":"call_1","name":"tool","arguments":""}}`,
		`{"type":"response.in_progress","response":{"usage":{"input_tokens":5,"output_tokens":0}}}`,
	} {
		for _, stream := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s_stream_%v", prefix, stream), func(t *testing.T) {
				svc, account, c, rec, dialer := newWSMappedRetryFixture(t, OAuthRetrySettings{Enabled: true, MaxRetries: 2, StatusCodes: []int{502}}, [][]byte{[]byte(prefix), []byte(wsRetryOverloadEvent)}, wsRetrySuccessfulEvents())
				_, _ = svc.Forward(context.Background(), c, account, []byte(fmt.Sprintf(`{"model":"gpt-5.6-sol","stream":%v,"input":"hello"}`, stream)))
				require.Equal(t, 1, dialer.requestCount())
				require.NotContains(t, rec.Body.String(), "successful answer")
			})
		}
	}
}

func TestForwardWSMappedRetryRespectsSettingsAndCancellation(t *testing.T) {
	for _, tc := range []struct {
		name     string
		settings OAuthRetrySettings
		cancel   bool
	}{
		{"disabled", OAuthRetrySettings{Enabled: false, MaxRetries: 2, StatusCodes: []int{502}}, false},
		{"unmatched", OAuthRetrySettings{Enabled: true, MaxRetries: 2, StatusCodes: []int{503}}, false},
		{"cancelled", OAuthRetrySettings{Enabled: true, MaxRetries: 2, StatusCodes: []int{502}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, account, c, rec, dialer := newWSMappedRetryFixture(t, tc.settings, [][]byte{[]byte(wsRetryOverloadEvent)}, wsRetrySuccessfulEvents())
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancel {
				dialer.onRead = cancel
			}
			c.Request = c.Request.WithContext(ctx)
			_, _ = svc.Forward(ctx, c, account, []byte(`{"model":"gpt-5.6-sol","stream":true,"input":"hello"}`))
			require.Equal(t, 1, dialer.requestCount())
			require.NotContains(t, rec.Body.String(), "successful answer")
		})
	}
}

func TestWSMappedRetryPreservesSpecialErrorPolicies(t *testing.T) {
	for _, errorJSON := range []string{
		`{"code":"invalid_api_key","type":"authentication_error","message":"invalid credentials"}`,
		`{"code":"permission_denied","type":"permission_error","message":"access denied"}`,
		`{"code":"rate_limit_exceeded","type":"rate_limit_error","message":"Rate limit exceeded"}`,
		`{"code":"server_error","status_code":429,"message":"Please retry later"}`,
		`{"code":"server_error","status_code":529,"message":"Please retry later"}`,
		`{"code":"invalid_request","type":"invalid_request_error","message":"invalid input"}`,
		`{"code":"previous_response_not_found","message":"previous response not found"}`,
		`{"code":"invalid_encrypted_content","message":"invalid encrypted content"}`,
		`{"code":"server_error","message":"Request blocked by content policy"}`,
		`{"code":"server_error","message":"Your input exceeds the context window of this model."}`,
	} {
		t.Run(errorJSON, func(t *testing.T) {
			svc, account, c, _, _ := newWSMappedRetryFixture(t, DefaultOAuthRetrySettings(), wsRetrySuccessfulEvents())
			c.Set(oauthMappedRetryKey, true)
			c.Set("oauth_mapped_retry_settings", OAuthRetrySettings{Enabled: true, MaxRetries: 5, StatusCodes: []int{400, 401, 403, 429, 502, 503, 529}})
			for _, payload := range []string{`{"type":"error","error":` + errorJSON + `}`, `{"type":"response.failed","response":{"error":` + errorJSON + `}}`} {
				require.Nil(t, svc.newOpenAIWSMappedRetryError(context.Background(), c, account, nil, []byte(payload), false))
			}
		})
	}
}

func TestForwardWSMappedRetryPreservesPassthroughStatus(t *testing.T) {
	svc, account, c, rec, dialer := newWSMappedRetryFixture(t, OAuthRetrySettings{Enabled: true, MaxRetries: 5, StatusCodes: []int{502}}, [][]byte{[]byte(wsRetryOverloadEvent)}, wsRetrySuccessfulEvents())
	rule := newNonFailoverPassthroughRule(503, "overloaded", 503, "custom overload")
	rule.Platforms = []string{PlatformOpenAI}
	rules := &ErrorPassthroughService{}
	rules.setLocalCache([]*model.ErrorPassthroughRule{rule})
	BindErrorPassthroughService(c, rules)
	_, _ = svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.6-sol","stream":true,"input":"hello"}`))
	require.Equal(t, 1, dialer.requestCount())
	require.NotContains(t, rec.Body.String(), "successful answer")
}
