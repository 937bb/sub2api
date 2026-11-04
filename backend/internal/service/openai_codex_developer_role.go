package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	openaiwsv2 "github.com/Wei-Shaw/sub2api/internal/service/openai_ws_v2"
	coderws "github.com/coder/websocket"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Only the selected Codex destination opts into this policy. Client history,
// API-key providers, and non-OpenAI OAuth providers retain their original roles.
func usesCodexDeveloperMessages(account *Account) bool {
	return account != nil && (account.IsOpenAIOAuthLike() ||
		(account.Platform == "" && account.Type == AccountTypeOAuth))
}

func codexDeveloperTextContent(content gjson.Result) bool {
	if content.Type == gjson.String {
		return true
	}
	if !content.IsArray() || len(content.Array()) == 0 {
		return false
	}
	for _, part := range content.Array() {
		kind := part.Get("type").String()
		if !part.IsObject() || (kind != "input_text" && kind != "text") || part.Get("text").Type != gjson.String {
			return false
		}
	}
	return true
}

// normalizeCodexDeveloperMessages runs after protocol translation. It changes
// only text message envelopes: tool outputs, assistant history, media, encrypted
// reasoning, IDs, and arbitrary JSON inside text/tool arguments are preserved.
// Surgical edits retain unknown fields and integer precision without mutating
// the inbound bytes needed by retries on a different account.
func normalizeCodexDeveloperMessages(body []byte, account *Account) ([]byte, bool, error) {
	if !usesCodexDeveloperMessages(account) || len(body) == 0 {
		return body, false, nil
	}
	root := gjson.ParseBytes(body)
	if !root.IsObject() || !gjson.ValidBytes(body) {
		return body, false, fmt.Errorf("normalize Codex developer messages: invalid JSON object")
	}
	if event := root.Get("type"); event.Exists() && event.String() != "response.create" {
		return body, false, nil
	}
	input := root.Get("input")
	if input.Type == gjson.String && strings.TrimSpace(input.String()) != "" {
		next, err := sjson.SetRawBytes(body, "input", []byte(`[{"type":"message","role":"developer","content":`+input.Raw+`}]`))
		return next, err == nil, err
	}
	if !input.IsArray() {
		return body, false, nil
	}
	next := body
	changed := false
	for i, item := range input.Array() {
		kind := item.Get("type")
		if !item.IsObject() || item.Get("role").String() != "user" ||
			(kind.Exists() && kind.String() != "message") ||
			item.Get("tool_call_id").Exists() || item.Get("tool_calls").Exists() ||
			!codexDeveloperTextContent(item.Get("content")) {
			continue
		}
		var err error
		next, err = sjson.SetBytes(next, fmt.Sprintf("input.%d.role", i), "developer")
		if err != nil {
			return body, false, fmt.Errorf("normalize Codex developer message role: %w", err)
		}
		changed = true
	}
	return next, changed, nil
}

// The pooled HTTP-to-WS path holds decoded messages. Clone changed envelopes
// so connection prewarm and retries never rewrite the caller's shared history.
func normalizeCodexDeveloperMessageMap(body map[string]any, account *Account) {
	if !usesCodexDeveloperMessages(account) {
		return
	}
	if input, ok := body["input"].(string); ok {
		if strings.TrimSpace(input) != "" {
			body["input"] = []any{map[string]any{"type": "message", "role": "developer", "content": input}}
		}
		return
	}
	input, ok := body["input"].([]any)
	if !ok {
		return
	}
	var next []any
	for i, item := range input {
		message, ok := item.(map[string]any)
		if !ok || message["role"] != "user" {
			continue
		}
		if kind, exists := message["type"]; exists && kind != "message" {
			continue
		}
		if _, exists := message["tool_call_id"]; exists {
			continue
		}
		if _, exists := message["tool_calls"]; exists {
			continue
		}
		content, err := json.Marshal(message["content"])
		if err != nil || !codexDeveloperTextContent(gjson.ParseBytes(content)) {
			continue
		}
		if next == nil {
			next = append([]any(nil), input...)
		}
		clone := make(map[string]any, len(message))
		for key, value := range message {
			clone[key] = value
		}
		clone["role"] = "developer"
		next[i] = clone
	}
	if next != nil {
		body["input"] = next
	}
}

// Convert only at the upstream write boundary so policy evaluation, session
// identity, and replay history continue to see the client's original roles.
type codexDeveloperFrameConn struct {
	openaiwsv2.FrameConn
	account *Account
}

func (c *codexDeveloperFrameConn) WriteFrame(ctx context.Context, msgType coderws.MessageType, payload []byte) error {
	if msgType == coderws.MessageText || msgType == coderws.MessageBinary {
		outbound, _, err := normalizeCodexDeveloperMessages(payload, c.account)
		if err != nil {
			return err
		}
		payload = outbound
	}
	return c.FrameConn.WriteFrame(ctx, msgType, payload)
}
