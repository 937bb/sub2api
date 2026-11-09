package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/requestaudit"
	"github.com/gin-gonic/gin"
)

func TestOpenAIRequestAuditTransportPreservesBodyAndIdentity(t *testing.T) {
	// No parallel execution: production's process singleton is replaced only for
	// this synchronous test, without starting a background maintenance goroutine.
	oldStore, oldInit := openAIAuditStore, openAIAuditInitError
	oldWritten, oldErrors := openAIAuditWritten.Load(), openAIAuditErrors.Load()
	openAIAuditOnce.Do(func() {})
	s, err := requestaudit.New(t.TempDir(), bytes.Repeat([]byte{5}, 32), 1<<20, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	openAIAuditStore = s
	defer func() {
		openAIAuditStore = oldStore
		openAIAuditInitError = oldInit
		openAIAuditWritten.Store(oldWritten)
		openAIAuditErrors.Store(oldErrors)
	}()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("api_key", &APIKey{ID: 71, UserID: 23})
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	ctx := context.WithValue(context.Background(), ctxkey.RequestID, "local-req-9")
	account := &Account{ID: 98, Platform: PlatformOpenAI}
	body := []byte(`{"model":"test-model","input":[{"role":"user","content":"a private question"}]}`)
	ctx = beginOpenAIRequestAudit(ctx, c, account, body)
	upstreamCtx, release := detachUpstreamContext(ctx)
	defer release()
	req, _ := http.NewRequestWithContext(upstreamCtx, "POST", "https://example.invalid/v1/responses", bytes.NewReader(body))
	auditOpenAIHTTPRequest(req, account)
	auditOpenAIHTTPResult(req, account, &http.Response{StatusCode: 400, Header: http.Header{"X-Request-Id": []string{"upstream-9"}}}, nil)
	forwarded, _ := io.ReadAll(req.Body)
	if !bytes.Equal(forwarded, body) {
		t.Fatal("audit consumed or changed request")
	}
	auditOpenAIRequest(ctx, c, &Account{ID: 99, Platform: PlatformAnthropic}, "excluded", body)
	paths, _ := filepath.Glob(filepath.Join(s.Dir, "audit-*.jsonl"))
	var rows []requestaudit.Record
	for _, p := range paths {
		raw, _ := os.ReadFile(p)
		for _, line := range bytes.Split(bytes.TrimSpace(raw), []byte{'\n'}) {
			r, e := s.Decode(line)
			if e != nil {
				t.Fatal(e)
			}
			rows = append(rows, r)
		}
	}
	if len(rows) != 3 {
		t.Fatalf("records=%d", len(rows))
	}
	for _, r := range rows {
		if r.RequestID != "local-req-9" || r.UserID != 23 || r.APIKeyID != 71 || r.AccountID != 98 || r.AttemptID == "" || r.AttemptID != rows[0].AttemptID {
			t.Fatalf("wrong association: %+v", r)
		}
	}
	if rows[2].UpstreamRequestID != "upstream-9" || rows[2].HTTPStatus != 400 {
		t.Fatal("missing upstream correlation")
	}
	var snapshot map[string]any
	json.Unmarshal(rows[1].Body, &snapshot)
	if snapshot["model"] != "test-model" {
		t.Fatal("missing body")
	}
}
