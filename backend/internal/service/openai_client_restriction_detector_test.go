package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type panicReadCloser struct{}

func (panicReadCloser) Read(_ []byte) (int, error) {
	panic("request body must not be read")
}

func (panicReadCloser) Close() error {
	return nil
}

func newCodexDetectorTestContext(ua string, originator string) *gin.Context {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	if ua != "" {
		c.Request.Header.Set("User-Agent", ua)
	}
	if originator != "" {
		c.Request.Header.Set("originator", originator)
	}
	return c
}

func newCodexDetectorTestContextWithCodexHeader(ua string, originator string) *gin.Context {
	c := newCodexDetectorTestContext(ua, originator)
	c.Request.Header.Set("x-codex-installation-id", "test-installation")
	return c
}

func newCodexCLIOnlyDetectorTestAccount(extra map[string]any) *Account {
	if extra == nil {
		extra = map[string]any{"codex_cli_only": true}
	}
	return &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra:    extra,
	}
}

func TestOpenAICodexClientRestrictionDetector_Detect(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("未开启开关时绕过", func(t *testing.T) {
		detector := NewOpenAICodexClientRestrictionDetector(nil)
		account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{}}

		result := detector.Detect(newCodexDetectorTestContext("curl/8.0", ""), account, nil)
		require.False(t, result.Enabled)
		require.False(t, result.Matched)
		require.Equal(t, CodexClientRestrictionReasonDisabled, result.Reason)
	})

	t.Run("非 OAuth 账号即使配置开关也绕过", func(t *testing.T) {
		detector := NewOpenAICodexClientRestrictionDetector(nil)
		account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{"codex_cli_only": true}}

		result := detector.Detect(newCodexDetectorTestContext("curl/8.0", "codex_chatgpt_desktop"), account, nil)
		require.False(t, result.Enabled)
		require.False(t, result.Matched)
		require.Equal(t, CodexClientRestrictionReasonDisabled, result.Reason)
	})

	t.Run("开启后 codex_cli_rs 命中", func(t *testing.T) {
		detector := NewOpenAICodexClientRestrictionDetector(nil)
		account := newCodexCLIOnlyDetectorTestAccount(nil)

		result := detector.Detect(newCodexDetectorTestContextWithCodexHeader("codex_cli_rs/0.99.0", ""), account, nil)
		require.True(t, result.Enabled)
		require.True(t, result.Matched)
		require.Equal(t, CodexClientRestrictionReasonMatchedUA, result.Reason)
	})

	t.Run("开启后 codex_vscode 命中", func(t *testing.T) {
		detector := NewOpenAICodexClientRestrictionDetector(nil)
		account := newCodexCLIOnlyDetectorTestAccount(nil)

		result := detector.Detect(newCodexDetectorTestContextWithCodexHeader("codex_vscode/1.0.0", ""), account, nil)
		require.True(t, result.Enabled)
		require.True(t, result.Matched)
		require.Equal(t, CodexClientRestrictionReasonMatchedUA, result.Reason)
	})

	t.Run("开启后 codex_app 命中", func(t *testing.T) {
		detector := NewOpenAICodexClientRestrictionDetector(nil)
		account := newCodexCLIOnlyDetectorTestAccount(nil)

		result := detector.Detect(newCodexDetectorTestContextWithCodexHeader("codex_app/2.1.0", ""), account, nil)
		require.True(t, result.Enabled)
		require.True(t, result.Matched)
		require.Equal(t, CodexClientRestrictionReasonMatchedUA, result.Reason)
	})

	t.Run("开启后官方 UA 严格前缀命中", func(t *testing.T) {
		detector := NewOpenAICodexClientRestrictionDetector(nil)
		account := newCodexCLIOnlyDetectorTestAccount(nil)
		tests := []string{
			"codex_cli_rs/0.99.0",
			"codex-tui/0.136.0",
			"codex_vscode/1.0.0",
			"codex_vscode_copilot/1.0.0",
			"codex_app/2.1.0",
			"codex_chatgpt_desktop/1.0.0",
			"codex_atlas/0.1.0",
			"codex_exec/0.1.0",
			"codex_sdk_ts/0.1.0",
			"Codex Desktop/1.2.3",
			"Codex Some Future Client/9.9",
			"cccc/0.141.0 (Mac OS 26.5.0; arm64) Apple_Terminal/470.2 (codex-tui; 0.141.0)",
		}

		for _, ua := range tests {
			t.Run(ua, func(t *testing.T) {
				result := detector.Detect(newCodexDetectorTestContextWithCodexHeader(ua, ""), account, nil)
				require.True(t, result.Enabled)
				require.True(t, result.Matched)
				require.Equal(t, CodexClientRestrictionReasonMatchedUA, result.Reason)
			})
		}
	})

	t.Run("开启后官方 UA 缺少 x-codex 头拒绝", func(t *testing.T) {
		detector := NewOpenAICodexClientRestrictionDetector(nil)
		account := newCodexCLIOnlyDetectorTestAccount(nil)

		result := detector.Detect(newCodexDetectorTestContext("codex_cli_rs/0.99.0", ""), account, nil)
		require.True(t, result.Enabled)
		require.False(t, result.Matched)
		require.Equal(t, CodexClientRestrictionReasonNotMatchedUA, result.Reason)
	})

	t.Run("开启后官方 UA 只有空 x-codex 头拒绝", func(t *testing.T) {
		detector := NewOpenAICodexClientRestrictionDetector(nil)
		account := newCodexCLIOnlyDetectorTestAccount(nil)
		c := newCodexDetectorTestContext("codex_cli_rs/0.99.0", "")
		c.Request.Header.Set("x-codex-installation-id", "  ")
		c.Request.Header.Add("x-codex-window-id", "")

		result := detector.Detect(c, account, nil)
		require.True(t, result.Enabled)
		require.False(t, result.Matched)
		require.Equal(t, CodexClientRestrictionReasonNotMatchedUA, result.Reason)
	})

	t.Run("开启后官方 originator 不能单独命中", func(t *testing.T) {
		detector := NewOpenAICodexClientRestrictionDetector(nil)
		account := newCodexCLIOnlyDetectorTestAccount(nil)
		tests := []struct {
			name string
			ua   string
		}{
			{name: "empty_ua", ua: ""},
			{name: "curl", ua: "curl/8.0"},
			{name: "browser", ua: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36"},
			{name: "unparseable", ua: "???"},
			{name: "non_codex_cli", ua: "PostmanRuntime/7.36.0"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := detector.Detect(newCodexDetectorTestContext(tt.ua, "codex_chatgpt_desktop"), account, nil)
				require.True(t, result.Enabled)
				require.False(t, result.Matched)
				require.Equal(t, CodexClientRestrictionReasonNotMatchedUA, result.Reason)
			})
		}
	})

	t.Run("开启后非官方客户端拒绝", func(t *testing.T) {
		detector := NewOpenAICodexClientRestrictionDetector(nil)
		account := newCodexCLIOnlyDetectorTestAccount(nil)

		result := detector.Detect(newCodexDetectorTestContext("curl/8.0", "my_client"), account, nil)
		require.True(t, result.Enabled)
		require.False(t, result.Matched)
		require.Equal(t, CodexClientRestrictionReasonNotMatchedUA, result.Reason)
	})

	t.Run("开启后复合 UA 嵌入 codex_app 拒绝", func(t *testing.T) {
		detector := NewOpenAICodexClientRestrictionDetector(nil)
		account := newCodexCLIOnlyDetectorTestAccount(nil)

		result := detector.Detect(newCodexDetectorTestContext("Mozilla/5.0 codex_app/0.141.0", ""), account, nil)
		require.True(t, result.Enabled)
		require.False(t, result.Matched)
		require.Equal(t, CodexClientRestrictionReasonNotMatchedUA, result.Reason)
	})

	t.Run("开启后浏览器 UA trailer 拒绝", func(t *testing.T) {
		detector := NewOpenAICodexClientRestrictionDetector(nil)
		account := newCodexCLIOnlyDetectorTestAccount(nil)

		result := detector.Detect(newCodexDetectorTestContext("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) (codex-tui; 0.141.0)", ""), account, nil)
		require.True(t, result.Enabled)
		require.False(t, result.Matched)
		require.Equal(t, CodexClientRestrictionReasonNotMatchedUA, result.Reason)
	})

	t.Run("开启后伪造 originator 拒绝", func(t *testing.T) {
		detector := NewOpenAICodexClientRestrictionDetector(nil)
		account := newCodexCLIOnlyDetectorTestAccount(nil)
		tests := []string{"evil-codex_cli", "my_codex_thing", "codex_unknown", "Codex"}

		for _, originator := range tests {
			t.Run(originator, func(t *testing.T) {
				result := detector.Detect(newCodexDetectorTestContext("curl/8.0", originator), account, nil)
				require.True(t, result.Enabled)
				require.False(t, result.Matched)
				require.Equal(t, CodexClientRestrictionReasonNotMatchedUA, result.Reason)
			})
		}
	})

	t.Run("开启 ForceCodexCLI 时允许通过", func(t *testing.T) {
		detector := NewOpenAICodexClientRestrictionDetector(&config.Config{
			Gateway: config.GatewayConfig{ForceCodexCLI: true},
		})
		account := newCodexCLIOnlyDetectorTestAccount(nil)

		result := detector.Detect(newCodexDetectorTestContext("curl/8.0", "my_client"), account, nil)
		require.True(t, result.Enabled)
		require.True(t, result.Matched)
		require.Equal(t, CodexClientRestrictionReasonForceCodexCLI, result.Reason)
	})
}

