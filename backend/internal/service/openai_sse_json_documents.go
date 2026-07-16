package service

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	maxOpenAIConcatenatedJSONDocuments = 16
	maxOpenAIConcatenatedJSONBytes     = 16 * 1024 * 1024
)

type openAIJSONDocumentRepairOutcome uint8

const (
	openAIJSONDocumentsUnchanged openAIJSONDocumentRepairOutcome = iota
	openAIJSONDocumentsRepaired
	openAIJSONDocumentsRejected
)

var errOpenAIConcatenatedJSONRejected = errors.New("rejected concatenated OpenAI JSON documents")

// openAIConcatenatedJSONDecodeHook is set only by same-package tests.
var openAIConcatenatedJSONDecodeHook func()

// openAIJSONHasTrailingDocumentCandidate is the allocation-free hot-path gate.
// Responses envelopes are objects, so a non-space byte after the first complete
// top-level object is sufficient to require bounded decoding or rejection.
func openAIJSONHasTrailingDocumentCandidate(payload []byte) bool {
	payload = bytes.TrimSpace(payload)
	if len(payload) == 0 || payload[0] != '{' {
		return false
	}
	depth := 0
	inString := false
	escaped := false
	for i, b := range payload {
		if inString {
			if escaped {
				escaped = false
			} else if b == '\\' {
				escaped = true
			} else if b == '"' {
				inString = false
			}
			continue
		}
		switch b {
		case '"':
			inString = true
		case '{', '[':
			depth++
		case '}', ']':
			depth--
			if depth == 0 {
				return len(bytes.TrimSpace(payload[i+1:])) > 0
			}
		}
	}
	return false
}

// splitOpenAIConcatenatedJSONDocuments recognizes multiple complete Responses
// envelopes and distinguishes detected-but-invalid concatenation from ordinary input.
func splitOpenAIConcatenatedJSONDocuments(payload []byte) ([][]byte, openAIJSONDocumentRepairOutcome) {
	if !openAIJSONHasTrailingDocumentCandidate(payload) {
		return nil, openAIJSONDocumentsUnchanged
	}
	payload = bytes.TrimSpace(payload)
	if len(payload) > maxOpenAIConcatenatedJSONBytes {
		return nil, openAIJSONDocumentsRejected
	}
	if openAIConcatenatedJSONDecodeHook != nil {
		openAIConcatenatedJSONDecodeHook()
	}

	decoder := json.NewDecoder(bytes.NewReader(payload))
	documents := make([][]byte, 0, 2)
	for {
		var raw json.RawMessage
		err := decoder.Decode(&raw)
		if err != nil {
			if err == io.EOF && len(documents) > 1 {
				return documents, openAIJSONDocumentsRepaired
			}
			return nil, openAIJSONDocumentsRejected
		}
		raw = bytes.TrimSpace(raw)
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return nil, openAIJSONDocumentsRejected
		}
		eventType := strings.TrimSpace(envelope.Type)
		if eventType == "" || strings.ContainsAny(eventType, "\r\n") {
			return nil, openAIJSONDocumentsRejected
		}
		if len(documents) == maxOpenAIConcatenatedJSONDocuments {
			return nil, openAIJSONDocumentsRejected
		}
		documents = append(documents, raw)
	}
}

type openAISSEJSONDocumentScanner struct {
	scanner   *bufio.Scanner
	pending   []string
	current   string
	repairErr error
}

func newOpenAISSEJSONDocumentScanner(scanner *bufio.Scanner) *openAISSEJSONDocumentScanner {
	return &openAISSEJSONDocumentScanner{scanner: scanner}
}

func (s *openAISSEJSONDocumentScanner) Scan() bool {
	if len(s.pending) > 0 {
		s.current = s.pending[0]
		s.pending = s.pending[1:]
		return true
	}
	if s.scanner == nil || !s.scanner.Scan() {
		return false
	}

	line := s.scanner.Text()
	data, ok := extractOpenAISSEDataLine(line)
	if !ok {
		s.current = line
		return true
	}
	documents, outcome := splitOpenAIConcatenatedJSONDocuments([]byte(data))
	switch outcome {
	case openAIJSONDocumentsUnchanged:
		s.current = line
		return true
	case openAIJSONDocumentsRejected:
		s.repairErr = fmt.Errorf("%w: invalid trailing JSON", errOpenAIConcatenatedJSONRejected)
		return false
	}

	expanded := make([]string, 0, len(documents)*3)
	for _, document := range documents {
		var envelope struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal(document, &envelope)
		eventType := strings.TrimSpace(envelope.Type)
		expanded = append(expanded, "event: "+eventType, "data: "+string(document), "")
		if openAIResponseStreamEventTypeIsTerminal(eventType) {
			break
		}
	}
	s.current = expanded[0]
	s.pending = expanded[1:]
	return true
}

func (s *openAISSEJSONDocumentScanner) Text() string { return s.current }

func (s *openAISSEJSONDocumentScanner) Err() error {
	if s.repairErr != nil {
		return s.repairErr
	}
	if s.scanner == nil {
		return nil
	}
	return s.scanner.Err()
}
