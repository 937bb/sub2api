package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCodexUSLocaleConfig(t *testing.T) {
	for _, enabled := range []string{"false", "true"} {
		t.Run(enabled, func(t *testing.T) {
			resetViperWithJWTSecret(t)
			t.Setenv("GATEWAY_CODEX_US_LOCALE", enabled)
			t.Setenv("TZ", "Asia/Shanghai")
			cfg, err := Load()
			require.NoError(t, err)
			require.Equal(t, enabled == "true", cfg.Gateway.CodexUSLocale)
			require.Equal(t, "Asia/Shanghai", cfg.Timezone)
		})
	}
}

func TestCodexUSLocaleDefaultsToEnabled(t *testing.T) {
	resetViperWithJWTSecret(t)
	t.Setenv("GATEWAY_CODEX_US_LOCALE", "")

	cfg, err := Load()
	require.NoError(t, err)
	require.True(t, cfg.Gateway.CodexUSLocale)
}
