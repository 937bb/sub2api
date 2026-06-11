package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestOpenAIWSProtocolResolver_Resolve(t *testing.T) {
	baseCfg := &config.Config{}
	baseCfg.Gateway.OpenAIWS.Enabled = true
	baseCfg.Gateway.OpenAIWS.OAuthEnabled = true
	baseCfg.Gateway.OpenAIWS.APIKeyEnabled = true
	baseCfg.Gateway.OpenAIWS.ResponsesWebsockets = false
	baseCfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true

	openAIOAuthEnabled := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"openai_oauth_ws_mode": OpenAIOAuthWSModeManagedSession,
		},
	}

	t.Run("v2优先", func(t *testing.T) {
		decision := NewOpenAIWSProtocolResolver(baseCfg).Resolve(openAIOAuthEnabled)
		require.Equal(t, OpenAIUpstreamTransportResponsesWebsocketV2, decision.Transport)
		require.Equal(t, "ws_v2_enabled", decision.Reason)
	})

	t.Run("v2关闭时回退v1", func(t *testing.T) {
		cfg := *baseCfg
		cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = false
		cfg.Gateway.OpenAIWS.ResponsesWebsockets = true

		decision := NewOpenAIWSProtocolResolver(&cfg).Resolve(openAIOAuthEnabled)
		require.Equal(t, OpenAIUpstreamTransportResponsesWebsocket, decision.Transport)
		require.Equal(t, "ws_v1_enabled", decision.Reason)
	})

	t.Run("透传开关不影响WS协议判定", func(t *testing.T) {
		account := *openAIOAuthEnabled
		account.Extra = map[string]any{
			"openai_oauth_ws_mode": OpenAIOAuthWSModeManagedSession,
			"openai_passthrough":   true,
		}
		decision := NewOpenAIWSProtocolResolver(baseCfg).Resolve(&account)
		require.Equal(t, OpenAIUpstreamTransportResponsesWebsocketV2, decision.Transport)
		require.Equal(t, "ws_v2_enabled", decision.Reason)
	})

	t.Run("账号级强制HTTP", func(t *testing.T) {
		account := *openAIOAuthEnabled
		account.Extra = map[string]any{
			"openai_oauth_ws_mode": OpenAIOAuthWSModeManagedSession,
			"openai_ws_force_http": true,
		}
		decision := NewOpenAIWSProtocolResolver(baseCfg).Resolve(&account)
		require.Equal(t, OpenAIUpstreamTransportHTTPSSE, decision.Transport)
		require.Equal(t, "account_force_http", decision.Reason)
	})

	t.Run("全局关闭保持HTTP", func(t *testing.T) {
		cfg := *baseCfg
		cfg.Gateway.OpenAIWS.Enabled = false
		decision := NewOpenAIWSProtocolResolver(&cfg).Resolve(openAIOAuthEnabled)
		require.Equal(t, OpenAIUpstreamTransportHTTPSSE, decision.Transport)
		require.Equal(t, "global_disabled", decision.Reason)
	})

	t.Run("账号开关关闭保持HTTP", func(t *testing.T) {
		account := *openAIOAuthEnabled
		account.Extra = map[string]any{
			"openai_oauth_ws_mode": OpenAIOAuthWSModeOff,
		}
		decision := NewOpenAIWSProtocolResolver(baseCfg).Resolve(&account)
		require.Equal(t, OpenAIUpstreamTransportHTTPSSE, decision.Transport)
		require.Equal(t, "account_disabled", decision.Reason)
	})

	t.Run("OAuth账号不会读取API Key专用开关", func(t *testing.T) {
		account := *openAIOAuthEnabled
		account.Extra = map[string]any{
			"openai_apikey_responses_websockets_v2_enabled": true,
		}
		decision := NewOpenAIWSProtocolResolver(baseCfg).Resolve(&account)
		require.Equal(t, OpenAIUpstreamTransportHTTPSSE, decision.Transport)
		require.Equal(t, "account_disabled", decision.Reason)
	})

	t.Run("OAuth不再继承通用旧键openai_ws_enabled", func(t *testing.T) {
		account := *openAIOAuthEnabled
		account.Extra = map[string]any{
			"openai_ws_enabled": true,
		}
		decision := NewOpenAIWSProtocolResolver(baseCfg).Resolve(&account)
		require.Equal(t, OpenAIUpstreamTransportHTTPSSE, decision.Transport)
		require.Equal(t, "account_disabled", decision.Reason)
	})

	t.Run("SetupToken使用OAuth WS开关且不继承通用旧键", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeSetupToken,
			Extra: map[string]any{
				"openai_ws_enabled": true,
			},
		}
		decision := NewOpenAIWSProtocolResolver(baseCfg).Resolve(account)
		require.Equal(t, OpenAIUpstreamTransportHTTPSSE, decision.Transport)
		require.Equal(t, "account_disabled", decision.Reason)

		account.Extra = map[string]any{"openai_oauth_ws_mode": OpenAIOAuthWSModeManagedSession}
		decision = NewOpenAIWSProtocolResolver(baseCfg).Resolve(account)
		require.Equal(t, OpenAIUpstreamTransportResponsesWebsocketV2, decision.Transport)
		require.Equal(t, "ws_v2_enabled", decision.Reason)
	})

	t.Run("按账号类型开关控制", func(t *testing.T) {
		cfg := *baseCfg
		cfg.Gateway.OpenAIWS.OAuthEnabled = false
		decision := NewOpenAIWSProtocolResolver(&cfg).Resolve(openAIOAuthEnabled)
		require.Equal(t, OpenAIUpstreamTransportHTTPSSE, decision.Transport)
		require.Equal(t, "oauth_disabled", decision.Reason)
	})

	t.Run("API Key 账号关闭开关时回退HTTP", func(t *testing.T) {
		cfg := *baseCfg
		cfg.Gateway.OpenAIWS.APIKeyEnabled = false
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Extra: map[string]any{
				"openai_apikey_responses_websockets_v2_enabled": true,
			},
		}
		decision := NewOpenAIWSProtocolResolver(&cfg).Resolve(account)
		require.Equal(t, OpenAIUpstreamTransportHTTPSSE, decision.Transport)
		require.Equal(t, "apikey_disabled", decision.Reason)
	})

	t.Run("未知认证类型回退HTTP", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     "unknown_type",
			Extra: map[string]any{
				"responses_websockets_v2_enabled": true,
			},
		}
		decision := NewOpenAIWSProtocolResolver(baseCfg).Resolve(account)
		require.Equal(t, OpenAIUpstreamTransportHTTPSSE, decision.Transport)
		require.Equal(t, "unknown_auth_type", decision.Reason)
	})
}

