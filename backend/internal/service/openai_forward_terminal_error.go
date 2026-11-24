package service

import (
	"errors"
	"strings"
)

// OpenAIForwardErrorIsTerminal preserves the classification of an upstream
// terminal response across the non-replayable Smart WS boundary. It does not
// prove that bytes reached the client; handlers must check their writer too.
func OpenAIForwardErrorIsTerminal(err error) bool {
	if err == nil {
		return false
	}
	var transport *PluginTransportError
	if errors.As(err, &transport) {
		return transport.TerminalResponse
	}
	message := strings.TrimSpace(err.Error())
	return strings.HasPrefix(message, "upstream response failed:") ||
		strings.HasPrefix(message, "non-streaming openai protocol error:")
}
