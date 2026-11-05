package kiro

import (
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestEnvironmentStableAndOperationSpecific(t *testing.T) {
	c, err := Normalize(map[string]any{"refreshToken": "test", "machineId": strings.Repeat("a", 64), "systemVersion": "win32#10.0.22631"})
	require.NoError(t, err)
	again, err := Normalize(c)
	require.NoError(t, err)
	require.Equal(t, c, again)
	h, h2 := http.Header{}, http.Header{}
	Headers(h, c, "secret")
	EnvironmentHeaders(h2, c, "runtime")
	require.Contains(t, h.Get("User-Agent"), "os/win32#10.0.22631 lang/js md/nodejs#22.22.0 api/codewhispererstreaming#1.0.34")
	require.Contains(t, h2.Get("User-Agent"), "api/codewhispererruntime#1.0.0")
	require.Contains(t, h2.Get("User-Agent"), c["machine_id"])
	_, err = uuid.Parse(h.Get("amz-sdk-invocation-id"))
	require.NoError(t, err)
	require.NotEqual(t, h.Get("amz-sdk-invocation-id"), h2.Get("amz-sdk-invocation-id"))
	other, err := Normalize(map[string]any{"refreshToken": "other"})
	require.NoError(t, err)
	require.NotEqual(t, c["machine_id"], other["machine_id"])
	for _, key := range []string{"system_version", "node_version", "kiro_version"} {
		for _, value := range []any{"bad\r\nInjected:yes", 55, ""} {
			_, err := Normalize(map[string]any{"refreshToken": "x", key: value})
			require.Error(t, err)
		}
	}
	RefreshHeaders(h2, c)
	require.Equal(t, "KiroIDE-"+Version+"-"+String(c, "machine_id"), h2.Get("User-Agent"))
	c["auth_method"] = "idc"
	RefreshHeaders(h2, c)
	require.Contains(t, h2.Get("User-Agent"), "api/sso-oidc#3.980.0")
}
func TestQuotaPrecisionAndActiveBalances(t *testing.T) {
	now := time.Unix(1000, 0)
	q, err := ParseQuota([]byte(`{"subscriptionInfo":{"subscriptionTitle":"KIRO FREE"},"nextDateReset":3000,"usageBreakdownList":[{"resourceType":"CREDIT","currentUsage":10,"currentUsageWithPrecision":0,"usageLimit":50,"freeTrialInfo":{"freeTrialStatus":"ACTIVE","freeTrialExpiry":2000,"currentUsageWithPrecision":2.5,"usageLimitWithPrecision":100},"bonuses":[{"status":"ACTIVE","currentUsage":2,"usageLimit":50},{"status":"EXPIRED","usageLimit":500},{"status":"ACTIVE","expiresAt":999,"usageLimit":500}]}]}`), now)
	require.NoError(t, err)
	require.Equal(t, 4.5, q.Used)
	require.Equal(t, 200.0, q.Limit)
	require.InDelta(t, 2.25, *q.Utilization, 0.001)
	require.EqualValues(t, 3000, q.ResetsAt.Unix())
	q, err = ParseQuota([]byte(`{"usageBreakdownList":[{"usageLimit":0,"currentUsage":3}]}`), now)
	require.NoError(t, err)
	require.Nil(t, q.Utilization)
	_, err = ParseQuota([]byte(`{"usageBreakdownList":[{"usageLimit":-1}]}`), now)
	require.Error(t, err)
	q, err = ParseQuota([]byte(`{"usageBreakdownList":[{"resourceType":"OTHER","usageLimit":999},{"resourceType":"CREDIT","usageLimit":10,"currentUsage":15}]}`), now)
	require.NoError(t, err)
	require.Equal(t, 150.0, *q.Utilization)
	require.Contains(t, UsageURL(map[string]any{"profile_arn": "arn:test&foo=bar"}), "profileArn=arn%3Atest%26foo%3Dbar")
}
