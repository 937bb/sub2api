package kiro

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

type Message struct {
	Role         string          `json:"role"`
	Content      json.RawMessage `json:"content"`
	OutputConfig json.RawMessage `json:"output_config,omitempty"`
}
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
	Type        string          `json:"type"`
}
type Request struct {
	ToolNames     map[string]string `json:"-"`
	Model         string            `json:"model"`
	MaxTokens     int               `json:"max_tokens"`
	Stream        bool              `json:"stream"`
	System        json.RawMessage   `json:"system"`
	Messages      []Message         `json:"messages"`
	Tools         []Tool            `json:"tools"`
	ToolChoice    json.RawMessage   `json:"tool_choice"`
	Thinking      json.RawMessage   `json:"thinking"`
	Temperature   *float64          `json:"temperature"`
	StopSequences []string          `json:"stop_sequences"`
	OutputConfig  json.RawMessage   `json:"output_config"`
}
type Block struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
	Source    *struct {
		Type      string `json:"type"`
		MediaType string `json:"media_type"`
		Data      string `json:"data"`
	} `json:"source,omitempty"`
}

func Blocks(raw json.RawMessage) ([]Block, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return []Block{{Type: "text", Text: text}}, nil
	}
	var blocks []Block
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, errors.New("invalid message content")
	}
	return blocks, nil
}

var datedModel = regexp.MustCompile(`-\d{8}$`)
var modelVersion = regexp.MustCompile(`-(\d+)-(\d+)$`)

func ModelID(model string) string {
	model = strings.TrimPrefix(model, "kiro/")
	model = strings.TrimSuffix(model, "[1m]")
	model = datedModel.ReplaceAllString(model, "")
	return modelVersion.ReplaceAllString(model, "-$1.$2")
}

// StableID includes the authenticated tenant and selected account in its namespace.
func StableID(scope, session string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("kiro:v1:"+scope+":"+session)).String()
}
func textContent(raw json.RawMessage) (string, error) {
	blocks, err := Blocks(raw)
	if err != nil {
		return "", err
	}
	parts := []string{}
	for _, b := range blocks {
		if b.Type != "text" {
			return "", fmt.Errorf("unsupported content block: %s", b.Type)
		}
		parts = append(parts, b.Text)
	}
	return strings.Join(parts, "\n"), nil
}

