package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestResolveOpenAIWSUpstreamReadLimitBytes(t *testing.T) {
	require.Equal(t, openAIWSUpstreamReadLimitBytesDefault, ResolveOpenAIWSUpstreamReadLimitBytes(nil))

	cfg := &config.Config{}
	require.Equal(t, openAIWSUpstreamReadLimitBytesDefault, ResolveOpenAIWSUpstreamReadLimitBytes(cfg))

	cfg.Gateway.OpenAIWS.UpstreamReadLimitBytes = 96 * 1024 * 1024
	require.Equal(t, int64(96*1024*1024), ResolveOpenAIWSUpstreamReadLimitBytes(cfg))

	cfg.Gateway.OpenAIWS.UpstreamReadLimitBytes = openAIWSUpstreamReadLimitBytesMax + 1
	require.Equal(t, openAIWSUpstreamReadLimitBytesMax, ResolveOpenAIWSUpstreamReadLimitBytes(cfg))
}

func TestNewOpenAIWSConnPoolUsesConfiguredUpstreamReadLimit(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.UpstreamReadLimitBytes = 96 * 1024 * 1024
	pool := newOpenAIWSConnPool(cfg)
	defer pool.Close()

	dialer, ok := pool.clientDialer.(*coderOpenAIWSClientDialer)
	require.True(t, ok)
	require.Equal(t, int64(96*1024*1024), dialer.upstreamReadLimitBytes)
}

func TestOpenAIWSPassthroughDialerUsesConfiguredUpstreamReadLimit(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.UpstreamReadLimitBytes = 96 * 1024 * 1024
	svc := &OpenAIGatewayService{cfg: cfg}

	dialer, ok := svc.getOpenAIWSPassthroughDialer().(*coderOpenAIWSClientDialer)
	require.True(t, ok)
	require.Equal(t, int64(96*1024*1024), dialer.upstreamReadLimitBytes)
}
