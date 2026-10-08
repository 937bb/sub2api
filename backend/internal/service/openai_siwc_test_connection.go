package service

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/gin-gonic/gin"
)

// The admin probe reuses the production adapter, token refresh and model guard.
func (s *AccountTestService) testSIWCAccountConnection(c *gin.Context, account *Account, model, prompt string) error {
	if s.openaiGatewayService == nil {
		return s.sendErrorAndEnd(c, "SIWC gateway unavailable")
	}
	if !account.IsModelSupported(model) {
		return s.sendErrorAndEnd(c, "Select a model from this SIWC account's catalog")
	}
	if prompt == "" {
		prompt = "hi"
	}
	payload := map[string]any{"model": model, "input": prompt, "stream": true, "store": false}
	if options, ok := pelicanTestOptionsFromContext(c.Request.Context()); ok {
		payload["input"] = options.prompt
		payload["reasoning"] = map[string]any{"effort": options.reasoningEffort}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return s.sendErrorAndEnd(c, "Invalid SIWC test payload")
	}
	recorder := httptest.NewRecorder()
	probe, _ := gin.CreateTestContext(recorder)
	probe.Request, err = http.NewRequestWithContext(c.Request.Context(), http.MethodPost, "/v1/responses", bytes.NewReader(body))
	if err != nil {
		return s.sendErrorAndEnd(c, "Invalid SIWC test request")
	}
	s.sendEvent(c, TestEvent{Type: "test_start", Model: model})
	_, err = s.openaiGatewayService.Forward(probe.Request.Context(), probe, account, body)
	if err != nil {
		return s.sendErrorAndEnd(c, err.Error())
	}
	return s.processOpenAIStream(c, bytes.NewReader(recorder.Body.Bytes()))
}
