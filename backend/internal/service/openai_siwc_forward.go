package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
	"github.com/Wei-Shaw/sub2api/internal/pkg/requestmodel"
	"github.com/Wei-Shaw/sub2api/internal/pkg/siwc"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

func (a *Account) supportsSIWCUpstreamModel(model string) bool {
	if model == "" {
		return false
	}
	switch models := a.Credentials["siwc_models"].(type) {
	case []string:
		for _, listed := range models {
			if listed == model {
				return true
			}
		}
	case []any:
		for _, listed := range models {
			if listed == model {
				return true
			}
		}
	}
	return false
}

// Use the same standard transport profile as SIWC credential acquisition. The
// caller retains request admission, RPM, accounting and response streaming.
func (s *OpenAIGatewayService) doSIWCHTTP(request *http.Request, proxyURL string) (*http.Response, error) {
	if request == nil || request.URL == nil || request.Method != http.MethodPost || request.URL.String() != siwc.ResponsesURL {
		return nil, errors.New("unsupported SIWC inference endpoint")
	}
	client, err := httpclient.GetClient(httpclient.Options{ProxyURL: proxyURL})
	if err != nil {
		return nil, err
	}
	isolated := *client
	if s.siwcTransport != nil {
		isolated.Transport = s.siwcTransport
	}
	isolated.Timeout = 0
	isolated.Jar = nil
	isolated.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return isolated.Do(request)
}

// Keep the selected model and caller instructions; adapt only the wire contract.
func normalizeSIWCResponsesBody(body []byte) ([]byte, error) {
	if err := requestmodel.ValidateJSONSelectors(body); err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := decodeOpenAIJSONUseNumber(body, &payload); err != nil || payload == nil {
		return nil, errors.New("SIWC requires a JSON object")
	}
	if value, exists := payload["previous_response_id"]; exists && value != nil && value != "" {
		return nil, errors.New("SIWC requires complete input history; previous_response_id is unsupported")
	}
	if value, exists := payload["conversation"]; exists && value != nil && value != "" {
		return nil, errors.New("SIWC requires complete input history; conversation is unsupported")
	}
	if text, ok := payload["input"].(string); ok {
		payload["input"] = []any{map[string]any{"role": "user", "content": text}}
	}
	items, ok := payload["input"].([]any)
	if !ok || len(items) == 0 {
		return nil, errors.New("SIWC requires nonempty input")
	}
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			return nil, errors.New("invalid SIWC input item")
		}
		if item["type"] == "item_reference" || item["type"] == "compaction_trigger" {
			return nil, errors.New("SIWC requires complete stateless history")
		}
		if item["role"] == "system" {
			item["role"] = "developer"
		}
	}
	if raw, exists := payload["tools"]; exists && raw != nil {
		tools, ok := raw.([]any)
		if !ok {
			return nil, errors.New("SIWC tools must be an array")
		}
		additional, top := []any{}, []any{}
		for _, raw := range tools {
			tool, ok := raw.(map[string]any)
			if !ok {
				return nil, errors.New("invalid SIWC tool")
			}
			switch tool["type"] {
			case "function", "custom":
				additional = append(additional, tool)
			case "namespace":
				nested, ok := tool["tools"].([]any)
				if !ok {
					return nil, errors.New("invalid SIWC namespace tools")
				}
				for _, rawChild := range nested {
					child, ok := rawChild.(map[string]any)
					if !ok || (child["type"] != "function" && child["type"] != "custom") {
						return nil, errors.New("unsupported SIWC namespace tool")
					}
				}
				top = append(top, tool)
			case "web_search", "web_search_preview":
				top = append(top, tool)
			default:
				return nil, errors.New("unsupported SIWC hosted tool")
			}
		}
		if len(additional) > 0 {
			items = append(items, map[string]any{"type": "additional_tools", "tools": additional})
		}
		delete(payload, "tools")
		if len(top) > 0 {
			payload["tools"] = top
		}
	}
	payload["input"], payload["store"], payload["stream"] = items, false, true
	for _, key := range []string{"background", "conversation", "max_output_tokens", "max_completion_tokens", "max_tool_calls", "metadata", "moderation", "multi_agent", "prompt", "prompt_cache_retention", "safety_identifier", "temperature", "top_logprobs", "top_p", "truncation", "user", "previous_response_id", "stream_options"} {
		delete(payload, key)
	}
	return marshalOpenAIUpstreamJSON(payload)
}

func buildSIWCResponsesRequest(ctx context.Context, c *gin.Context, account *Account, body []byte, token string) (*http.Request, error) {
	if !account.IsOpenAISiwc() {
		return nil, errors.New("SIWC account authorization required")
	}
	if !siwc.HasSharingScopes(account.GetCredential("granted_scope")) || strings.TrimSpace(token) == "" {
		return nil, errors.New("SIWC token-sharing authorization missing")
	}
	if isOpenAIResponsesCompactPath(c) {
		return nil, errors.New("SIWC compact endpoint is unsupported; send full history to /v1/responses")
	}
	normalized, err := normalizeSIWCResponsesBody(body)
	if err != nil {
		return nil, err
	}
	if !account.supportsSIWCUpstreamModel(gjson.GetBytes(normalized, "model").String()) {
		return nil, errors.New("model is not in the SIWC account catalog")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, siwc.ResponsesURL, bytes.NewReader(normalized))
	if err != nil {
		return nil, fmt.Errorf("build SIWC response request: %w", err)
	}
	// Never copy inbound credentials, cookies or Codex-specific identity headers.
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("User-Agent", siwc.UserAgent)
	return req, nil
}
