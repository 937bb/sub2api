package kiro

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const maxToolNameBytes = 63

// Keep short names unchanged and make long MCP names stable across requests.
// Avoid splitting UTF-8 and reject alias collisions in prepareTools.
func upstreamToolName(name string) string {
	if len(name) <= maxToolNameBytes {
		return name
	}
	sum := sha256.Sum256([]byte(name))
	prefix := name[:54]
	for !utf8.ValidString(prefix) {
		prefix = prefix[:len(prefix)-1]
	}
	return prefix + "_" + hex.EncodeToString(sum[:4])
}

func objectJSON(raw []byte) bool {
	raw = []byte(strings.TrimSpace(string(raw)))
	return len(raw) > 0 && raw[0] == '{' && json.Valid(raw)
}

// Normalize nullable MCP schema fields without erasing constraints or nested schemas.
func toolSchema(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		raw = json.RawMessage(`{"type":"object"}`)
	}
	if !objectJSON(raw) {
		return nil, errors.New("tool input_schema must be an object")
	}
	var schema map[string]json.RawMessage
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, err
	}
	if v := schema["type"]; len(v) == 0 || string(v) == "null" {
		schema["type"] = json.RawMessage(`"object"`)
	}
	if string(schema["type"]) != `"object"` {
		return nil, errors.New("tool input_schema must describe an object")
	}
	if v := schema["properties"]; len(v) == 0 || string(v) == "null" {
		schema["properties"] = json.RawMessage(`{}`)
	} else if !objectJSON(v) {
		return nil, errors.New("tool schema properties must be an object")
	}
	if string(schema["required"]) == "null" {
		delete(schema, "required")
	}
	if v, ok := schema["required"]; ok {
		var required []string
		if len(v) == 0 || v[0] != '[' || json.Unmarshal(v, &required) != nil {
			return nil, errors.New("tool schema required must be a string array")
		}
	}
	if string(schema["additionalProperties"]) == "null" {
		delete(schema, "additionalProperties")
	}
	return json.Marshal(schema)
}

// Merge consecutive same-role blocks before pairing. Some clients send separate
// user messages for parallel results. Never drop unmatched tools or their results.
func normalizeToolHistory(messages []Message) ([]Message, map[string]bool, error) {
	merged := make([]Message, 0, len(messages))
	for _, m := range messages {
		blocks, err := Blocks(m.Content)
		if err != nil {
			return nil, nil, err
		}
		if len(merged) > 0 && merged[len(merged)-1].Role == m.Role {
			prev, err := Blocks(merged[len(merged)-1].Content)
			if err != nil {
				return nil, nil, err
			}
			merged[len(merged)-1].Content, err = json.Marshal(append(prev, blocks...))
			if err != nil {
				return nil, nil, err
			}
		} else {
			merged = append(merged, m)
		}
	}
	pending, seen, names := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, m := range merged {
		blocks, err := Blocks(m.Content)
		if err != nil {
			return nil, nil, err
		}
		if m.Role == "assistant" && len(pending) != 0 {
			return nil, nil, errors.New("tool_use is missing its tool_result before the next assistant turn")
		}
		for _, b := range blocks {
			switch b.Type {
			case "tool_use":
				if m.Role != "assistant" || b.ID == "" || b.Name == "" || !objectJSON(b.Input) {
					return nil, nil, errors.New("invalid tool_use block: expected assistant, id, name and object input")
				}
				if seen[b.ID] {
					return nil, nil, errors.New("duplicate tool_use id")
				}
				pending[b.ID], seen[b.ID], names[b.Name] = true, true, true
			case "tool_result":
				if m.Role != "user" || b.ToolUseID == "" || !pending[b.ToolUseID] {
					return nil, nil, errors.New("tool_result has no pending tool_use or is duplicated")
				}
				delete(pending, b.ToolUseID)
			}
		}
		if m.Role == "user" && len(pending) != 0 {
			return nil, nil, errors.New("tool results must include every pending tool_use")
		}
	}
	if len(pending) != 0 {
		return nil, nil, errors.New("tool_use is missing its tool_result")
	}
	return merged, names, nil
}

