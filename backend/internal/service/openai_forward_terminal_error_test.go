package service

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSmartWSTerminalErrorSurvivesWrappingWithoutReplay(t *testing.T) {
	for _, message := range []string{
		"non-streaming openai protocol error: unsupported MIME type image/mpo",
		"upstream response failed: Encrypted function output content could not be decrypted or decoded.",
	} {
		t.Run(message, func(t *testing.T) {
			attempt := &groupSmartWSAttempt{}
			attempt.sent.Store(true)
			err := finishGroupSmartWSAttempt(attempt, errors.New(message))
			var transport *PluginTransportError
			require.ErrorAs(t, err, &transport)
			require.True(t, transport.RequestSent)
			require.True(t, OpenAIForwardErrorIsTerminal(err))
			require.True(t, OpenAIForwardErrorIsTerminal(fmt.Errorf("forward: %w", err)))
			require.Same(t, err, finishGroupSmartWSAttempt(attempt, err))
			require.Nil(t, errors.Unwrap(err))
		})
	}
	attempt := &groupSmartWSAttempt{}
	attempt.sent.Store(true)
	for _, cause := range []error{errors.New("stream read error: unexpected EOF"), &UpstreamFailoverError{StatusCode: 429}} {
		err := finishGroupSmartWSAttempt(attempt, cause)
		require.False(t, OpenAIForwardErrorIsTerminal(err))
		var failover *UpstreamFailoverError
		require.False(t, errors.As(err, &failover))
	}
	require.False(t, OpenAIForwardErrorIsTerminal(nil))
}

func TestNonStreamingProtocolErrorPreservesInputDetails(t *testing.T) {
	for _, payload := range []string{
		`{"error":{"type":"invalid_request_error","code":"invalid_value","param":"input[0].content[9].file_data","message":"unsupported MIME type image/mpo"}}`,
		`{"type":"response.failed","response":{"error":{"type":"invalid_request_error","code":"invalid_value","param":"input[0].content[9].file_data","message":"unsupported MIME type image/mpo"}}}`,
		`{"type":"error","code":"invalid_value","param":"input[0].content[9].file_data","message":"unsupported MIME type image/mpo"}`,
	} {
		t.Run(payload, func(t *testing.T) {
			c, rec := newNonStreamingFailoverContext(t)
			svc := newNonStreamingFailoverService()
			err := svc.writeOpenAINonStreamingProtocolError(newNonStreamingSSEResponse(), c, []byte(payload), "unsupported MIME type image/mpo")
			require.Error(t, err)
			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.True(t, gjson.ValidBytes(rec.Body.Bytes()))
			require.Equal(t, "invalid_request_error", gjson.GetBytes(rec.Body.Bytes(), "error.type").String())
			require.Equal(t, "invalid_value", gjson.GetBytes(rec.Body.Bytes(), "error.code").String())
			require.Equal(t, "input[0].content[9].file_data", gjson.GetBytes(rec.Body.Bytes(), "error.param").String())
			require.True(t, IsResponseCommitted(c))
			attempt := &groupSmartWSAttempt{}
			attempt.sent.Store(true)
			require.True(t, OpenAIForwardErrorIsTerminal(finishGroupSmartWSAttempt(attempt, err)))
		})
	}
}

func TestTopLevelStreamErrorStatusPreservesFailureClass(t *testing.T) {
	for _, tc := range []struct {
		code   string
		status int
	}{
		{"invalid_value", http.StatusBadRequest},
		{"rate_limit_exceeded", http.StatusTooManyRequests},
		{"server_is_overloaded", http.StatusServiceUnavailable},
		{"unknown_failure", http.StatusBadGateway},
	} {
		t.Run(tc.code, func(t *testing.T) {
			payload := []byte(fmt.Sprintf(`{"type":"error","code":%q,"message":"request failed"}`, tc.code))
			require.Equal(t, tc.status, openAIStreamFailureStatus(payload, "request failed"))
		})
	}
}
