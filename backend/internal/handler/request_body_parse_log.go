package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"go.uber.org/zap"
)

// logRequestBodyParseFailure records sanitized diagnostics for a request body
// parse failure. It never logs parser strings, body snippets, or field names.
//
// err may be nil for call sites that validate with gjson.ValidBytes directly;
// the diagnostic error is derived from the body in that case.
func logRequestBodyParseFailure(reqLog *zap.Logger, body []byte, err error) {
	if reqLog == nil {
		return
	}
	diag, ok := err.(service.InvalidJSONDiagnostic)
	if !ok {
		diag = service.NewInvalidJSONDiagnostic(body)
	}

	fields := []zap.Field{
		zap.Int("body_len", diag.Length),
		zap.String("json_error_category", diag.Category),
	}
	if diag.Offset > 0 {
		fields = append(fields, zap.Int64("json_error_offset", diag.Offset))
	}

	reqLog.Warn("parse request body failed", fields...)
}