func prepareTools(req *Request) error {
	var err error
	var historical map[string]bool
	req.Messages, historical, err = normalizeToolHistory(req.Messages)
	if err != nil {
		return err
	}
	choice := "auto"
	if len(req.ToolChoice) > 0 && string(req.ToolChoice) != "null" {
		var tc struct {
			Type            string `json:"type"`
			DisableParallel bool   `json:"disable_parallel_tool_use"`
		}
		if err := json.Unmarshal(req.ToolChoice, &tc); err != nil {
			return errors.New("invalid tool_choice")
		}
		choice = tc.Type
		if choice != "auto" && choice != "none" {
			return errors.New("Kiro cannot enforce forced tool choice; use auto or none")
		}
		if tc.DisableParallel {
			return errors.New("Kiro cannot enforce disable_parallel_tool_use")
		}
	}
	req.ToolNames = map[string]string{}
	aliases, seen := map[string]string{}, map[string]bool{}
	historyNames := map[string]bool{}
	for name := range historical {
		historyNames[name] = true
	}
	for i := range req.Tools {
		t := &req.Tools[i]
		if t.Type != "" && t.Type != "custom" {
			return fmt.Errorf("unsupported Kiro server tool %s", t.Type)
		}
		if strings.TrimSpace(t.Name) == "" || seen[t.Name] {
			return errors.New("tool names must be nonempty and unique")
		}
		seen[t.Name] = true
		alias := upstreamToolName(t.Name)
		if prev, ok := aliases[alias]; ok && prev != t.Name {
			return errors.New("tool name alias collision")
		}
		aliases[alias], req.ToolNames[alias] = t.Name, t.Name
		t.InputSchema, err = toolSchema(t.InputSchema)
		if err != nil {
			return fmt.Errorf("tool input_schema: %w", err)
		}
		if t.Description == "" {
			t.Description = t.Name
		}
		if runes := []rune(t.Description); len(runes) > 10000 {
			t.Description = string(runes[:10000])
		}
	}
	// Historical definitions are required by Kiro even if the client has since
	// removed the tool. They are deliberately excluded from the response allowlist.
	for _, m := range req.Messages {
		blocks, _ := Blocks(m.Content)
		for _, b := range blocks {
			if b.Type != "tool_use" || !historical[b.Name] {
				continue
			}
			delete(historical, b.Name)
			alias := upstreamToolName(b.Name)
			if prev, ok := aliases[alias]; ok && prev != b.Name {
				return errors.New("historical tool name alias collision")
			}
			aliases[alias] = b.Name
			if !seen[b.Name] {
				req.Tools = append(req.Tools, Tool{Name: b.Name, Description: "Historical tool, unavailable for new calls. Do not call it.", InputSchema: json.RawMessage(`{"type":"object","properties":{}}`)})
			}
		}
	}
	if choice == "none" {
		// The output allowlist also prevents historical definitions from enabling calls.
		req.ToolNames = map[string]string{}
		kept := req.Tools[:0]
		for _, t := range req.Tools {
			if historyNames[t.Name] {
				t.Description = "Historical tool, unavailable for new calls. Do not call it."
				kept = append(kept, t)
			}
		}
		req.Tools = kept
	}
	return nil
}

func convertImage(b Block) (map[string]any, error) {
	if b.Source == nil || b.Source.Type != "base64" || b.Source.Data == "" {
		return nil, errors.New("Kiro requires nonempty base64 images")
	}
	format := strings.TrimPrefix(b.Source.MediaType, "image/")
	switch format {
	case "png", "jpeg", "gif", "webp":
	default:
		return nil, errors.New("unsupported Kiro image format")
	}
	return map[string]any{"format": format, "source": map[string]any{"bytes": b.Source.Data}}, nil
}

// Kiro toolResults carry text; images are attached to the containing user message.
func toolResultContent(raw json.RawMessage) (string, []any, error) {
	blocks, err := Blocks(raw)
	if err != nil {
		return "", nil, err
	}
	texts, images := []string{}, []any{}
	for _, b := range blocks {
		switch b.Type {
		case "text":
			texts = append(texts, b.Text)
		case "image":
			image, err := convertImage(b)
			if err != nil {
				return "", nil, err
			}
			images = append(images, image)
		default:
			return "", nil, fmt.Errorf("unsupported tool_result content block: %s", b.Type)
		}
	}
	return strings.Join(texts, "\n"), images, nil
}

// Anthropic/OpenAI bridges expect sequential content blocks. Keep the first
// block live and queue interleaved later blocks until the preceding one closes.
// Decode already bounds the total upstream response size.
func serialToolEmitter(emit Emit) Emit {
	if emit == nil {
		return nil
	}
	type event struct {
		name string
		data any
	}
	pending := map[int][]event{}
	next := 0
	return func(name string, data any) error {
		if name != "content_block_start" && name != "content_block_delta" && name != "content_block_stop" {
			return emit(name, data)
		}
		fields, ok := data.(map[string]any)
		if !ok {
			return errors.New("invalid Kiro block event")
		}
		index, ok := fields["index"].(int)
		if !ok || index < next {
			return errors.New("invalid Kiro block order")
		}
		pending[index] = append(pending[index], event{name, data})
		for {
			events := pending[next]
			if len(events) == 0 {
				return nil
			}
			delete(pending, next)
			for _, e := range events {
				if err := emit(e.name, e.data); err != nil {
					return err
				}
				if e.name == "content_block_stop" {
					next++
				}
			}
		}
	}
}
