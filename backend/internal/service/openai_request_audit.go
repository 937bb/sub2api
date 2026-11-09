package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/requestaudit"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

type openAIAuditContextKey struct{}

var openAIAuditInstanceID = uuid.NewString()
var openAIAuditOnce sync.Once
var openAIAuditStore *requestaudit.Store
var openAIAuditInitError string
var openAIAuditWritten atomic.Int64
var openAIAuditErrors atomic.Int64

func openAIRequestAuditStore() *requestaudit.Store {
	openAIAuditOnce.Do(func() {
		dir := os.Getenv("OPENAI_REQUEST_AUDIT_DIR")
		if dir == "" {
			return
		}
		key, err := os.ReadFile(os.Getenv("OPENAI_REQUEST_AUDIT_KEY_FILE"))
		if err == nil {
			openAIAuditStore, err = requestaudit.New(dir, key, 5<<30, 72*time.Hour)
		}
		if err != nil {
			openAIAuditInitError = err.Error()
			slog.Error("openai_request_audit_init_failed", "error", err)
			return
		}
		go func() {
			t := time.NewTicker(10 * time.Second)
			defer t.Stop()
			for range t.C {
				if err := openAIAuditStore.Prune(); err != nil {
					reportOpenAIAuditError(err)
				}
				b, _ := json.Marshal(openAIRequestAuditStats())
				p := filepath.Join(dir, "status-"+openAIAuditInstanceID+".json")
				if err := os.WriteFile(p+".tmp", b, 0600); err == nil {
					_ = os.Rename(p+".tmp", p)
				}
			}
		}()
	})
	return openAIAuditStore
}

func openAIRequestAuditStats() map[string]any {
	return map[string]any{"instance_id": openAIAuditInstanceID, "server_port": os.Getenv("SERVER_PORT"), "pid": os.Getpid(), "enabled": openAIAuditStore != nil, "init_error": openAIAuditInitError, "written": openAIAuditWritten.Load(), "write_errors": openAIAuditErrors.Load(), "retention_hours": 72, "quota_bytes": int64(5 << 30), "at": time.Now().UTC()}
}

func reportOpenAIAuditError(err error) {
	n := openAIAuditErrors.Add(1)
	if n == 1 || n%100 == 0 {
		slog.Error("openai_request_audit_write_failed", "error", err, "failure_count", n)
	}
}

func openAIRequestAuditMeta(ctx context.Context, c *gin.Context) requestaudit.Record {
	r, _ := ctx.Value(openAIAuditContextKey{}).(requestaudit.Record)
	if id, ok := ctx.Value(ctxkey.RequestID).(string); ok {
		r.RequestID = id
	}
	if id, ok := ctx.Value(ctxkey.ClientRequestID).(string); ok {
		r.ClientRequestID = id
	}
	if id, ok := ctx.Value(ctxkey.UserID).(int64); ok {
		r.UserID = id
	}
	if c != nil {
		r.APIKeyID = getAPIKeyIDFromContext(c)
		if v, ok := c.Get("api_key"); ok {
			if k, ok := v.(*APIKey); ok && k != nil {
				r.UserID = k.UserID
			}
		}
		if c.Request != nil {
			r.Endpoint = c.Request.URL.Path
			if r.RequestID == "" {
				r.RequestID, _ = c.Request.Context().Value(ctxkey.RequestID).(string)
			}
		}
	}
	return r
}

func beginOpenAIRequestAudit(ctx context.Context, c *gin.Context, account *Account, body []byte) context.Context {
	if account == nil || account.Platform != PlatformOpenAI || openAIRequestAuditStore() == nil {
		return ctx
	}
	meta := openAIRequestAuditMeta(ctx, c)
	if meta.AttemptID == "" {
		meta.AttemptID = uuid.NewString()
	}
	ctx = context.WithValue(ctx, openAIAuditContextKey{}, meta)
	if body != nil {
		auditOpenAIRequest(ctx, c, account, "account_attempt_input", body)
	}
	return ctx
}

