package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccount_OpenAISetupTokenAdapterBoundaryHelpers(t *testing.T) {
	setupToken := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeSetupToken,
		Credentials: map[string]any{
			"chatgpt_account_id": "chatgpt-acc",
		},
	}
	require.Equal(t, "chatgpt-acc", setupToken.GetChatGPTAccountID())
	require.False(t, setupToken.IsPrivacySet(), "setup-token uses the adapter boundary but must not claim full OAuth privacy state")

	oauthWithoutPrivacy := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	require.False(t, oauthWithoutPrivacy.IsPrivacySet())

	apiKeyWithoutPrivacy := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	require.False(t, apiKeyWithoutPrivacy.IsPrivacySet())
}

func TestShouldBlockAccountForPrivacyRequirement_OpenAIOnlyBlocksFullOAuth(t *testing.T) {
	group := &Group{Name: "privacy-required", Platform: PlatformOpenAI, RequirePrivacySet: true}

	require.True(t, shouldBlockAccountForPrivacyRequirement(&Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, group))
	require.False(t, shouldBlockAccountForPrivacyRequirement(&Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{"privacy_mode": PrivacyModeTrainingOff},
	}, group))
	require.False(t, shouldBlockAccountForPrivacyRequirement(&Account{Platform: PlatformOpenAI, Type: AccountTypeSetupToken}, group))
	require.False(t, shouldBlockAccountForPrivacyRequirement(&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, group))
}

func TestAccount_IsOpenAIAPIKeyPassthroughEnabled(t *testing.T) {
	t.Run("APIKey new field enables passthrough", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Extra: map[string]any{
				"openai_passthrough": true,
			},
		}
		require.True(t, account.IsOpenAIAPIKeyPassthroughEnabled())
		require.True(t, account.IsOpenAIPassthroughEnabled())
	})

	t.Run("APIKey legacy oauth field remains ignored", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Extra: map[string]any{
				"openai_oauth_passthrough": true,
			},
		}
		require.False(t, account.IsOpenAIAPIKeyPassthroughEnabled())
		require.False(t, account.IsOpenAIPassthroughEnabled())
	})

	t.Run("OAuth legacy generic flag does not enable APIKey passthrough", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"openai_passthrough": true,
			},
		}
		require.False(t, account.IsOpenAIAPIKeyPassthroughEnabled())
		require.False(t, account.IsOpenAIPassthroughEnabled())
	})

	t.Run("OAuth legacy oauth flag does not enable APIKey passthrough", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"openai_oauth_passthrough": true,
			},
		}
		require.False(t, account.IsOpenAIAPIKeyPassthroughEnabled())
		require.False(t, account.IsOpenAIPassthroughEnabled())
	})

	t.Run("non OpenAI account always disabled", func(t *testing.T) {
		account := &Account{
			Platform: PlatformAnthropic,
			Type:     AccountTypeAPIKey,
			Extra: map[string]any{
				"openai_passthrough": true,
			},
		}
		require.False(t, account.IsOpenAIAPIKeyPassthroughEnabled())
		require.False(t, account.IsOpenAIPassthroughEnabled())
	})

	t.Run("missing extra defaults disabled", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
		}
		require.False(t, account.IsOpenAIAPIKeyPassthroughEnabled())
		require.False(t, account.IsOpenAIPassthroughEnabled())
	})
}

func TestAccount_IsCodexCLIOnlyEnabled(t *testing.T) {
	t.Run("OpenAI OAuth 开启", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"codex_cli_only": true,
			},
		}
		require.True(t, account.IsCodexCLIOnlyEnabled())
	})

	t.Run("OpenAI OAuth 关闭", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"codex_cli_only": false,
			},
		}
		require.False(t, account.IsCodexCLIOnlyEnabled())
	})

	t.Run("字段缺失默认关闭", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{},
		}
		require.False(t, account.IsCodexCLIOnlyEnabled())
	})

	t.Run("类型非法默认关闭", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"codex_cli_only": "true",
			},
		}
		require.False(t, account.IsCodexCLIOnlyEnabled())
	})

	t.Run("非 OAuth 账号始终关闭", func(t *testing.T) {
		apiKeyAccount := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Extra: map[string]any{
				"codex_cli_only": true,
			},
		}
		require.False(t, apiKeyAccount.IsCodexCLIOnlyEnabled())

		otherPlatform := &Account{
			Platform: PlatformAnthropic,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"codex_cli_only": true,
			},
		}
		require.False(t, otherPlatform.IsCodexCLIOnlyEnabled())
	})
}