func TestOpenAICodexClientRestrictionDetector_Detect_DoesNotReadRequestBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	detector := NewOpenAICodexClientRestrictionDetector(nil)
	c := newCodexDetectorTestContext("curl/8.0", "codex_chatgpt_desktop")
	c.Request.Body = panicReadCloser{}
	c.Request.GetBody = func() (io.ReadCloser, error) {
		panic("request body must not be opened")
	}

	result := detector.Detect(c, newCodexCLIOnlyDetectorTestAccount(nil), nil)
	require.True(t, result.Enabled)
	require.False(t, result.Matched)
	require.Equal(t, CodexClientRestrictionReasonNotMatchedUA, result.Reason)
}

func TestOpenAICodexClientRestrictionDetector_Detect_NilRequestFailsClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	detector := NewOpenAICodexClientRestrictionDetector(nil)

	result := detector.Detect(c, newCodexCLIOnlyDetectorTestAccount(nil), nil)
	require.True(t, result.Enabled)
	require.False(t, result.Matched)
	require.Equal(t, CodexClientRestrictionReasonNotMatchedUA, result.Reason)
}

func TestHasNonEmptyCodexHTTPHeader(t *testing.T) {
	tests := []struct {
		name    string
		headers http.Header
		want    bool
	}{
		{
			name:    "missing",
			headers: http.Header{"User-Agent": []string{"codex_cli_rs/0.99.0"}},
			want:    false,
		},
		{
			name:    "empty codex header",
			headers: http.Header{"x-codex-installation-id": []string{"", "  "}},
			want:    false,
		},
		{
			name:    "non empty lowercase codex header",
			headers: http.Header{"x-codex-installation-id": []string{"test-installation"}},
			want:    true,
		},
		{
			name:    "non empty canonical codex header",
			headers: http.Header{"X-Codex-Window-Id": []string{"window-1"}},
			want:    true,
		},
		{
			name:    "bounded prefix only",
			headers: http.Header{"x-codextra-installation-id": []string{"test-installation"}},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, hasNonEmptyCodexHTTPHeader(tt.headers))
		})
	}
}

