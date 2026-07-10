//go:build unit

package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCodexVersionConstants_Consistency(t *testing.T) {
	require.True(t, strings.Contains(codexCLIUserAgent, codexCLIVersion),
		"codexCLIUserAgent must embed codexCLIVersion")
	require.Equal(t, codexCLIUserAgent, DefaultOpenAICodexUserAgent,
		"default and request-time Codex user agents must stay in sync")
}
