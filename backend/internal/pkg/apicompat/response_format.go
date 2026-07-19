package apicompat

import (
	"bytes"
	"encoding/json"
)

type rawJSONMember struct {
	key        string
	start, end int
	value      json.RawMessage
}

func chatResponseFormatToResponsesTextFormat(raw json.RawMessage) json.RawMessage {
	raw = normalizedFormatJSON(raw)
	if len(raw) == 0 {
		return nil
	}
	members, ok := scanRawJSONObject(raw)
	typeMember, schemaMember, safe := uniqueFormatMembers(members, "json_schema")
	if !ok || !safe || typeMember == nil || schemaMember == nil || schemaMember.key != "json_schema" {
		return raw
	}
	schema := bytes.TrimSpace(schemaMember.value)
	inner, innerOK := scanRawJSONObject(schema)
	if !innerOK || hasMember(inner, "type") {
		return raw
	}
	return insertObjectMember(schema, `"type":"json_schema"`)
}

func responsesTextFormatToChatResponseFormat(raw json.RawMessage) json.RawMessage {
	raw = normalizedFormatJSON(raw)
	if len(raw) == 0 {
		return nil
	}
	members, ok := scanRawJSONObject(raw)
	typeMember, _, safe := uniqueFormatMembers(members, "json_schema")
	if !ok || !safe || typeMember == nil || len(members) < 2 || hasMember(members, "json_schema") {
		return raw
	}
	var body []byte
	for _, member := range members {
		if member.key == "type" {
			continue
		}
		if len(body) > 0 {
			body = append(body, ',')
		}
		body = append(body, raw[member.start:member.end]...)
	}
	out := append([]byte(`{"type":"json_schema","json_schema":{`), body...)
	out = append(out, '}', '}')
	return out
}

func normalizedFormatJSON(raw json.RawMessage) json.RawMessage {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil
	}
	return raw
}

func uniqueFormatMembers(members []rawJSONMember, wantedType string) (*rawJSONMember, *rawJSONMember, bool) {
	var typ, schema *rawJSONMember
	for i := range members {
		switch members[i].key {
		case "type":
			if typ != nil {
				return nil, nil, false
			}
			typ = &members[i]
		case "json_schema":
			if schema != nil {
				return nil, nil, false
			}
			schema = &members[i]
		}
	}
	if typ == nil {
		return nil, schema, true
	}
	var value string
	if json.Unmarshal(typ.value, &value) != nil || value != wantedType {
		return nil, schema, true
	}
	return typ, schema, true
}

func hasMember(members []rawJSONMember, key string) bool {
	for _, member := range members {
		if member.key == key {
			return true
		}
	}
	return false
}

func insertObjectMember(obj []byte, member string) json.RawMessage {
	inner := bytes.TrimSpace(obj[1 : len(obj)-1])
	out := append([]byte{'{'}, member...)
	if len(inner) > 0 {
		out = append(out, ',')
		out = append(out, inner...)
	}
	return append(out, '}')
}

func scanRawJSONObject(raw []byte) ([]rawJSONMember, bool) {
	i := skipJSONSpace(raw, 0)
	if i >= len(raw) || raw[i] != '{' {
		return nil, false
	}
	i++
	out := make([]rawJSONMember, 0, 4)
	for {
		i = skipJSONSpace(raw, i)
		if i < len(raw) && raw[i] == '}' {
			i++
			return out, skipJSONSpace(raw, i) == len(raw)
		}
		start := i
		keyEnd, ok := scanJSONString(raw, i)
		if !ok {
			return nil, false
		}
		var key string
		if json.Unmarshal(raw[i:keyEnd], &key) != nil {
			return nil, false
		}
		i = skipJSONSpace(raw, keyEnd)
		if i >= len(raw) || raw[i] != ':' {
			return nil, false
		}
		i++
		i = skipJSONSpace(raw, i)
		valueStart := i
		valueEnd, ok := scanJSONValue(raw, i, 1)
		if !ok {
			return nil, false
		}
		out = append(out, rawJSONMember{key: key, start: start, end: valueEnd, value: raw[valueStart:valueEnd]})
		i = skipJSONSpace(raw, valueEnd)
		if i >= len(raw) {
			return nil, false
		}
		if raw[i] == ',' {
			i++
			if next := skipJSONSpace(raw, i); next >= len(raw) || raw[next] == '}' {
				return nil, false
			}
			continue
		}
		if raw[i] != '}' {
			return nil, false
		}
	}
}

const maxJSONNestingDepth = 10000

type jsonScanFrame struct {
	close  byte
	object bool
}

