package service

import (
	"context"
	"errors"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
	"github.com/gin-gonic/gin"
)

func supportsMixedKiroPlatform(platform string) bool {
	return platform == PlatformAnthropic || platform == PlatformGemini || platform == PlatformOpenAI
}
func mixedAccountPlatforms(platform string) []string {
	out := []string{platform, PlatformKiro}
	if platform == PlatformAnthropic || platform == PlatformGemini {
		out = append(out, PlatformAntigravity)
	}
	return out
}
func isMixedAccountForPlatform(a *Account, platform string) bool {
	if a == nil {
		return false
	}
	if a.Platform == platform {
		return true
	}
	if a.Platform == PlatformKiro {
		return supportsMixedKiroPlatform(platform)
	}
	return a.Platform == PlatformAntigravity && (platform == PlatformAnthropic || platform == PlatformGemini) && a.IsMixedSchedulingEnabled()
}

// Explicit aliases may target any discovered Kiro model. Default routing accepts
// the native model families verified in Kiro's catalog, without vendor remapping.
// The selected account's live catalog remains authoritative before inference.
func (a *Account) supportsKiroModel(model string) bool {
	mapping := a.GetModelMapping()
	if len(mapping) > 0 {
		return mappingSupportsRequestedModel(mapping, model)
	}
	model = kiro.ModelID(model)
	switch model {
	case "auto", "gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna", "kimi-k3",
		"deepseek-3.2", "minimax-m2.5", "minimax-m2.1", "glm-5", "qwen3-coder-next":
		return true
	}
	return strings.HasPrefix(model, "claude-haiku-") || strings.HasPrefix(model, "claude-sonnet-") || strings.HasPrefix(model, "claude-opus-")
}

func kiroParsedRequest(c *gin.Context, body []byte) (*ParsedRequest, error) {
	p, err := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
	if err != nil {
		return nil, err
	}
	if value, ok := c.Get("api_key"); ok {
		if key, ok := value.(*APIKey); ok {
			p.GroupID = key.GroupID
		}
	}
	return p, nil
}

func (s *OpenAIGatewayService) forwardKiroCompatible(ctx context.Context, c *gin.Context, a *Account, body []byte, protocol string) (*OpenAIForwardResult, error) {
	if s.kiroGateway == nil {
		return nil, errors.New("Kiro gateway unavailable")
	}
	p, err := kiroParsedRequest(c, body)
	if err != nil {
		return nil, err
	}
	var result *ForwardResult
	switch protocol {
	case "messages":
		result, err = s.kiroGateway.forwardKiro(ctx, c, a, p)
	case "chat":
		result, err = s.kiroGateway.ForwardAsChatCompletions(ctx, c, a, body, p)
	default:
		result, err = s.kiroGateway.ForwardAsResponses(ctx, c, a, body, p)
	}
	if result == nil {
		return nil, err
	}
	return &OpenAIForwardResult{
		RequestID: result.RequestID, UpstreamHeaders: result.UpstreamHeaders,
		Model: result.Model, UpstreamModel: result.UpstreamModel,
		UpstreamResponseModel: result.UpstreamResponseModel, UpstreamResponseModelConflict: result.UpstreamResponseModelConflict,
		Usage: OpenAIUsage{
			// OpenAI billing expects total input, including both cache buckets.
			InputTokens:              result.Usage.InputTokens + result.Usage.CacheCreationInputTokens + result.Usage.CacheReadInputTokens,
			OutputTokens:             result.Usage.OutputTokens,
			CacheCreationInputTokens: result.Usage.CacheCreationInputTokens,
			CacheReadInputTokens:     result.Usage.CacheReadInputTokens,
			CacheCreation5mTokens:    result.Usage.CacheCreation5mTokens,
			CacheCreation1hTokens:    result.Usage.CacheCreation1hTokens,
		},
		Stream: result.Stream, Duration: result.Duration, FirstTokenMs: result.FirstTokenMs, ClientDisconnect: result.ClientDisconnect,
		ReasoningEffort: result.ReasoningEffort, RequestedReasoningEffort: result.RequestedReasoningEffort,
	}, err
}