func TestOpenAICodexClientRestrictionDetector_Detect_AllowedClients(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const (
		claudeCodeUA         = "Claude Code/0.5.0 (Macos 15.5; arm64) iTerm2.app (Claude Code; 1.0.4)"
		claudeCodeOriginator = "Claude Code"
	)

	t.Run("配置 claude_code 白名单且命中真实签名时放行", func(t *testing.T) {
		detector := NewOpenAICodexClientRestrictionDetector(nil)
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"codex_cli_only":                 true,
				"codex_cli_only_allowed_clients": []any{"claude_code"},
			},
		}

		result := detector.Detect(newCodexDetectorTestContext(claudeCodeUA, claudeCodeOriginator), account, nil)
		require.True(t, result.Enabled)
		require.True(t, result.Matched)
		require.Equal(t, CodexClientRestrictionReasonMatchedAllowedClient, result.Reason)
	})

	t.Run("配置白名单但伪造 originator 仍拒绝", func(t *testing.T) {
		detector := NewOpenAICodexClientRestrictionDetector(nil)
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"codex_cli_only":                 true,
				"codex_cli_only_allowed_clients": []any{"claude_code"},
			},
		}

		result := detector.Detect(newCodexDetectorTestContext(claudeCodeUA, "my_client"), account, nil)
		require.True(t, result.Enabled)
		require.False(t, result.Matched)
		require.Equal(t, CodexClientRestrictionReasonNotMatchedUA, result.Reason)
	})

	t.Run("未配置白名单时 Claude Code 签名仍拒绝", func(t *testing.T) {
		detector := NewOpenAICodexClientRestrictionDetector(nil)
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"codex_cli_only": true},
		}

		result := detector.Detect(newCodexDetectorTestContext(claudeCodeUA, claudeCodeOriginator), account, nil)
		require.True(t, result.Enabled)
		require.False(t, result.Matched)
		require.Equal(t, CodexClientRestrictionReasonNotMatchedUA, result.Reason)
	})

	t.Run("未开启 codex_cli_only 时白名单不参与，直接绕过", func(t *testing.T) {
		detector := NewOpenAICodexClientRestrictionDetector(nil)
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"codex_cli_only_allowed_clients": []any{"claude_code"}},
		}

		result := detector.Detect(newCodexDetectorTestContext(claudeCodeUA, claudeCodeOriginator), account, nil)
		require.False(t, result.Enabled)
		require.False(t, result.Matched)
		require.Equal(t, CodexClientRestrictionReasonDisabled, result.Reason)
	})

	t.Run("全局列表含 claude_code + 命中签名 → 放行(global)", func(t *testing.T) {
		detector := NewOpenAICodexClientRestrictionDetector(nil)
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"codex_cli_only": true},
		}
		result := detector.Detect(
			newCodexDetectorTestContext("Claude Code/0.5.0 (Macos 15.5; arm64) iTerm2.app (Claude Code; 1.0.4)", "Claude Code"),
			account,
			[]string{"claude_code"},
		)
		require.True(t, result.Enabled)
		require.True(t, result.Matched)
		require.Equal(t, CodexClientRestrictionReasonMatchedGlobalAllowedClient, result.Reason)
	})

	t.Run("全局列表含 claude_code + 非签名 → 403", func(t *testing.T) {
		detector := NewOpenAICodexClientRestrictionDetector(nil)
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"codex_cli_only": true},
		}
		result := detector.Detect(newCodexDetectorTestContext("curl/8.0", "my_client"), account, []string{"claude_code"})
		require.True(t, result.Enabled)
		require.False(t, result.Matched)
		require.Equal(t, CodexClientRestrictionReasonNotMatchedUA, result.Reason)
	})

	t.Run("全局列表为空 + 账号未配 → 403", func(t *testing.T) {
		detector := NewOpenAICodexClientRestrictionDetector(nil)
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"codex_cli_only": true},
		}
		result := detector.Detect(
			newCodexDetectorTestContext("Claude Code/0.5.0 (Macos) (Claude Code; 1.0.4)", "Claude Code"),
			account,
			nil,
		)
		require.True(t, result.Enabled)
		require.False(t, result.Matched)
		require.Equal(t, CodexClientRestrictionReasonNotMatchedUA, result.Reason)
	})

	t.Run("账号白名单优先于全局列表（reason=account）", func(t *testing.T) {
		detector := NewOpenAICodexClientRestrictionDetector(nil)
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"codex_cli_only":                 true,
				"codex_cli_only_allowed_clients": []any{"claude_code"},
			},
		}
		result := detector.Detect(
			newCodexDetectorTestContext("Claude Code/0.5.0 (Macos) (Claude Code; 1.0.4)", "Claude Code"),
			account,
			[]string{"claude_code"},
		)
		require.True(t, result.Matched)
		require.Equal(t, CodexClientRestrictionReasonMatchedAllowedClient, result.Reason)
	})
}
