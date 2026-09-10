package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCodexWSCapacityBeforeOutputEntersBoundedGatewayFailover(t *testing.T) {
	events := map[string]string{
		"error":  `{"type":"error","error":{"code":"server_is_overloaded","type":"service_unavailable_error","message":"Our servers are currently overloaded. Please try again later."}}`,
		"failed": `{"type":"response.failed","response":{"id":"resp_failed","error":{"code":"server_is_overloaded","message":"Our servers are currently overloaded. Please try again later."}}}`,
	}
	for name, event := range events {
		for _, accountType := range []string{AccountTypeOAuth, AccountTypeSetupToken} {
			for _, streaming := range []bool{false, true} {
				t.Run(name+"/"+accountType+"/"+map[bool]string{false: "json", true: "sse"}[streaming], func(t *testing.T) {
					svc, account, conn, c, recorder, repo := codexWSCapacityFixture(t, accountType, [][]byte{
						[]byte(`{"type":"response.created","response":{"id":"resp_failed"}}`),
						[]byte(event),
					})
					body := []byte(`{"model":"gpt-5.6-sol","stream":` + map[bool]string{false: "false", true: "true"}[streaming] + `,"input":[{"role":"user","content":"hi"}]}`)
					result, err := svc.Forward(context.Background(), c, account, body)
					var failover *UpstreamFailoverError
					require.ErrorAs(t, err, &failover)
					require.Nil(t, result)
					require.Equal(t, http.StatusServiceUnavailable, failover.ClientStatusCode)
					require.True(t, failover.RequestScopedTransient)
					require.True(t, failover.ShouldRetryNextAccount())
					require.False(t, c.Writer.Written(), "the handler must retain the response for recovery")
					require.Empty(t, recorder.Body.String())
					require.True(t, conn.closed, "a trailing failure frame must not leak into another request")
					require.Len(t, conn.writes, 1, "recovery belongs to the bounded outer handler, not a nested WS retry loop")
					require.Zero(t, repo.tempUnschedCalls, "capacity must not cool down the whole account")
					require.True(t, account.RateLimitedAt == nil && account.OverloadUntil == nil)
				})
			}
		}
	}
}

func TestCodexWSCapacityAfterOutputCannotReplay(t *testing.T) {
	for name, output := range map[string]string{
		"text": `{"type":"response.output_text.delta","delta":"already delivered"}`,
		"tool": `{"type":"response.function_call_arguments.delta","item_id":"fc_one","delta":"already delivered"}`,
	} {
		t.Run(name, func(t *testing.T) {
			svc, account, conn, c, recorder, _ := codexWSCapacityFixture(t, AccountTypeOAuth, [][]byte{
				[]byte(`{"type":"response.created","response":{"id":"resp_partial"}}`),
				[]byte(output),
				[]byte(`{"type":"error","error":{"code":"server_is_overloaded","message":"Our servers are currently overloaded. Please try again later."}}`),
			})
			_, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.6-sol","stream":true,"input":[{"role":"user","content":"hi"}]}`))
			var failover *UpstreamFailoverError
			require.Error(t, err)
			require.False(t, errors.As(err, &failover))
			require.Contains(t, recorder.Body.String(), "already delivered")
			require.Len(t, conn.writes, 1)
			require.True(t, conn.closed)
		})
	}
}

func codexWSCapacityFixture(t *testing.T, accountType string, events [][]byte) (*OpenAIGatewayService, *Account, *openAIWSCaptureConn, *gin.Context, *httptest.ResponseRecorder, *capacityShedAccountRepoStub) {
	t.Helper()
	cfg := newOpenAIWSV2TestConfig()
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	conn := &openAIWSCaptureConn{events: events}
	pool := newOpenAIWSConnPool(cfg)
	t.Cleanup(pool.Close)
	pool.setClientDialerForTest(&openAIWSCaptureDialer{conn: conn})
	repo := &capacityShedAccountRepoStub{}
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: &httpUpstreamRecorder{}, cache: &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg), toolCorrector: NewCodexToolCorrector(), openaiWSPool: pool}
	account := &Account{ID: 81, Platform: PlatformOpenAI, Type: accountType, Concurrency: 1,
		Credentials: map[string]any{"access_token": "test-token", "chatgpt_account_id": "workspace-81"},
		Extra: map[string]any{"responses_websockets_v2_enabled": true,
			openAICodexInstallationIDExtraKey: "550e8400-e29b-41d4-a716-446655440000"}}
	svc.rateLimitService = &RateLimitService{accountRepo: repo, cfg: cfg}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	return svc, account, conn, c, recorder, repo
}
