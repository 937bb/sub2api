//go:build unit

package service

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestSIWCUnsupportedHistoryCanSwitchWithoutDroppingContext(t *testing.T) {
	for _, body := range []string{
		`{"model":"gpt-6-astra","stream":true,"input":[{"role":"user","content":"summarize this history"},{"type":"compaction_trigger"}]}`,
		`{"model":"gpt-6-astra","stream":true,"input":[{"role":"user","content":"find a file"}],"tools":[{"type":"file_search","vector_store_ids":["vs_test"]}]}`,
	} {
		upstream := &httpUpstreamRecorder{resp: siwcRoutingResponse()}
		gateway := newOpenAIRejectedFieldTestService(upstream)
		gateway.accountRepo = &refreshAPIAccountRepo{}
		gateway.siwcTransport = siwcCatalogRoundTripper(func(*http.Request) (*http.Response, error) {
			t.Fatal("unsupported request must be skipped before sending upstream")
			return nil, nil
		})
		raw := []byte(body)
		c := newOpenAIRejectedFieldTestContext(raw)
		result, err := gateway.Forward(context.Background(), c, siwcFixtureAccount(), raw)
		var failover *UpstreamFailoverError
		require.ErrorAs(t, err, &failover)
		require.Nil(t, result, "a skipped account must not generate billing")
		require.True(t, failover.ShouldRetryNextAccount())
		require.False(t, failover.ShouldReportAccountScheduleFailure())
		require.False(t, c.Writer.Written(), "account switch must remain possible")
		require.Equal(t, body, string(raw), "original history must survive the retry")
		ordinary := siwcFixtureAccount()
		delete(ordinary.Credentials, "auth_mode")
		result, err = gateway.Forward(context.Background(), c, ordinary, raw)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, 1, len(upstream.requests))
		require.Equal(t, chatgptCodexURL, upstream.lastReq.URL.String())
	}
}

func TestSIWCStandaloneCompactExcludedEvenWhenForced(t *testing.T) {
	account := siwcFixtureAccount()
	for _, mode := range []string{"auto", "force_on", "force_off"} {
		account.Extra["openai_compact_mode"] = mode
		account.Extra["openai_compact_supported"] = true
		require.False(t, account.AllowsOpenAICompact())
	}
}

func TestSIWCMixedPoolFiltersUnsupportedRequestsBeforeSelection(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)
	ctx := WithSIWCRequestCompatibility(context.Background(), []byte(`{"model":"gpt-6-astra","input":[{"role":"user","content":"history"},{"type":"compaction_trigger"}],"stream":true}`))
	accounts := make([]Account, 0, 10)
	for i := range 9 {
		account := siwcFixtureAccount()
		account.ID = int64(100 + i)
		accounts = append(accounts, *account)
	}
	ordinary := siwcFixtureAccount()
	ordinary.ID, ordinary.Priority = 999, 99
	delete(ordinary.Credentials, "auth_mode")
	accounts = append(accounts, *ordinary)
	svc := &OpenAIGatewayService{accountRepo: schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache: &schedulerTestGatewayCache{}, cfg: &config.Config{},
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{})}
	for _, advanced := range []bool{false, true} {
		svc.rateLimitService = newOpenAIAdvancedSchedulerRateLimitService(strconv.FormatBool(advanced))
		svc.cfg.Gateway.Scheduling.LoadBatchEnabled = advanced
		require.Equal(t, advanced, svc.getOpenAIAccountScheduler(ctx) != nil)
		selection, _, err := svc.SelectAccountWithScheduler(ctx, nil, "", "", "gpt-6-astra", nil, OpenAIUpstreamTransportAny, false)
		require.NoError(t, err)
		require.NotNil(t, selection)
		require.Equal(t, ordinary.ID, selection.Account.ID)
		if selection.ReleaseFunc != nil {
			selection.ReleaseFunc()
		}
	}
}

func TestSIWCRequestCompatibilityKeepsSupportedHistoriesEligible(t *testing.T) {
	for _, body := range []string{
		`{"input":"hello"}`,
		`{"input":[{"type":"reasoning","encrypted_content":"opaque"},{"type":"function_call_output","call_id":"call_1","output":[{"type":"input_image","image_url":"data:image/png;base64,eA=="}]}]}`,
		`{"input":[{"type":"compaction","encrypted_content":"opaque"}],"tools":[{"type":"namespace","tools":[{"type":"custom","name":"exec"}]}]}`,
		`{"input":[{"type":"item_reference","id":"msg_previous"},{"type":"computer_call","call_id":"computer_1","action":{"type":"screenshot"}},{"type":"computer_call_output","call_id":"computer_1","output":{"type":"computer_screenshot","image_url":"data:image/png;base64,eA=="}}]}`,
		`{"input":[{"type":"tool_search_call","call_id":"search_1","arguments":{"query":"github"}},{"type":"tool_search_output","call_id":"search_1","tools":[]}],"tools":[{"type":"tool_search"}]}`,
		`{"previous_response_id":null,"conversation":"","tools":[{"type":"function","name":"lookup"},{"type":"web_search"}]}`,
	} {
		require.False(t, siwcRequestIncompatible(WithSIWCRequestCompatibility(context.Background(), []byte(body)), siwcFixtureAccount()), body)
	}
}