func TestAccount_IsOpenAIResponsesWebSocketV2Enabled(t *testing.T) {
	t.Run("OAuth使用迁移后的OAuth专用WS mode", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"openai_oauth_ws_mode": "managed_session",
			},
		}
		require.True(t, account.IsOpenAIResponsesWebSocketV2Enabled())
	})

	t.Run("OAuth迁移后的off mode关闭WS", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"openai_oauth_ws_mode":                         "off",
				"openai_oauth_responses_websockets_v2_enabled": true,
			},
		}
		require.False(t, account.IsOpenAIResponsesWebSocketV2Enabled())
	})

	t.Run("OAuth运行时不再读取旧OAuth WS开关", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"openai_oauth_responses_websockets_v2_enabled": true,
			},
		}
		require.False(t, account.IsOpenAIResponsesWebSocketV2Enabled())
	})

	t.Run("API Key使用API Key专用开关", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Extra: map[string]any{
				"openai_apikey_responses_websockets_v2_enabled": true,
			},
		}
		require.True(t, account.IsOpenAIResponsesWebSocketV2Enabled())
	})

	t.Run("OAuth账号不会读取API Key专用开关", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"openai_apikey_responses_websockets_v2_enabled": true,
			},
		}
		require.False(t, account.IsOpenAIResponsesWebSocketV2Enabled())
	})

	t.Run("OAuth运行时不继承通用兼容WS开关", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"responses_websockets_v2_enabled": true,
				"openai_ws_enabled":               true,
			},
		}
		require.False(t, account.IsOpenAIResponsesWebSocketV2Enabled())
	})

	t.Run("SetupToken使用迁移后的OAuth专用WS mode", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeSetupToken,
			Extra: map[string]any{
				"openai_oauth_ws_mode": "managed_session",
			},
		}
		require.True(t, account.IsOpenAIResponsesWebSocketV2Enabled())
	})

	t.Run("SetupToken运行时不继承通用兼容WS开关", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeSetupToken,
			Extra: map[string]any{
				"responses_websockets_v2_enabled": true,
				"openai_ws_enabled":               true,
			},
		}
		require.False(t, account.IsOpenAIResponsesWebSocketV2Enabled())
	})

	t.Run("分类型键缺失时回退兼容键", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Extra: map[string]any{
				"responses_websockets_v2_enabled": true,
			},
		}
		require.True(t, account.IsOpenAIResponsesWebSocketV2Enabled())
	})

	t.Run("非OpenAI账号默认关闭", func(t *testing.T) {
		account := &Account{
			Platform: PlatformAnthropic,
			Type:     AccountTypeAPIKey,
			Extra: map[string]any{
				"responses_websockets_v2_enabled": true,
			},
		}
		require.False(t, account.IsOpenAIResponsesWebSocketV2Enabled())
	})
}