func auditOpenAIRequest(ctx context.Context, c *gin.Context, account *Account, phase string, body []byte) {
	if account == nil || account.Platform != PlatformOpenAI {
		return
	}
	s := openAIRequestAuditStore()
	if s == nil {
		return
	}
	r := openAIRequestAuditMeta(ctx, c)
	r.AccountID = account.ID
	r.ParentAccountID = account.ParentAccountID
	r.ProxyID = account.ProxyID
	r.Phase = phase
	r.Model = gjson.GetBytes(body, "model").String()
	if err := s.Write(r, body); err != nil {
		reportOpenAIAuditError(err)
	} else {
		openAIAuditWritten.Add(1)
	}
}

func auditOpenAIHTTPRequest(req *http.Request, account *Account) {
	if account == nil || account.Platform != PlatformOpenAI || openAIRequestAuditStore() == nil {
		return
	}
	if req.GetBody == nil {
		auditOpenAIRequest(req.Context(), nil, account, "outbound_http_body_unavailable", nil)
		return
	}
	reader, err := req.GetBody()
	if err != nil {
		reportOpenAIAuditError(err)
		return
	}
	defer reader.Close()
	body, err := io.ReadAll(reader)
	if err != nil {
		reportOpenAIAuditError(err)
		return
	}
	if mediaType, params, err := mime.ParseMediaType(req.Header.Get("Content-Type")); err == nil && mediaType == "multipart/form-data" {
		var parts []map[string]any
		reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				reportOpenAIAuditError(err)
				break
			}
			data, err := io.ReadAll(part)
			part.Close()
			if err != nil {
				reportOpenAIAuditError(err)
				break
			}
			item := map[string]any{"name": part.FormName(), "bytes": len(data)}
			if part.FileName() != "" {
				item["filename"] = part.FileName()
				item["sha256"] = fmt.Sprintf("%x", sha256.Sum256(data))
				item["omitted"] = "binary_file"
			} else {
				item["fields"] = map[string]string{part.FormName(): string(data)}
			}
			parts = append(parts, item)
		}
		snapshot, _ := json.Marshal(map[string]any{"multipart_fields": parts, "original_body_sha256": fmt.Sprintf("%x", sha256.Sum256(body)), "original_body_bytes": len(body)})
		auditOpenAIRequest(req.Context(), nil, account, "outbound_http_multipart_fields", snapshot)
		return
	}
	auditOpenAIRequest(req.Context(), nil, account, "outbound_http_or_smartws", body)
}

// Capture headers already received; never consume or buffer a streaming response.
func auditOpenAIHTTPResult(req *http.Request, account *Account, response *http.Response, resultErr error) {
	if account == nil || account.Platform != PlatformOpenAI {
		return
	}
	s := openAIRequestAuditStore()
	if s == nil {
		return
	}
	r := openAIRequestAuditMeta(req.Context(), nil)
	r.AccountID = account.ID
	r.ParentAccountID = account.ParentAccountID
	r.Phase = "upstream_http_headers"
	r.TransportError = resultErr != nil
	if response != nil {
		r.HTTPStatus = response.StatusCode
		r.UpstreamRequestID = response.Header.Get("x-request-id")
	}
	if err := s.Write(r, nil); err != nil {
		reportOpenAIAuditError(err)
	} else {
		openAIAuditWritten.Add(1)
	}
}

func auditOpenAIWSResult(ctx context.Context, account *Account, requestID string, failed bool) {
	if account == nil || account.Platform != PlatformOpenAI {
		return
	}
	s := openAIRequestAuditStore()
	if s == nil {
		return
	}
	r := openAIRequestAuditMeta(ctx, nil)
	r.AccountID = account.ID
	r.ParentAccountID = account.ParentAccountID
	r.Phase = "upstream_ws_result"
	r.UpstreamRequestID = requestID
	r.TransportError = failed
	if err := s.Write(r, nil); err != nil {
		reportOpenAIAuditError(err)
	} else {
		openAIAuditWritten.Add(1)
	}
}