func TestOpenAIWSProtocolResolver_Resolve_ModeRouterV2(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	cfg.Gateway.OpenAIWS.IngressModeDefault = OpenAIWSIngressModeCtxPool

	account := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Extra: map[string]any{
			"openai_oauth_ws_mode": OpenAIOAuthWSModeManagedSession,
		},
	}

	t.Run("ctx_pool mode routes to ws v2", func(t *testing.T) {
		decision := NewOpenAIWSProtocolResolver(cfg).Resolve(account)
		require.Equal(t, OpenAIUpstreamTransportResponsesWebsocketV2, decision.Transport)
		require.Equal(t, "ws_v2_mode_ctx_pool", decision.Reason)
	})

	t.Run("off mode routes to http", func(t *testing.T) {
		offAccount := &Account{
			Platform:    PlatformOpenAI,
			Type:        AccountTypeOAuth,
			Concurrency: 1,
			Extra: map[string]any{
				"openai_oauth_ws_mode": OpenAIOAuthWSModeOff,
			},
		}
		decision := NewOpenAIWSProtocolResolver(cfg).Resolve(offAccount)
		require.Equal(t, OpenAIUpstreamTransportHTTPSSE, decision.Transport)
		require.Equal(t, "account_mode_off", decision.Reason)
	})

	t.Run("legacy boolean maps to ctx_pool in v2 router", func(t *testing.T) {
		legacyAccount := &Account{
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Concurrency: 1,
			Extra: map[string]any{
				"openai_apikey_responses_websockets_v2_enabled": true,
			},
		}
		decision := NewOpenAIWSProtocolResolver(cfg).Resolve(legacyAccount)
		require.Equal(t, OpenAIUpstreamTransportResponsesWebsocketV2, decision.Transport)
		require.Equal(t, "ws_v2_mode_ctx_pool", decision.Reason)
	})

	t.Run("OAuth legacy passthrough mode no longer routes runtime", func(t *testing.T) {
		passthroughAccount := &Account{
			Platform:    PlatformOpenAI,
			Type:        AccountTypeOAuth,
			Concurrency: 1,
			Extra: map[string]any{
				"openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModePassthrough,
			},
		}
		decision := NewOpenAIWSProtocolResolver(cfg).Resolve(passthroughAccount)
		require.Equal(t, OpenAIUpstreamTransportHTTPSSE, decision.Transport)
		require.Equal(t, "account_mode_off", decision.Reason)
	})

	t.Run("OAuth nil extra缺少迁移后mode时关闭WS", func(t *testing.T) {
		cfgWithPassthroughDefault := *cfg
		cfgWithPassthroughDefault.Gateway.OpenAIWS.IngressModeDefault = OpenAIWSIngressModePassthrough
		passthroughAccount := &Account{
			Platform:    PlatformOpenAI,
			Type:        AccountTypeOAuth,
			Concurrency: 1,
		}
		decision := NewOpenAIWSProtocolResolver(&cfgWithPassthroughDefault).Resolve(passthroughAccount)
		require.Equal(t, OpenAIUpstreamTransportHTTPSSE, decision.Transport)
		require.Equal(t, "account_mode_off", decision.Reason)
	})

	t.Run("APIKey passthrough mode still routes to native relay", func(t *testing.T) {
		passthroughAccount := &Account{
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Concurrency: 1,
			Extra: map[string]any{
				"openai_apikey_responses_websockets_v2_mode": OpenAIWSIngressModePassthrough,
			},
		}
		decision := NewOpenAIWSProtocolResolver(cfg).Resolve(passthroughAccount)
		require.Equal(t, OpenAIUpstreamTransportResponsesWebsocketV2, decision.Transport)
		require.Equal(t, "ws_v2_mode_passthrough", decision.Reason)
	})

	t.Run("non-positive concurrency is rejected in v2 router", func(t *testing.T) {
		invalidConcurrency := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"openai_oauth_ws_mode": OpenAIOAuthWSModeManagedSession,
			},
		}
		decision := NewOpenAIWSProtocolResolver(cfg).Resolve(invalidConcurrency)
		require.Equal(t, OpenAIUpstreamTransportHTTPSSE, decision.Transport)
		require.Equal(t, "account_concurrency_invalid", decision.Reason)
	})
}
