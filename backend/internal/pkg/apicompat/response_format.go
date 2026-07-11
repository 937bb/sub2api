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
	if !json.Valid(raw) {
		return nil, false
	}
	i := skipJSONSpace(raw, 0)
	if i >= len(raw) || raw[i] != '{' {
		return nil, false
	}
	i++
	var out []rawJSONMember
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
		valueEnd, ok := scanJSONValue(raw, i)
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
			continue
		}
		if raw[i] != '}' {
			return nil, false
		}
	}
}

func scanJSONValue(raw []byte, i int) (int, bool) {
	if i >= len(raw) {
		return 0, false
	}
	if raw[i] == '"' {
		return scanJSONString(raw, i)
	}
	if raw[i] == '{' || raw[i] == '[' {
		stack := []byte{raw[i]}
		i++
		for i < len(raw) {
			if raw[i] == '"' {
				var ok bool
				i, ok = scanJSONString(raw, i)
				if !ok {
					return 0, false
				}
				continue
			}
			switch raw[i] {
			case '{', '[':
				stack = append(stack, raw[i])
			case '}', ']':
				open := stack[len(stack)-1]
				if (open == '{') != (raw[i] == '}') {
					return 0, false
				}
				stack = stack[:len(stack)-1]
				if len(stack) == 0 {
					return i + 1, true
				}
			}
			i++
		}
		return 0, false
	}
	end := i
	for end < len(raw) && !bytes.ContainsRune([]byte(",}] \t\r\n"), rune(raw[end])) {
		end++
	}
	if end == i || !json.Valid(raw[i:end]) {
		return 0, false
	}
	return end, true
}

func scanJSONString(raw []byte, i int) (int, bool) {
	if i >= len(raw) || raw[i] != '"' {
		return 0, false
	}
	i++
	for i < len(raw) {
		if raw[i] == '\\' {
			i += 2
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

func skipJSONSpace(raw []byte, i int) int {
	for i < len(raw) && (raw[i] == ' ' || raw[i] == '\t' || raw[i] == '\r' || raw[i] == '\n') {
		i++
	}
	return i
}
