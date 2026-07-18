package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func compactBodySignalContext(path string, body []byte) *gin.Context {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	return c
}

func TestOpenAICompactBodySignalPromotesBeforeStreamAndRouting(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","stream":true,"store":true,"prompt_cache_key":"session-1","input":[{"type":"compaction_trigger"}]}`)
	c := compactBodySignalContext("/v1/responses", body)
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.136.0")

	require.True(t, service.PromoteOpenAICompactBodySignal(c, body, false))
	require.Equal(t, "/v1/responses/compact", c.Request.URL.Path)
	require.True(t, service.IsOpenAICompactBodySignalRequest(c))

	normalized, changed, err := service.NormalizeOpenAICompactRequestBodyForTest(body)
	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, gjson.GetBytes(normalized, "stream").Exists())
	require.False(t, gjson.GetBytes(normalized, "store").Exists())
	require.Equal(t, "session-1", gjson.GetBytes(normalized, "prompt_cache_key").String())
	require.Equal(t, "compaction_trigger", gjson.GetBytes(normalized, "input.0.type").String())
}

func TestOpenAICompactBodySignalRequiresOfficialCodexIdentity(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","input":[{"type":"compaction_trigger"}]}`)
	c := compactBodySignalContext("/v1/responses", body)

	require.False(t, service.PromoteOpenAICompactBodySignal(c, body, false))
	require.Equal(t, "/v1/responses", c.Request.URL.Path)
	require.False(t, service.IsOpenAICompactBodySignalRequest(c))
}

func TestOpenAICompactBodySignalDoesNotPromoteResponseSubpaths(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","input":[{"type":"compaction_trigger"}]}`)
	c := compactBodySignalContext("/v1/responses/resp_123/cancel", body)
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.136.0")

	require.False(t, service.PromoteOpenAICompactBodySignal(c, body, false))
	require.Equal(t, "/v1/responses/resp_123/cancel", c.Request.URL.Path)
}

func TestOpenAICompactBodySignalForceCodexCLIIsExplicitOverride(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","input":[{"type":"compaction_trigger"}]}`)
	c := compactBodySignalContext("/v1/responses", body)
	h := &OpenAIGatewayHandler{cfg: &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: true}}}

	require.True(t, service.PromoteOpenAICompactBodySignal(c, body, h.cfg.Gateway.ForceCodexCLI))
	require.Equal(t, "/v1/responses/compact", c.Request.URL.Path)
}
