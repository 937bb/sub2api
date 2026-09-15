package service

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io"
	"strings"

	"github.com/tidwall/gjson"
)

type codexTextEdit struct {
	start, end int
	value      string
}

// Codex renders this as a contextual user fragment. Only rewrite complete
// environment fragments, never quoted examples, arbitrary prose or tool data.
// Source: openai/codex core/src/context/world_state/environment.rs.
func normalizeCodexEnvironmentContext(text string) string {
	if !strings.Contains(text, "<timezone>") || !strings.HasPrefix(strings.TrimSpace(text), "<environment_context>") {
		return text
	}
	decoder := xml.NewDecoder(strings.NewReader(text))
	depth, timezoneStart := 0, -1
	var edits []codexTextEdit
	for {
		before := int(decoder.InputOffset())
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return text
		}
		switch token := token.(type) {
		case xml.StartElement:
			if depth == 0 && (token.Name.Local != "environment_context" || token.Name.Space != "" || len(token.Attr) != 0) {
				return text
			}
			depth++
			if depth == 2 && token.Name.Local == "timezone" && token.Name.Space == "" && len(token.Attr) == 0 {
				timezoneStart = int(decoder.InputOffset())
			} else if timezoneStart >= 0 {
				return text
			}
		case xml.EndElement:
			if depth == 2 && timezoneStart >= 0 {
				value := text[timezoneStart:before]
				if strings.TrimSpace(value) != "" && value != codexClientTimezone {
					edits = append(edits, codexTextEdit{timezoneStart, before, codexClientTimezone})
				}
				timezoneStart = -1
			}
			depth--
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(token)) != "" {
				return text
			}
		case xml.Directive, xml.ProcInst:
			return text
		}
	}
	if depth != 0 || len(edits) == 0 {
		return text
	}
	var out strings.Builder
	previous := 0
	for _, edit := range edits {
		out.WriteString(text[previous:edit.start])
		out.WriteString(edit.value)
		previous = edit.end
	}
	out.WriteString(text[previous:])
	return out.String()
}

// Splice only changed JSON strings in one pass; retain opaque items, numeric
// precision and image bytes without decoding or re-encoding the full request.
func applyCodexEnvironmentContextRaw(body []byte) ([]byte, bool) {
	if !bytes.Contains(body, []byte("environment_context")) {
		return body, false
	}
	var edits []codexTextEdit
	visitText := func(value gjson.Result) {
		if value.Type != gjson.String {
			return
		}
		original := value.String()
		if next := normalizeCodexEnvironmentContext(original); next != original {
			encoded, err := json.Marshal(next)
			if err == nil {
				edits = append(edits, codexTextEdit{value.Index, value.Index + len(value.Raw), string(encoded)})
			}
		}
	}
	root := gjson.ParseBytes(body)
	if !root.IsObject() || !gjson.ValidBytes(body) {
		return body, false
	}
	// Iterate fields in wire order so edits remain sorted without reordering
	// input or copying the whole request once for every historical message.
	root.ForEach(func(key, value gjson.Result) bool {
		switch key.String() {
		case "instructions":
			visitText(value)
		case "input":
			if value.Type == gjson.String {
				visitText(value)
			} else if value.IsArray() {
				value.ForEach(func(_, item gjson.Result) bool {
					role, kind := item.Get("role").String(), item.Get("type").String()
					if (role != "user" && role != "developer" && role != "system") || (kind != "" && kind != "message") {
						return true
					}
					content := item.Get("content")
					if content.IsArray() {
						content.ForEach(func(_, part gjson.Result) bool {
							if kind := part.Get("type").String(); kind == "input_text" || kind == "text" {
								visitText(part.Get("text"))
							}
							return true
						})
					} else {
						visitText(content)
					}
					return true
				})
			}
		}
		return true
	})
	if len(edits) == 0 {
		return body, false
	}
	var out bytes.Buffer
	out.Grow(len(body))
	previous := 0
	for _, edit := range edits {
		if edit.start < previous || edit.end > len(body) || edit.start >= edit.end {
			return body, false
		}
		out.Write(body[previous:edit.start])
		out.WriteString(edit.value)
		previous = edit.end
	}
	out.Write(body[previous:])
	return out.Bytes(), true
}

func applyCodexEnvironmentContextMap(body map[string]any) bool {
	changed := false
	visit := func(values map[string]any, key string) {
		if text, ok := values[key].(string); ok {
			if next := normalizeCodexEnvironmentContext(text); next != text {
				values[key] = next
				changed = true
			}
		}
	}
	visit(body, "instructions")
	visit(body, "input")
	items, _ := body["input"].([]any)
	for _, value := range items {
		item, ok := value.(map[string]any)
		if !ok {
			continue
		}
		role, _ := item["role"].(string)
		kind, _ := item["type"].(string)
		if (role != "user" && role != "developer" && role != "system") || (kind != "" && kind != "message") {
			continue
		}
		visit(item, "content")
		parts, _ := item["content"].([]any)
		for _, value := range parts {
			if part, ok := value.(map[string]any); ok && (part["type"] == "input_text" || part["type"] == "text") {
				visit(part, "text")
			}
		}
	}
	return changed
}
