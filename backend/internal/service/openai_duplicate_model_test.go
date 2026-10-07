package service

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestForwardRejectsAmbiguousModelBeforeDependencies(t *testing.T) {
	for _, body := range []string{
		`{"model":"gpt-6-astra","model":"gpt-6.1-sol","messages":[]}`,
		`{"model":"gpt-6.1-sol","model":"gpt-6-astra","input":"hi"}`,
		`{"model":"gpt-6-astra","Model":"gpt-6.1-sol","input":"hi"}`,
	} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		// No account, upstream client or billing dependencies: rejection must precede all of them.
		s := &OpenAIGatewayService{}
		result, err := s.Forward(context.Background(), c, nil, []byte(body))
		if err == nil || result != nil || recorder.Code != 400 {
			t.Fatalf("result=%v err=%v status=%d", result, err, recorder.Code)
		}
	}
}
