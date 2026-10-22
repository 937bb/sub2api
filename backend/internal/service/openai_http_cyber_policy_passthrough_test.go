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

func TestOpenAIHTTPForbiddenCyberPolicyPreservesStatusAndMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	body := `{"error":{"type":"invalid_request_error","code":"cyber_policy","message":"This content was flagged for possible cybersecurity risk."}}`
	resp := &http.Response{
		StatusCode: http.StatusForbidden,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	account := &Account{ID: 1, Name: "upstream", Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	err := svc.handleErrorResponsePassthrough(context.Background(), resp, c, account, nil, []byte(body))

	require.Error(t, err)
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Equal(t, "cyber_policy", gjson.Get(rec.Body.String(), "error.code").String())
	require.Contains(t, gjson.Get(rec.Body.String(), "error.message").String(), "possible cybersecurity risk")
	require.NotNil(t, GetOpsCyberPolicy(c))
}
