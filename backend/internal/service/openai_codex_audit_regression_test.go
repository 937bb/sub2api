package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestCodexParentReferencesShareExistingThreadAndTurnNamespace(t *testing.T) {
	account := accountDeviceFixture(11, "owner")
	body := []byte(`{"input":[],"client_metadata":{"parent_thread_id":"parent","x-codex-parent-thread-id":"parent","forked_from_thread_id":"parent","parent_turn_id":"turn","x-codex-turn-metadata":"{\"parent_thread_id\":\"parent\",\"parent_turn_id\":\"turn\",\"counter\":9007199254740993,\"analytics_enabled\":false}"}}`)
	parent := scopeCodexAccountIdentityValue(account, 77, "thread", "parent")
	turn := scopeCodexAccountIdentityValue(account, 77, "turn", "turn")
	out, changed, err := applyCodexAccountIdentityClientMetadataRaw(body, account, 77)
	require.NoError(t, err)
	require.True(t, changed)
	for _, name := range []string{"parent_thread_id", "x-codex-parent-thread-id", "forked_from_thread_id"} {
		require.Equal(t, parent, gjson.GetBytes(out, "client_metadata."+name).String())
	}
	require.Equal(t, turn, gjson.GetBytes(out, "client_metadata.parent_turn_id").String())
	embedded := gjson.GetBytes(out, "client_metadata.x-codex-turn-metadata").String()
	require.Equal(t, parent, gjson.Get(embedded, "parent_thread_id").String())
	require.Equal(t, turn, gjson.Get(embedded, "parent_turn_id").String())
	require.Equal(t, "9007199254740993", gjson.Get(embedded, "counter").Raw)
	require.Equal(t, "false", gjson.Get(embedded, "analytics_enabled").Raw)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(body, &decoded))
	require.True(t, applyCodexAccountIdentityClientMetadataMap(decoded, account, 77))
	encoded, err := json.Marshal(decoded)
	require.NoError(t, err)
	require.JSONEq(t, string(out), string(encoded))
	other, _, err := applyCodexAccountIdentityClientMetadataRaw(body, account, 78)
	require.NoError(t, err)
	require.NotEqual(t, parent, gjson.GetBytes(other, "client_metadata.parent_thread_id").String())
	require.Contains(t, string(body), `"parent_thread_id":"parent"`)
}

func TestCodexSubagentHeadersSurviveHTTPPassthroughAndWS(t *testing.T) {
	account := accountDeviceFixture(11, "owner")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	c.Set("api_key", &APIKey{ID: 77})
	c.Request.Header.Set(codexParentThreadIDHeader, "parent")
	c.Request.Header.Set(openAISubagentHeader, "guardian")
	svc := &OpenAIGatewayService{}
	body := []byte(`{"input":[]}`)
	req, err := svc.buildUpstreamRequest(context.Background(), c, account, body, "fixture", true, "", true)
	require.NoError(t, err)
	passthrough, err := svc.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, account, body, "fixture")
	require.NoError(t, err)
	ws, _, err := svc.buildOpenAIWSHeaders(context.Background(), c, account, "fixture", OpenAIWSProtocolDecision{}, true, "", "", "", "", "")
	require.NoError(t, err)
	for _, h := range []http.Header{req.Header, passthrough.Header, ws} {
		require.Equal(t, "guardian", h.Get(openAISubagentHeader))
		require.Equal(t, scopeCodexAccountIdentityValue(account, 77, "thread", "parent"), h.Get(codexParentThreadIDHeader))
	}
	require.Equal(t, "parent", c.Request.Header.Get(codexParentThreadIDHeader))
}

func TestCodexMetadataPreservesExecutionAndAnalytics(t *testing.T) {
	account := accountDeviceFixture(11, "owner")
	for _, raw := range []string{`{"client_metadata":{"thread_id":"fixture"}}`, `{"client_metadata":{"thread_id":"fixture","analytics_enabled":false,"sandbox":"seatbelt","thread_source":"user"}}`} {
		out, _, err := applyCodexAccountIdentityClientMetadataRaw([]byte(raw), account, 77)
		require.NoError(t, err)
		for _, field := range []string{"analytics_enabled", "sandbox", "thread_source", "timezone", "locale"} {
			require.Equal(t, gjson.Get(raw, "client_metadata."+field).Raw, gjson.GetBytes(out, "client_metadata."+field).Raw)
		}
	}
}
