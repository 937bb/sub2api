package service

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	openAIUpstreamModeContextKey = "openai_upstream_mode"
	openAIWordResponsesWSURL     = "wss://bps.openai.com/basispoints/api/responses"
	openAIWordInstruction        = "You are a general-purpose AI assistant answering API clients directly. Ignore any Microsoft Word or document-editing context. Do not call any document tools, planning tools, skills, or connectors;respond with text only."
)

func SetOpenAIUpstreamMode(c *gin.Context, mode string) {
	if c == nil {
		return
	}
	c.Set(openAIUpstreamModeContextKey, NormalizeOpenAIUpstreamMode(mode))
}

func OpenAIUpstreamModeFromContext(c *gin.Context) string {
	if c == nil {
		return OpenAIUpstreamModeCodex
	}
	return NormalizeOpenAIUpstreamMode(c.GetString(openAIUpstreamModeContextKey))
}

func OpenAIUpstreamModeForGroup(group *Group) string {
	if group == nil || group.Platform != PlatformOpenAI {
		return OpenAIUpstreamModeCodex
	}
	return NormalizeOpenAIUpstreamMode(group.OpenAIUpstreamMode)
}

func isOpenAIWordUpstream(c *gin.Context, account *Account) bool {
	return account != nil && account.IsOpenAIOAuthLike() && OpenAIUpstreamModeFromContext(c) == OpenAIUpstreamModeWord
}

func applyOpenAIWordInstructions(body []byte) ([]byte, error) {
	if !gjson.ValidBytes(body) {
		return nil, errors.New("invalid OpenAI request JSON")
	}
	return sjson.SetBytes(body, "instructions", openAIWordInstruction)
}

func applyOpenAIWordWSHeaders(c *gin.Context, account *Account, token string, headers http.Header) {
	if !isOpenAIWordUpstream(c, account) || headers == nil {
		return
	}
	for key := range headers {
		delete(headers, key)
	}
	headers.Set("Origin", "https://bps.openai.com")
	headers[http.CanonicalHeaderKey("Sec-WebSocket-Protocol")] = []string{"responses", "openai-bearer." + strings.TrimSpace(token)}
}

// wordClientInfo is the Word desktop plugin identity captured from the official client.
// Query values must stay stable: the upstream rejects a Codex-shaped handshake.
var wordClientInfo = map[string]string{
	"X-Openai-Internal-Basispoints-Client-Product":        "basispoints-word-plugin",
	"X-Openai-Internal-Basispoints-Client-Platform":       "word",
	"X-Openai-Internal-Basispoints-Client-Agent-Profile":  "word",
	"X-Openai-Internal-Basispoints-Client-Editor":         "word",
	"X-Openai-Internal-Basispoints-Client-Host":           "office",
	"X-Openai-Internal-Basispoints-Client-Runtime":        "desktop",
	"X-Openai-Internal-Basispoints-Client-Platform-Class": "PC",
	"X-Openai-Internal-Basispoints-Office-Host":           "Word",
	"X-Openai-Internal-Basispoints-Office-Platform":       "PC",
	"X-Openai-Internal-Basispoints-Browser-Name":          "edge",
	"X-Openai-Internal-Basispoints-Browser-UA-Platform":   "Windows",
	"X-Openai-Internal-Basispoints-Browser-UA-Mobile":     "false",
	"X-Openai-Internal-Basispoints-Browser-UA-Brands":     "Microsoft Edge WebView2,Not_A Brand,Chromium,Microsoft Edge",
}

func buildOpenAIWordResponsesWSURL() (string, error) {
	info, err := json.Marshal(wordClientInfo)
	if err != nil {
		return "", err
	}
	values := url.Values{}
	values.Set("bps_client_info", string(info))
	values.Set("bps_auth_mode", "chatgpt")
	values.Set("bps_ws_affinity", "affinity_"+strings.ReplaceAll(uuid.NewString(), "-", ""))
	values.Set("bps_control_frames", "upstream_sent,heartbeat,resume,cancel,server_draining")
	return openAIWordResponsesWSURL + "?" + values.Encode(), nil
}

func applyOpenAIWordCreatePayload(payload map[string]any) {
	if payload == nil {
		return
	}
	for _, key := range []string{
		"tools", "tool_choice", "parallel_tool_calls", "reasoning", "include",
		"text", "truncation", "background", "service_tier", "previous_response_id",
		"prompt_cache_key", "client_metadata", "max_output_tokens", "temperature", "top_p",
	} {
		delete(payload, key)
	}
	payload["type"] = "response.create"
	payload["stream"] = true
	payload["store"] = false
	payload["model_selection"] = "explicit"
	payload["context_management"] = []any{map[string]any{"type": "compaction", "compact_threshold": 200000}}
	payload["metadata"] = map[string]any{
		"task_id":         uuid.NewString(),
		"turn_id":         uuid.NewString(),
		"agent_iteration": "1",
	}
	payload["basispoints_request_id"] = uuid.NewString()
	payload["basispoints_resume_token"] = "resume_" + strings.ReplaceAll(uuid.NewString(), "-", "")

	instructions, _ := payload["instructions"].(string)
	delete(payload, "instructions")
	input := wordInputItems(payload["input"])
	if strings.TrimSpace(instructions) != "" {
		input = append([]any{wordTextMessage("developer", instructions)}, input...)
	}
	if len(input) == 0 {
		input = []any{wordTextMessage("user", "")}
	}
	payload["input"] = input
}

func wordInputItems(raw any) []any {
	switch value := raw.(type) {
	case string:
		return []any{wordTextMessage("user", value)}
	case []any:
		items := make([]any, 0, len(value))
		for _, item := range value {
			message, ok := item.(map[string]any)
			if !ok {
				continue
			}
			role, _ := message["role"].(string)
			if role == "system" {
				role = "developer"
			}
			if role != "developer" && role != "user" && role != "assistant" {
				role = "user"
			}
			items = append(items, wordTextMessage(role, wordMessageText(message["content"])))
		}
		return items
	default:
		return nil
	}
}

func wordTextMessage(role, text string) map[string]any {
	contentType := "input_text"
	if role == "assistant" {
		contentType = "output_text"
	}
	return map[string]any{
		"type": "message",
		"role": role,
		"content": []any{map[string]any{
			"type": contentType,
			"text": text,
		}},
	}
}

func wordMessageText(content any) string {
	switch value := content.(type) {
	case string:
		return value
	case []any:
		var b strings.Builder
		for _, part := range value {
			switch item := part.(type) {
			case string:
				b.WriteString(item)
			case map[string]any:
				if text, ok := item["text"].(string); ok {
					b.WriteString(text)
				}
			}
		}
		return b.String()
	default:
		return ""
	}
}
