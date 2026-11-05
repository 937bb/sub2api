package kiro

import (
	"encoding/json"
	"fmt"
	"strings"
)

// GeminiRequest converts the supported generateContent surface to Messages.
// Reject controls that cannot be honored instead of silently changing semantics.
func GeminiRequest(raw []byte, model string) ([]byte, error) {
	var in map[string]json.RawMessage
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, err
	}
	for key := range in {
		switch key {
		case "contents", "systemInstruction", "tools", "toolConfig", "generationConfig":
		default:
			return nil, fmt.Errorf("Kiro does not support Gemini field %s", key)
		}
	}
	out := Request{Model: model, MaxTokens: 4096, Stream: true}
	type part struct {
		Text       *string `json:"text"`
		InlineData *struct {
			MimeType string `json:"mimeType"`
			Data     string `json:"data"`
		} `json:"inlineData"`
		FunctionCall *struct {
			ID   string          `json:"id"`
			Name string          `json:"name"`
			Args json.RawMessage `json:"args"`
		} `json:"functionCall"`
		FunctionResponse *struct {
			ID       string          `json:"id"`
			Name     string          `json:"name"`
			Response json.RawMessage `json:"response"`
		} `json:"functionResponse"`
	}
	type content struct {
		Role  string            `json:"role"`
		Parts []json.RawMessage `json:"parts"`
	}
	var contents []content
	if err := json.Unmarshal(in["contents"], &contents); err != nil || len(contents) == 0 {
		return nil, fmt.Errorf("Gemini contents are required")
	}
	pending := map[string][]string{}
	for i, message := range contents {
		role := message.Role
		if role == "" {
			role = "user"
		}
		if role == "model" {
			role = "assistant"
		}
		if role != "user" && role != "assistant" {
			return nil, fmt.Errorf("unsupported Gemini role %s", role)
		}
		var blocks []Block
		for j, rawPart := range message.Parts {
			var p part
			var fields map[string]json.RawMessage
			if json.Unmarshal(rawPart, &p) != nil || json.Unmarshal(rawPart, &fields) != nil {
				return nil, fmt.Errorf("invalid Gemini part")
			}
			for key := range fields {
				if key != "text" && key != "inlineData" && key != "functionCall" && key != "functionResponse" {
					return nil, fmt.Errorf("unsupported Gemini part %s", key)
				}
			}
			if len(fields) != 1 {
				return nil, fmt.Errorf("Gemini part must contain exactly one supported value")
			}
			switch {
			case p.Text != nil:
				blocks = append(blocks, Block{Type: "text", Text: *p.Text})
			case p.InlineData != nil:
				b := Block{Type: "image"}
				b.Source = &struct {
					Type      string `json:"type"`
					MediaType string `json:"media_type"`
					Data      string `json:"data"`
				}{"base64", p.InlineData.MimeType, p.InlineData.Data}
				blocks = append(blocks, b)
			case p.FunctionCall != nil:
				if role != "assistant" || p.FunctionCall.Name == "" {
					return nil, fmt.Errorf("invalid Gemini functionCall")
				}
				call := p.FunctionCall
				id := call.ID
				if id == "" {
					id = fmt.Sprintf("call_%d_%d", i, j)
				}
				args := call.Args
				if len(args) == 0 {
					args = json.RawMessage(`{}`)
				}
				pending[call.Name] = append(pending[call.Name], id)
				blocks = append(blocks, Block{Type: "tool_use", ID: id, Name: call.Name, Input: args})
			case p.FunctionResponse != nil:
				if role != "user" {
					return nil, fmt.Errorf("functionResponse must belong to user")
				}
				result := p.FunctionResponse
				queue := pending[result.Name]
				idx := 0
				if result.ID != "" {
					idx = -1
					for k, id := range queue {
						if id == result.ID {
							idx = k
							break
						}
					}
				}
				if idx < 0 || idx >= len(queue) {
					return nil, fmt.Errorf("functionResponse has no matching functionCall")
				}
				id := queue[idx]
				pending[result.Name] = append(queue[:idx], queue[idx+1:]...)
				text, _ := json.Marshal(string(result.Response))
				blocks = append(blocks, Block{Type: "tool_result", ToolUseID: id, Content: text})
			default:
				return nil, fmt.Errorf("unsupported Gemini part")
			}
		}
		if len(blocks) == 0 {
			return nil, fmt.Errorf("empty Gemini content")
		}
		data, _ := json.Marshal(blocks)
		out.Messages = append(out.Messages, Message{Role: role, Content: data})
	}
	if rawSystem, ok := in["systemInstruction"]; ok {
		var system content
		if json.Unmarshal(rawSystem, &system) != nil {
			return nil, fmt.Errorf("invalid systemInstruction")
		}
		texts := []string{}
		for _, rawPart := range system.Parts {
			var p map[string]json.RawMessage
			if json.Unmarshal(rawPart, &p) != nil || len(p) != 1 {
				return nil, fmt.Errorf("systemInstruction requires text")
			}
			var text string
			if json.Unmarshal(p["text"], &text) != nil {
				return nil, fmt.Errorf("systemInstruction requires text")
			}
			texts = append(texts, text)
		}
		out.System, _ = json.Marshal(strings.Join(texts, "\n"))
	}
	if rawConfig, ok := in["generationConfig"]; ok {
		var cfg map[string]json.RawMessage
		if json.Unmarshal(rawConfig, &cfg) != nil {
			return nil, fmt.Errorf("invalid generationConfig")
		}
		for key, value := range cfg {
			switch key {
			case "maxOutputTokens":
				if json.Unmarshal(value, &out.MaxTokens) != nil || out.MaxTokens <= 0 {
					return nil, fmt.Errorf("invalid maxOutputTokens")
				}
			case "temperature":
				if json.Unmarshal(value, &out.Temperature) != nil {
					return nil, fmt.Errorf("invalid temperature")
				}
			case "candidateCount":
				var count int
				if json.Unmarshal(value, &count) != nil || count != 1 {
					return nil, fmt.Errorf("Kiro supports one candidate")
				}
			default:
				return nil, fmt.Errorf("Kiro does not support generationConfig.%s", key)
			}
		}
	}
	if rawTools, ok := in["tools"]; ok {
		var tools []map[string]json.RawMessage
		if json.Unmarshal(rawTools, &tools) != nil {
			return nil, fmt.Errorf("invalid tools")
		}
		for _, tool := range tools {
			if len(tool) != 1 || tool["functionDeclarations"] == nil {
				return nil, fmt.Errorf("Kiro only supports functionDeclarations")
			}
			var funcs []struct {
				Name                 string `json:"name"`
				Description          string `json:"description"`
				Parameters           any    `json:"parameters"`
				ParametersJSONSchema any    `json:"parametersJsonSchema"`
			}
			if json.Unmarshal(tool["functionDeclarations"], &funcs) != nil {
				return nil, fmt.Errorf("invalid functionDeclarations")
			}
			for _, fn := range funcs {
				schema := fn.Parameters
				if fn.ParametersJSONSchema != nil {
					if schema != nil {
						return nil, fmt.Errorf("provide only one function schema")
					}
					schema = fn.ParametersJSONSchema
				}
				if schema == nil {
					schema = map[string]any{"type": "object", "properties": map[string]any{}}
				}
				schema = geminiSchema(schema)
				encoded, _ := json.Marshal(schema)
				out.Tools = append(out.Tools, Tool{Name: fn.Name, Description: fn.Description, InputSchema: encoded})
			}
		}
	}
	if rawChoice, ok := in["toolConfig"]; ok {
		var cfg map[string]json.RawMessage
		if json.Unmarshal(rawChoice, &cfg) != nil || len(cfg) != 1 {
			return nil, fmt.Errorf("unsupported toolConfig")
		}
		var fc map[string]json.RawMessage
		if json.Unmarshal(cfg["functionCallingConfig"], &fc) != nil || len(fc) != 1 {
			return nil, fmt.Errorf("unsupported functionCallingConfig")
		}
		var mode string
		if json.Unmarshal(fc["mode"], &mode) != nil {
			return nil, fmt.Errorf("invalid tool mode")
		}
		switch mode {
		case "AUTO":
			out.ToolChoice = json.RawMessage(`{"type":"auto"}`)
		case "NONE":
			out.Tools = nil
		default:
			return nil, fmt.Errorf("Kiro supports AUTO or NONE tool modes")
		}
	}
	return json.Marshal(out)
}
func geminiSchema(value any) any {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			if key == "type" {
				if text, ok := child.(string); ok {
					v[key] = strings.ToLower(text)
					continue
				}
			}
			v[key] = geminiSchema(child)
		}
	case []any:
		for i, child := range v {
			v[i] = geminiSchema(child)
		}
	}
	return value
}
