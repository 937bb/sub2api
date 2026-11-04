package service

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// The measured CLI sends compact JSON without an encoder newline or HTML
// escaping. JSON string escaping still protects quotes and control characters.
func marshalCodexWSJSON(value any) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, fmt.Errorf("encode Codex WS message: %w", err)
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte{'\n'}), nil
}
