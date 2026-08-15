package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIGatewayServiceForwardGroupMappingIsFinalUpstreamModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(
			"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_group_mapping\",\"model\":\"gpt-5.6-sol-wm\",\"usage\":{\"input_tokens\":1,\"output_tokens\":2}}}\n\n" +
				"data: [DONE]\n\n",
		)),
	}}
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
	account := &Account{
		ID:          937,
		Name:        "group-model-mapping-test",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-account",
			"model_mapping": map[string]any{
				"gpt-5.6-sol": "must-not-win",
			},
		},
		Status:      StatusActive,
		Schedulable: true,
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
	MarkOpenAIGroupModelMapping(c, "gpt-5.6-sol", "gpt-5.6-sol-wm")

	body := []byte(`{"model":"gpt-5.6-sol","stream":true,"instructions":"mapping-test","input":"hello"}`)
	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "gpt-5.6-sol-wm", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, "gpt-5.6-sol", result.Model)
	require.Equal(t, "gpt-5.6-sol-wm", result.BillingModel)
	require.Equal(t, "gpt-5.6-sol-wm", result.UpstreamModel)
}