// Convert preserves full message history. Account changes get a new upstream session.
func Convert(raw []byte, model, session string, credentials map[string]any) ([]byte, *Request, error) {
	var req Request
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, nil, errors.New("invalid Kiro request JSON")
	}
	if len(req.Messages) == 0 {
		return nil, nil, errors.New("messages must not be empty")
	}
	if req.MaxTokens <= 0 {
		return nil, nil, errors.New("max_tokens must be positive")
	}
	if len(req.StopSequences) > 0 {
		return nil, nil, errors.New("Kiro does not support stop_sequences")
	}
	if err := normalizeControlMessages(&req); err != nil {
		return nil, nil, err
	}
	if err := prepareTools(&req); err != nil {
		return nil, nil, err
	}
	system, err := textContent(req.System)
	if err != nil {
		return nil, nil, fmt.Errorf("system: %w", err)
	}
	tools := make([]any, 0, len(req.Tools))
	for _, t := range req.Tools {
		tools = append(tools, map[string]any{"toolSpecification": map[string]any{"name": upstreamToolName(t.Name), "description": t.Description, "inputSchema": map[string]any{"json": t.InputSchema}}})
	}

	history := make([]any, 0, len(req.Messages))
	var current map[string]any
	for i, m := range req.Messages {
		blocks, err := Blocks(m.Content)
		if err != nil {
			return nil, nil, err
		}
		texts := []string{}
		images := []any{}
		results := []any{}
		uses := []any{}
		for _, b := range blocks {
			switch b.Type {
			case "text":
				texts = append(texts, b.Text)
			case "image":
				if m.Role != "user" {
					return nil, nil, errors.New("Kiro images require a user message")
				}
				image, err := convertImage(b)
				if err != nil {
					return nil, nil, err
				}
				images = append(images, image)
			case "tool_use":
				if m.Role != "assistant" || b.ID == "" || b.Name == "" || !json.Valid(b.Input) {
					return nil, nil, errors.New("invalid tool_use block")
				}
				uses = append(uses, map[string]any{"toolUseId": b.ID, "name": upstreamToolName(b.Name), "input": b.Input})
			case "tool_result":
				if m.Role != "user" || b.ToolUseID == "" {
					return nil, nil, errors.New("invalid tool_result block")
				}
				text, resultImages, err := toolResultContent(b.Content)
				if err != nil {
					return nil, nil, fmt.Errorf("tool_result: %w", err)
				}
				images = append(images, resultImages...)
				status := "success"
				if b.IsError {
					status = "error"
				}
				results = append(results, map[string]any{"toolUseId": b.ToolUseID, "status": status, "content": []any{map[string]any{"text": text}}})
			case "thinking", "redacted_thinking":
				// Kiro has no Anthropic signature continuation contract; preserve visible history only.
			default:
				return nil, nil, fmt.Errorf("unsupported Kiro content block %s", b.Type)
			}
		}
		content := strings.Join(texts, "\n")
		switch m.Role {
		case "user":
			if i == 0 && system != "" {
				content = system + "\n\n" + content
			}
			msg := map[string]any{"content": content, "modelId": ModelID(model), "origin": "AI_EDITOR"}
			context := map[string]any{}
			if len(results) > 0 {
				context["toolResults"] = results
			}
			if i == len(req.Messages)-1 && len(tools) > 0 {
				context["tools"] = tools
			}
			msg["userInputMessageContext"] = context
			if len(images) > 0 {
				msg["images"] = images
			}
			if i == len(req.Messages)-1 {
				current = map[string]any{"userInputMessage": msg}
			} else {
				history = append(history, map[string]any{"userInputMessage": msg})
			}
		case "assistant":
			if i == len(req.Messages)-1 {
				return nil, nil, errors.New("Kiro requires a final user message; assistant prefill is unsupported")
			}
			msg := map[string]any{"content": content}
			if len(uses) > 0 {
				msg["toolUses"] = uses
			}
			history = append(history, map[string]any{"assistantResponseMessage": msg})
		default:
			return nil, nil, errors.New("Kiro messages support user and assistant roles")
		}
	}
	body := map[string]any{"conversationState": map[string]any{"conversationId": session, "agentContinuationId": session, "agentTaskType": "vibe", "chatTriggerType": "MANUAL", "currentMessage": current, "history": history}}
	if arn := String(credentials, "profile_arn"); arn != "" && String(credentials, "auth_method") != "api_key" {
		body["profileArn"] = arn
	}
	fields := map[string]any{}
	// Haiku and legacy Kiro models reject the entire additional-fields object.
	// Forward the token limit only when the account catalog advertises support.
	schema, _ := credentials["kiro_request_fields"].(map[string]any)
	if limit, ok := schema["max_tokens"]; ok {
		fields["max_tokens"] = catalogTokenLimit(req.MaxTokens, limit)
	}
	if len(req.Thinking) > 0 && string(req.Thinking) != "null" {
		if _, ok := schema["thinking"]; !ok {
			return nil, nil, errors.New("selected Kiro model does not support thinking configuration")
		}
		thinking, err := normalizeThinkingDisplay(req.Thinking)
		if err != nil {
			return nil, nil, err
		}
		fields["thinking"] = thinking
	}
	if len(req.OutputConfig) > 0 && string(req.OutputConfig) != "null" {
		if _, ok := schema["output_config"]; !ok {
			return nil, nil, errors.New("selected Kiro model does not support output_config")
		}
		outputConfig, err := normalizeOutputEffort(req.OutputConfig, schema["output_config"])
		if err != nil {
			return nil, nil, err
		}
		fields["output_config"] = outputConfig
	}
	if req.Temperature != nil {
		if _, ok := schema["temperature"]; !ok {
			return nil, nil, errors.New("selected Kiro model does not support temperature")
		}
		fields["temperature"] = *req.Temperature
	}
	if len(fields) > 0 {
		body["additionalModelRequestFields"] = fields
	}
	applyEndpointBody(body, credentials)
	encoded, err := json.Marshal(body)
	return encoded, &req, err
}

// CacheKey permits explicitly opted-in full-response reuse for text-only requests; tools, thinking and multimodal requests bypass.
func CacheKey(raw []byte, scope string) (string, bool) {
	var req Request
	if json.Unmarshal(raw, &req) != nil || len(req.Tools) > 0 || (len(req.Thinking) > 0 && strings.TrimSpace(string(req.Thinking)) != "null") || (req.Temperature != nil && *req.Temperature != 0) {
		return "", false
	}
	if _, err := textContent(req.System); err != nil {
		return "", false
	}
	for _, m := range req.Messages {
		if _, err := textContent(m.Content); err != nil {
			return "", false
		}
	}
	var obj map[string]any
	if json.Unmarshal(raw, &obj) != nil {
		return "", false
	}
	delete(obj, "stream")
	canonical, err := json.Marshal(obj)
	if err != nil {
		return "", false
	}
	hash := sha256.Sum256(append([]byte(scope+":"), canonical...))
	return hex.EncodeToString(hash[:]), true
}
