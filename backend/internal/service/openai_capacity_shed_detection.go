package service

import (
	"strings"

	"github.com/tidwall/gjson"
)

type openAICapacityShedMatch struct {
	Code    string
	Message string
}

// detectOpenAIUpstreamCapacityShed only inspects structured upstream error
// envelopes. Normal assistant output is never scanned for overload wording.
func detectOpenAIUpstreamCapacityShed(payload []byte) (openAICapacityShedMatch, bool) {
	if len(payload) == 0 || !gjson.ValidBytes(payload) {
		return openAICapacityShedMatch{}, false
	}

	for _, path := range []string{"response.error", "error"} {
		errorValue := gjson.GetBytes(payload, path)
		if !errorValue.Exists() || !errorValue.IsObject() {
			continue
		}

		code := strings.ToLower(strings.TrimSpace(errorValue.Get("code").String()))
		message := strings.TrimSpace(errorValue.Get("message").String())
		switch code {
		case "server_is_overloaded", "slow_down", "model_overloaded", "overloaded_error":
			return openAICapacityShedMatch{Code: code, Message: message}, true
		}

		lowerMessage := strings.ToLower(message)
		for _, marker := range []string{
			"server is overloaded",
			"servers are overloaded",
			"server is currently overloaded",
			"servers are currently overloaded",
			"selected model is at capacity",
			"model is at capacity",
			"model is currently overloaded",
		} {
			if strings.Contains(lowerMessage, marker) {
				return openAICapacityShedMatch{Code: code, Message: message}, true
			}
		}
	}
	return openAICapacityShedMatch{}, false
}

// findOpenAIUpstreamCapacityShed accepts either a JSON response or a buffered
// Responses SSE body and returns the structured error payload that matched.
func findOpenAIUpstreamCapacityShed(body []byte) ([]byte, string, bool) {
	if match, ok := detectOpenAIUpstreamCapacityShed(body); ok {
		return append([]byte(nil), body...), match.Message, true
	}
	if !bodyHasSSEFraming(body) {
		return nil, "", false
	}

	var matchedPayload []byte
	matchedMessage := ""
	forEachOpenAISSEDataPayload(string(body), func(payload []byte) {
		if matchedPayload != nil {
			return
		}
		if match, ok := detectOpenAIUpstreamCapacityShed(payload); ok {
			matchedPayload = append([]byte(nil), payload...)
			matchedMessage = match.Message
		}
	})
	return matchedPayload, matchedMessage, matchedPayload != nil
}
