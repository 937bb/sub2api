package service

import (
	"context"
	"errors"
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

func TestOpenAIGatewayService_NativeOpenAIHTTPRelayRejectsOAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptestNewRecorderForNativePassthroughBoundary()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptestNewRequestForNativePassthroughBoundary()

	svc := &OpenAIGatewayService{}
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"openai_passthrough": true,
		},
	}

	_, err := svc.forwardOpenAIPassthrough(
		context.Background(),
		c,
		account,
		[]byte(`{"model":"gpt-5","input":"hi","instructions":"be helpful"}`),
		"gpt-5",
		nil,
		true,
		time.Now(),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "requires APIKey account")
}

func TestOpenAIGatewayService_OAuthHTTPResidualLegacyConfigUsesAdapter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{}`))
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.136.0")
	c.Request.Header.Set("X-Api-Key", "sk-inbound")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid-oauth-adapter"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"type":"response.completed","response":{"id":"resp_oauth_adapter","usage":{"input_tokens":1,"output_tokens":1}}}`,
			"",
			"data: [DONE]",
			"",
		}, "\n"))),
	}}
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{},
		httpUpstream: upstream,
	}
	account := &Account{
		ID:          77,
		Name:        "oauth-legacy-residual",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
		},
		Extra: map[string]any{
			"openai_oauth_passthrough": true,
			"openai_passthrough":       true,
		},
		Status:      StatusActive,
		Schedulable: true,
	}
	body := []byte(`{"type":"response.create","generate":false,"previous_response_id":"resp_legacy","model":"gpt-5.2","stream":true,"store":true,"instructions":"be helpful","input":"hi","unknown_field":"drop"}`)

	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, chatgptCodexURL, upstream.lastReq.URL.String())
	require.Equal(t, "chatgpt.com", upstream.lastReq.Host)
	require.Equal(t, "Bearer oauth-token", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, "chatgpt-acc", upstream.lastReq.Header.Get("chatgpt-account-id"))
	require.Empty(t, upstream.lastReq.Header.Get("X-Api-Key"))
	require.False(t, gjson.GetBytes(upstream.lastBody, "unknown_field").Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, "previous_response_id").Exists())
	passthroughFlag, _ := c.Get("openai_passthrough")
	require.NotEqual(t, true, passthroughFlag)
}

func TestOpenAIGatewayService_NativeOpenAIWSRelayRejectsOAuth(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModePassthrough,
		},
	}

	err := svc.proxyResponsesWebSocketV2Passthrough(
		context.Background(),
		nil,
		nil,
		account,
		"oauth-token",
		[]byte(`{"type":"response.create","model":"gpt-5","input":"hi"}`),
		nil,
		OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2},
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "requires APIKey account")
}

func TestOpenAIGatewayService_OAuthWSResidualLegacyModeDoesNotEnterNativeRelay(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	cfg.Gateway.OpenAIWS.IngressModeDefault = OpenAIWSIngressModePassthrough

	captureDialer := &openAIWSCaptureDialer{conn: &openAIWSCaptureConn{}}
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(captureDialer)
	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     &httpUpstreamRecorder{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		openaiWSPool:     pool,
	}
	account := &Account{
		ID:          78,
		Name:        "oauth-legacy-ws-passthrough",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{"access_token": "oauth-token"},
		Extra: map[string]any{
			"openai_oauth_passthrough":                  true,
			"openai_passthrough":                        true,
			"openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModePassthrough,
		},
		Status:      StatusActive,
		Schedulable: true,
	}

	errCh := make(chan error, 1)
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeover})
		if err != nil {
			errCh <- err
			return
		}
		defer func() { _ = conn.CloseNow() }()

		rec := httptest.NewRecorder()
		ginCtx, _ := gin.CreateTestContext(rec)
		req := r.Clone(r.Context())
		req.Header = req.Header.Clone()
		req.Header.Set("User-Agent", "codex_cli_rs/0.136.0")
		ginCtx.Request = req

		readCtx, cancelRead := context.WithTimeout(r.Context(), 3*time.Second)
		msgType, firstMessage, readErr := conn.Read(readCtx)
		cancelRead()
		if readErr != nil {
			errCh <- readErr
			return
		}
		if msgType != coderws.MessageText && msgType != coderws.MessageBinary {
			errCh <- errors.New("unsupported websocket client message type")
			return
		}

		errCh <- svc.ProxyResponsesWebSocketFromClient(r.Context(), ginCtx, conn, account, "oauth-token", firstMessage, nil)
	}))
	defer wsServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http"), nil)
	cancelDial()
	require.NoError(t, err)
	defer func() { _ = clientConn.CloseNow() }()

	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5","stream":false,"input":"hi"}`))
	cancelWrite()
	require.NoError(t, err)

	select {
	case serverErr := <-errCh:
		var closeErr *OpenAIWSClientCloseError
		require.ErrorAs(t, serverErr, &closeErr)
		require.Equal(t, coderws.StatusPolicyViolation, closeErr.StatusCode())
		require.Equal(t, "websocket mode is disabled for this account", closeErr.Reason())
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for OAuth websocket boundary check")
	}
	require.Equal(t, 0, captureDialer.DialCount(), "OAuth legacy passthrough mode must not enter native WS relay")
}

func httptestNewRecorderForNativePassthroughBoundary() *httptest.ResponseRecorder {
	return httptest.NewRecorder()
}

func httptestNewRequestForNativePassthroughBoundary() *http.Request {
	return httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{}`))
}