func scanJSONValue(raw []byte, i, depth int) (int, bool) {
	stack := make([]jsonScanFrame, 0, 8)
	wantValue := true
	for {
		i = skipJSONSpace(raw, i)
		if i >= len(raw) {
			return 0, false
		}

		if wantValue {
			var ok bool
			switch raw[i] {
			case '"':
				i, ok = scanJSONString(raw, i)
			case '{', '[':
				if depth+len(stack) == maxJSONNestingDepth {
					return 0, false
				}
				open := raw[i]
				close := byte(']')
				if open == '{' {
					close = '}'
				}
				stack = append(stack, jsonScanFrame{close: close, object: open == '{'})
				i = skipJSONSpace(raw, i+1)
				if i < len(raw) && raw[i] == stack[len(stack)-1].close {
					i++
					stack = stack[:len(stack)-1]
					if len(stack) == 0 {
						return i, true
					}
					wantValue = false
					continue
				}
				if open == '{' {
					i, ok = scanJSONString(raw, i)
					if !ok {
						return 0, false
					}
					i = skipJSONSpace(raw, i)
					if i >= len(raw) || raw[i] != ':' {
						return 0, false
					}
					i++
				}
				continue
			case 't':
				i, ok = scanJSONLiteral(raw, i, "true")
			case 'f':
				i, ok = scanJSONLiteral(raw, i, "false")
			case 'n':
				i, ok = scanJSONLiteral(raw, i, "null")
			default:
				i, ok = scanJSONNumber(raw, i)
			}
			if !ok {
				return 0, false
			}
			wantValue = false
		}

		if len(stack) == 0 {
			return i, true
		}
		i = skipJSONSpace(raw, i)
		frame := &stack[len(stack)-1]
		if i >= len(raw) {
			return 0, false
		}
		if raw[i] == frame.close {
			i++
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				return i, true
			}
			continue
		}
		if raw[i] != ',' {
			return 0, false
		}
		i = skipJSONSpace(raw, i+1)
		if i >= len(raw) || raw[i] == frame.close {
			return 0, false
		}
		if frame.object {
			var ok bool
			i, ok = scanJSONString(raw, i)
			if !ok {
				return 0, false
			}
			i = skipJSONSpace(raw, i)
			if i >= len(raw) || raw[i] != ':' {
				return 0, false
			}
			i++
		}
		wantValue = true
	}
}

func scanJSONLiteral(raw []byte, i int, literal string) (int, bool) {
	end := i + len(literal)
	return end, end <= len(raw) && string(raw[i:end]) == literal
}

func scanJSONNumber(raw []byte, i int) (int, bool) {
	start := i
	if i < len(raw) && raw[i] == '-' {
		i++
	}
	if i >= len(raw) {
		return 0, false
	}
	if raw[i] == '0' {
		i++
	} else {
		if raw[i] < '1' || raw[i] > '9' {
			return 0, false
		}
		for i < len(raw) && raw[i] >= '0' && raw[i] <= '9' {
			i++
		}
	}
	if i < len(raw) && raw[i] == '.' {
		i++
		if i >= len(raw) || raw[i] < '0' || raw[i] > '9' {
			return 0, false
		}
		for i < len(raw) && raw[i] >= '0' && raw[i] <= '9' {
			i++
		}
	}
	if i < len(raw) && (raw[i] == 'e' || raw[i] == 'E') {
		i++
		if i < len(raw) && (raw[i] == '+' || raw[i] == '-') {
			i++
		}
		if i >= len(raw) || raw[i] < '0' || raw[i] > '9' {
			return 0, false
		}
		for i < len(raw) && raw[i] >= '0' && raw[i] <= '9' {
			i++
		}
	}
	return i, i > start
}

func scanJSONString(raw []byte, i int) (int, bool) {
	if i >= len(raw) || raw[i] != '"' {
		return 0, false
	}
	i++
	for i < len(raw) {
		if raw[i] == '\\' {
			i++
			if i >= len(raw) || !bytes.ContainsRune([]byte(`"\\/bfnrtu`), rune(raw[i])) {
				return 0, false
			}
			if raw[i] == 'u' {
				for n := 0; n < 4; n++ {
					i++
					if i >= len(raw) || !isJSONHex(raw[i]) {
						return 0, false
					}
				}
			}
			i++
			continue
		}
		if raw[i] == '"' {
			return i + 1, true
		}
		if raw[i] < 0x20 {
			return 0, false
		}
		i++
	}
	return 0, false
}

func isJSONHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

func skipJSONSpace(raw []byte, i int) int {
	for i < len(raw) && (raw[i] == ' ' || raw[i] == '\t' || raw[i] == '\r' || raw[i] == '\n') {
		i++
	}
	return i
}
