package service

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
)

const openAIGroupModelMappingContextKey = "openai_group_model_mapping"

type openAIGroupModelMappingContext struct {
	RequestedModel string
	UpstreamModel  string
}

// NormalizeOpenAIGroupModelMapping validates and canonicalizes administrator
// supplied group mappings. Only a trailing wildcard is supported.
func NormalizeOpenAIGroupModelMapping(platform string, mapping map[string]string) (map[string]string, error) {
	if platform != PlatformOpenAI || len(mapping) == 0 {
		return map[string]string{}, nil
	}

	out := make(map[string]string, len(mapping))
	for source, target := range mapping {
		source = strings.TrimSpace(source)
		target = strings.TrimSpace(target)
		if source == "" || target == "" {
			return nil, fmt.Errorf("model mapping source and target must not be empty")
		}
		if wildcard := strings.IndexByte(source, '*'); wildcard >= 0 && wildcard != len(source)-1 {
			return nil, fmt.Errorf("model mapping wildcard must be the final character: %s", source)
		}
		if strings.Count(source, "*") > 1 {
			return nil, fmt.Errorf("model mapping supports only one trailing wildcard: %s", source)
		}
		out[source] = target
	}
	return out, nil
}

// CloneOpenAIModelMapping returns an independent copy suitable for DTOs,
// snapshots, and duplicated groups.
func CloneOpenAIModelMapping(mapping map[string]string) map[string]string {
	if len(mapping) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(mapping))
	for source, target := range mapping {
		out[source] = target
	}
	return out
}

// ResolveOpenAIModelMapping applies an exact rule first, then the longest
// matching prefix wildcard. A matched target is a final upstream model ID.
func (g *Group) ResolveOpenAIModelMapping(requestedModel string) (string, bool) {
	if g == nil || g.Platform != PlatformOpenAI || requestedModel == "" {
		return requestedModel, false
	}
	if target, ok := g.OpenAIModelMapping[requestedModel]; ok {
		if target = strings.TrimSpace(target); target != "" {
			return target, true
		}
	}

	bestPrefixLen := -1
	bestTarget := ""
	for pattern, target := range g.OpenAIModelMapping {
		if !strings.HasSuffix(pattern, "*") {
			continue
		}
		prefix := strings.TrimSuffix(pattern, "*")
		if len(prefix) <= bestPrefixLen || !strings.HasPrefix(requestedModel, prefix) {
			continue
		}
		if target = strings.TrimSpace(target); target == "" {
			continue
		}
		bestPrefixLen = len(prefix)
		bestTarget = target
	}
	if bestPrefixLen >= 0 {
		return bestTarget, true
	}
	return requestedModel, false
}

func MarkOpenAIGroupModelMapping(c *gin.Context, requestedModel, upstreamModel string) {
	if c == nil {
		return
	}
	upstreamModel = strings.TrimSpace(upstreamModel)
	if upstreamModel == "" {
		return
	}
	c.Set(openAIGroupModelMappingContextKey, openAIGroupModelMappingContext{
		RequestedModel: strings.TrimSpace(requestedModel),
		UpstreamModel:  upstreamModel,
	})
}

func openAIGroupMappedModel(c *gin.Context) (string, bool) {
	if c == nil {
		return "", false
	}
	value, ok := c.Get(openAIGroupModelMappingContextKey)
	if !ok {
		return "", false
	}
	mapping, ok := value.(openAIGroupModelMappingContext)
	if !ok || strings.TrimSpace(mapping.UpstreamModel) == "" {
		return "", false
	}
	return strings.TrimSpace(mapping.UpstreamModel), true
}

func resolveOpenAIModelForUpstream(c *gin.Context, account *Account, candidate string) string {
	if target, matched := openAIGroupMappedModel(c); matched {
		return target
	}
	return normalizeOpenAIModelForUpstream(account, candidate)
}
