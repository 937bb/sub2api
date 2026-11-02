package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// BeginHTTPUpstreamTrace records each gateway HTTP attempt independently of
// successful usage billing. It does not read request bodies or change headers.
// Background probes without a gateway correlation ID do not emit trace events.
func BeginHTTPUpstreamTrace(req *http.Request, accountID int64) func(*http.Response, error) {
	if req == nil {
		return func(*http.Response, error) {}
	}
	ctx := req.Context()
	clientID, _ := ctx.Value(ctxkey.ClientRequestID).(string)
	requestID, _ := ctx.Value(ctxkey.RequestID).(string)
	if clientID == "" && requestID == "" {
		return func(*http.Response, error) {}
	}
	started := time.Now()
	fields := []zap.Field{
		zap.String("component", "upstream_trace"),
		zap.Bool(logger.OpsSystemLogSkipField, true),
		zap.String("attempt_id", uuid.NewString()),
		zap.String("request_id", requestID),
		zap.String("client_request_id", clientID),
		zap.Int64("account_id", accountID),
		zap.String("method", req.Method),
	}
	if clientID != "" {
		fields = append(fields, zap.String("usage_request_id", "client:"+clientID))
	}
	if userID, ok := ctx.Value(ctxkey.UserID).(int64); ok {
		fields = append(fields, zap.Int64("user_id", userID))
	}
	if model, ok := ctx.Value(ctxkey.Model).(string); ok {
		fields = append(fields, zap.String("model", model))
	}
	// Hostname is sufficient for routing diagnostics; paths and queries can
	// contain customer data or credentials and are deliberately excluded.
	if req.URL != nil {
		fields = append(fields, zap.String("upstream_host", req.URL.Hostname()))
	}
	log := logger.FromContext(ctx).With(fields...)
	log.Info("upstream_attempt_started")
	return func(resp *http.Response, err error) {
		elapsed := zap.Int64("elapsed_ms", time.Since(started).Milliseconds())
		if err != nil {
			// Raw transport errors may include credentials in URLs.
			outcome := "transport_error"
			if errors.Is(err, context.Canceled) {
				outcome = "canceled"
			} else if errors.Is(err, context.DeadlineExceeded) {
				outcome = "deadline_exceeded"
			}
			log.Info("upstream_attempt_finished", elapsed, zap.String("outcome", outcome))
			return
		}
		if resp == nil {
			log.Info("upstream_attempt_finished", elapsed, zap.String("outcome", "no_response"))
			return
		}
		header, id := ResolveUpstreamRequestID(nil, resp.Header)
		if len(id) > maxUsageUpstreamRequestIDLen {
			id = id[:maxUsageUpstreamRequestIDLen]
		}
		log = log.With(zap.Int("status", resp.StatusCode), zap.String("upstream_request_id", id), zap.String("upstream_request_id_header", header))
		log.Info("upstream_attempt_headers", elapsed)
		if resp.Body == nil {
			log.Info("upstream_attempt_finished", elapsed, zap.String("outcome", "no_body"))
			return
		}
		resp.Body = &upstreamTraceBody{ReadCloser: resp.Body, finish: func(outcome string) {
			log.Info("upstream_attempt_finished", zap.Int64("elapsed_ms", time.Since(started).Milliseconds()), zap.String("outcome", outcome))
		}}
	}
}

// EOF describes transport completion, not an application-level SSE success.
// A body closed after an SSE terminal event can correctly report closed_early.
// Close still reaches the underlying body even after EOF or a read error.
type upstreamTraceBody struct {
	io.ReadCloser
	finish func(string)
	once   sync.Once
}

func (b *upstreamTraceBody) complete(outcome string) {
	b.once.Do(func() { b.finish(outcome) })
}

func (b *upstreamTraceBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if errors.Is(err, io.EOF) {
		b.complete("eof")
	} else if err != nil {
		outcome := "body_read_error"
		if errors.Is(err, context.Canceled) {
			outcome = "canceled"
		}
		b.complete(outcome)
	}
	return n, err
}

func (b *upstreamTraceBody) Close() error {
	err := b.ReadCloser.Close()
	outcome := "closed_early"
	if err != nil {
		outcome = "close_error"
	}
	b.complete(outcome)
	return err
}
