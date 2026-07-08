//go:build unit

package handler

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func newObservedLogger(t *testing.T) (*zap.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zap.WarnLevel)
	return zap.New(core), logs
}

func loggedFields(t *testing.T, logs *observer.ObservedLogs) map[string]any {
	t.Helper()
	entries := logs.All()
	require.Len(t, entries, 1)
	fields := map[string]any{}
	for _, f := range entries[0].Context {
		switch f.Key {
		case "body_len":
			fields[f.Key] = int(f.Integer)
		case "json_error_offset":
			fields[f.Key] = f.Integer
		default:
			fields[f.Key] = f.String
		}
	}
	return fields
}

func TestLogRequestBodyParseFailure_DerivesErrorWhenNil(t *testing.T) {
	log, logs := newObservedLogger(t)
	body := []byte(`{"model": bad}`)

	logRequestBodyParseFailure(log, body, nil)

	fields := loggedFields(t, logs)
	require.Equal(t, len(body), fields["body_len"])
	require.Equal(t, service.InvalidJSONCategorySyntax, fields["json_error_category"])
	require.Equal(t, int64(11), fields["json_error_offset"])
}

func TestLogRequestBodyParseFailure_NoBodyBytesOrParserText(t *testing.T) {
	log, logs := newObservedLogger(t)
	secret := "sk-super-secret-value"
	field := "api_key"
	body := []byte(`{"` + field + `":` + secret + `}`)

	logRequestBodyParseFailure(log, body, nil)

	fields := loggedFields(t, logs)
	require.NotContains(t, fields, "error")
	require.NotContains(t, fields, "body_head")
	require.NotContains(t, fields, "body_tail")
	for _, value := range fields {
		text, ok := value.(string)
		if !ok {
			continue
		}
		require.NotContains(t, text, secret)
		require.NotContains(t, text, field)
		require.NotContains(t, text, "invalid character")
	}
}

func TestLogRequestBodyParseFailure_NilLoggerNoPanic(t *testing.T) {
	require.NotPanics(t, func() {
		logRequestBodyParseFailure(nil, []byte(`{`), nil)
	})
}
