package service

import "github.com/Wei-Shaw/sub2api/internal/config"

const (
	openAIWSUpstreamReadLimitBytesDefault int64 = 64 * 1024 * 1024
	openAIWSUpstreamReadLimitBytesMax     int64 = 256 * 1024 * 1024
	openAIWSDownstreamReadLimitBytes      int64 = 16 * 1024 * 1024
)

// ResolveOpenAIWSUpstreamReadLimitBytes keeps trusted upstream response events
// separate from the lower limit used for untrusted downstream WS messages.
func ResolveOpenAIWSUpstreamReadLimitBytes(cfg *config.Config) int64 {
	if cfg == nil || cfg.Gateway.OpenAIWS.UpstreamReadLimitBytes <= 0 {
		return openAIWSUpstreamReadLimitBytesDefault
	}
	if cfg.Gateway.OpenAIWS.UpstreamReadLimitBytes > openAIWSUpstreamReadLimitBytesMax {
		return openAIWSUpstreamReadLimitBytesMax
	}
	return cfg.Gateway.OpenAIWS.UpstreamReadLimitBytes
}