func TestAccount_ResolveOpenAIResponsesWebSocketV2Mode(t *testing.T) {
	t.Run("OAuth迁移后的managed_session mode resolves to ctx_pool adapter", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"openai_oauth_ws_mode":                      "managed_session",
				"openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModeOff,
			},
		}
		require.Equal(t, OpenAIWSIngressModeCtxPool, account.ResolveOpenAIResponsesWebSocketV2Mode(OpenAIWSIngressModeOff))
	})

	t.Run("OAuth迁移后的off mode resolves to off", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"openai_oauth_ws_mode": "off",
			},
		}
		require.Equal(t, OpenAIWSIngressModeOff, account.ResolveOpenAIResponsesWebSocketV2Mode(OpenAIWSIngressModeCtxPool))
	})

	t.Run("OAuth迁移后的新mode不接受passthrough别名", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"openai_oauth_ws_mode": OpenAIWSIngressModePassthrough,
			},
		}
		require.False(t, account.IsOpenAIResponsesWebSocketV2Enabled())
		require.Equal(t, OpenAIWSIngressModeOff, account.ResolveOpenAIResponsesWebSocketV2Mode(OpenAIWSIngressModeCtxPool))
	})

	t.Run("APIKey default fallback to ctx_pool", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Extra:    map[string]any{},
		}
		require.Equal(t, OpenAIWSIngressModeCtxPool, account.ResolveOpenAIResponsesWebSocketV2Mode(""))
		require.Equal(t, OpenAIWSIngressModeCtxPool, account.ResolveOpenAIResponsesWebSocketV2Mode("invalid"))
	})

	t.Run("OAuth运行时不再读取旧OAuth WS mode", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModePassthrough,
			},
		}
		require.Equal(t, OpenAIWSIngressModeOff, account.ResolveOpenAIResponsesWebSocketV2Mode(OpenAIWSIngressModeCtxPool))
	})

	t.Run("OAuth缺少迁移后mode时默认fail closed", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{},
		}
		require.Equal(t, OpenAIWSIngressModeOff, account.ResolveOpenAIResponsesWebSocketV2Mode(OpenAIWSIngressModePassthrough))
	})

	t.Run("SetupToken使用迁移后的managed_session mode resolves to ctx_pool adapter", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeSetupToken,
			Extra: map[string]any{
				"openai_oauth_ws_mode": "managed_session",
			},
		}
		require.Equal(t, OpenAIWSIngressModeCtxPool, account.ResolveOpenAIResponsesWebSocketV2Mode(OpenAIWSIngressModePassthrough))
	})

	t.Run("SetupToken运行时不继承通用兼容WS开关或passthrough默认", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeSetupToken,
			Extra: map[string]any{
				"responses_websockets_v2_enabled": true,
				"openai_ws_enabled":               true,
			},
		}
		require.Equal(t, OpenAIWSIngressModeOff, account.ResolveOpenAIResponsesWebSocketV2Mode(OpenAIWSIngressModePassthrough))
	})

	t.Run("OAuth nil extra缺少迁移后mode时默认fail closed", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
		}
		require.Equal(t, OpenAIWSIngressModeOff, account.ResolveOpenAIResponsesWebSocketV2Mode(OpenAIWSIngressModePassthrough))
	})

	t.Run("APIKey passthrough mode remains native relay", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Extra: map[string]any{
				"openai_apikey_responses_websockets_v2_mode": OpenAIWSIngressModePassthrough,
			},
		}
		require.Equal(t, OpenAIWSIngressModePassthrough, account.ResolveOpenAIResponsesWebSocketV2Mode(OpenAIWSIngressModeCtxPool))
	})

	t.Run("legacy enabled maps to ctx_pool", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Extra: map[string]any{
				"responses_websockets_v2_enabled": true,
			},
		}
		require.Equal(t, OpenAIWSIngressModeCtxPool, account.ResolveOpenAIResponsesWebSocketV2Mode(OpenAIWSIngressModeOff))
	})

	t.Run("APIKey default shared/dedicated mode strings are compatible with ctx_pool", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Extra:    map[string]any{},
		}
		require.Equal(t, OpenAIWSIngressModeCtxPool, account.ResolveOpenAIResponsesWebSocketV2Mode(OpenAIWSIngressModeShared))
		require.Equal(t, OpenAIWSIngressModeCtxPool, account.ResolveOpenAIResponsesWebSocketV2Mode(OpenAIWSIngressModeDedicated))
		require.Equal(t, OpenAIWSIngressModeCtxPool, normalizeOpenAIWSIngressDefaultMode(OpenAIWSIngressModeShared))
		require.Equal(t, OpenAIWSIngressModeCtxPool, normalizeOpenAIWSIngressDefaultMode(OpenAIWSIngressModeDedicated))
	})

	t.Run("legacy disabled maps to off", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Extra: map[string]any{
				"openai_apikey_responses_websockets_v2_enabled": false,
				"responses_websockets_v2_enabled":               true,
			},
		}
		require.Equal(t, OpenAIWSIngressModeOff, account.ResolveOpenAIResponsesWebSocketV2Mode(OpenAIWSIngressModeCtxPool))
	})

	t.Run("non openai always off", func(t *testing.T) {
		account := &Account{
			Platform: PlatformAnthropic,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModeDedicated,
			},
		}
		require.Equal(t, OpenAIWSIngressModeOff, account.ResolveOpenAIResponsesWebSocketV2Mode(OpenAIWSIngressModeDedicated))
	})
}

func TestAccount_OpenAIWSExtraFlags(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"openai_ws_force_http":           true,
			"openai_ws_allow_store_recovery": true,
		},
	}
	require.True(t, account.IsOpenAIWSForceHTTPEnabled())
	require.True(t, account.IsOpenAIWSAllowStoreRecoveryEnabled())

	off := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{}}
	require.False(t, off.IsOpenAIWSForceHTTPEnabled())
	require.False(t, off.IsOpenAIWSAllowStoreRecoveryEnabled())

	var nilAccount *Account
	require.False(t, nilAccount.IsOpenAIWSAllowStoreRecoveryEnabled())

	nonOpenAI := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"openai_ws_allow_store_recovery": true,
		},
	}
	require.False(t, nonOpenAI.IsOpenAIWSAllowStoreRecoveryEnabled())
}
