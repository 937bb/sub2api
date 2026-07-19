package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"
)

const (
	openAIPlanGatedBodyMaxBytes = 64 << 10
	openAIPlanGatedMaxDepth     = 32
	openAIPlanGatedMaxTokens    = 4096
)

const (
	openAIPlanGatedPrefix = "The '"
	openAIPlanGatedSuffix = "' model is not supported when using Codex with a ChatGPT account."
)

var upstreamModelNotFoundKeywords = []string{"model not found", "unknown model", "not found"}

func isUpstreamModelNotFoundError(statusCode int, body []byte) bool {
	if statusCode != http.StatusNotFound {
		return false
	}
	normalized := normalizeModelNotFoundBody(body)
	if normalized == "" || !strings.Contains(normalized, "model") {
		return false
	}
	return containsModelNotFoundKeyword(normalized)
}

func isModelNotFoundError(statusCode int, body []byte) bool {
	return isUpstreamModelNotFoundError(statusCode, body) || statusCode == http.StatusNotFound
}

func containsModelNotFoundKeyword(normalizedBody string) bool {
	if normalizedBody == "" {
		return false
	}
	for _, keyword := range upstreamModelNotFoundKeywords {
		if strings.Contains(normalizedBody, keyword) {
			return true
		}
	}
	return false
}

func normalizeModelNotFoundBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	normalized := strings.ToLower(string(body))
	normalized = strings.NewReplacer("_", " ", "-", " ", "\n", " ", "\r", " ", "\t", " ").Replace(normalized)
	return strings.Join(strings.Fields(normalized), " ")
}

func openAIPlanGatedModel(body []byte) (string, bool) {
	if len(body) == 0 || len(body) > openAIPlanGatedBodyMaxBytes || !utf8.Valid(body) {
		return "", false
	}
	parser := openAIPlanGateParser{decoder: json.NewDecoder(bytes.NewReader(body))}
	first, err := parser.next()
	if err != nil || first != json.Delim('{') {
		return "", false
	}
	if err := parser.consume(first, openAIPlanGatePathRoot, 1); err != nil {
		return "", false
	}
	if token, err := parser.next(); err != io.EOF || token != nil {
		return "", false
	}
	if len(parser.messages) != 1 {
		return "", false
	}

	message := parser.messages[0]
	if !strings.HasPrefix(message, openAIPlanGatedPrefix) || !strings.HasSuffix(message, openAIPlanGatedSuffix) {
		return "", false
	}
	model := strings.TrimSuffix(strings.TrimPrefix(message, openAIPlanGatedPrefix), openAIPlanGatedSuffix)
	if model == "" || strings.ContainsAny(model, "'\r\n") || strings.TrimSpace(model) != model {
		return "", false
	}
	return model, true
}

var errOpenAIPlanGateBoundExceeded = errors.New("OpenAI plan-gate classification bound exceeded")

type openAIPlanGatePath uint8

const (
	openAIPlanGatePathOther openAIPlanGatePath = iota
	openAIPlanGatePathRoot
	openAIPlanGatePathError
)

type openAIPlanGateParser struct {
	decoder  *json.Decoder
	messages []string
	tokens   int
}

func (p *openAIPlanGateParser) next() (json.Token, error) {
	if p.tokens >= openAIPlanGatedMaxTokens {
		return nil, errOpenAIPlanGateBoundExceeded
	}
	p.tokens++
	return p.decoder.Token()
}

// consume traverses the bounded document but only retains exact trusted-path
// string leaves. Duplicate or case-colliding object keys fail closed.
func (p *openAIPlanGateParser) consume(token json.Token, path openAIPlanGatePath, depth int) error {
	if depth > openAIPlanGatedMaxDepth {
		return errOpenAIPlanGateBoundExceeded
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := make(map[string]struct{})
		for p.decoder.More() {
			keyToken, err := p.next()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("OpenAI plan-gate object key is not a string")
			}
			folded := strings.ToLower(key)
			if _, exists := seen[folded]; exists {
				return errors.New("OpenAI plan-gate object key is ambiguous")
			}
			seen[folded] = struct{}{}

			value, err := p.next()
			if err != nil {
				return err
			}
			childPath := openAIPlanGatePathOther
			trustedLeaf := false
			switch path {
			case openAIPlanGatePathRoot:
				switch folded {
				case "detail":
					if key != "detail" {
						return errors.New("OpenAI plan-gate detail key is ambiguous")
					}
					trustedLeaf = true
				case "error":
					if key != "error" || value != json.Delim('{') {
						return errors.New("OpenAI plan-gate error field is invalid")
					}
					childPath = openAIPlanGatePathError
				}
			case openAIPlanGatePathError:
				if folded == "message" {
					if key != "message" {
						return errors.New("OpenAI plan-gate message key is ambiguous")
					}
					trustedLeaf = true
				}
			}
			if trustedLeaf {
				message, ok := value.(string)
				if !ok {
					return errors.New("OpenAI plan-gate message is not a string")
				}
				p.messages = append(p.messages, message)
				continue
			}
			if err := p.consume(value, childPath, depth+1); err != nil {
				return err
			}
		}
		end, err := p.next()
		if err != nil || end != json.Delim('}') {
			return errors.New("OpenAI plan-gate object is incomplete")
		}
		return nil
	case '[':
		for p.decoder.More() {
			value, err := p.next()
			if err != nil {
				return err
			}
			if err := p.consume(value, openAIPlanGatePathOther, depth+1); err != nil {
				return err
			}
		}
		end, err := p.next()
		if err != nil || end != json.Delim(']') {
			return errors.New("OpenAI plan-gate array is incomplete")
		}
		return nil
	default:
		return errors.New("unexpected OpenAI plan-gate JSON delimiter")
	}
}
