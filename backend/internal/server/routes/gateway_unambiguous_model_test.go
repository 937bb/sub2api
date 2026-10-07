package routes

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGatewayAliasesRejectAmbiguousModelsBeforeRouting(t *testing.T) {
	router := newGatewayRoutesTestRouter()
	for _, path := range []string{
		"/v1/responses", "/responses", "/v1/responses/compact", "/responses/compact",
		"/backend-api/codex/responses", "/backend-api/codex/responses/compact",
		"/v1/chat/completions", "/chat/completions", "/v1/messages",
		"/v1/embeddings", "/embeddings", "/v1/images/generations", "/v1/live",
		"/antigravity/v1/messages",
	} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest("POST", path, strings.NewReader(`{"model":"gpt-6-astra","model":"gpt-6.1-sol","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
			req.Header.Set("Content-Type", "application/json")
			res := httptest.NewRecorder()
			router.ServeHTTP(res, req)
			if res.Code != 400 || !strings.Contains(res.Body.String(), "ambiguous_model") {
				t.Fatalf("status=%d body=%s", res.Code, res.Body)
			}
		})
	}
}
