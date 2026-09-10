package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIUsageOutcomeDistinguishesZeroUsageAndUnsuccessfulWork(t *testing.T) {
	for _, tc := range []struct {
		name        string
		result      OpenAIForwardResult
		err         error
		cyber, want bool
	}{
		{name: "completed_zero", result: OpenAIForwardResult{OpenAIWSMode: true, UpstreamTerminalEvent: "response.completed"}, want: true},
		{name: "legacy_zero", want: true},
		{name: "failed_zero", result: OpenAIForwardResult{OpenAIWSMode: true, UpstreamTerminalEvent: "response.failed"}},
		{name: "incomplete_zero", result: OpenAIForwardResult{OpenAIWSMode: true, UpstreamTerminalEvent: "response.incomplete"}},
		{name: "cancelled_zero", result: OpenAIForwardResult{ClientDisconnect: true}},
		{name: "error_zero", err: errors.New("connection lost")},
		{name: "partial_tokens", result: OpenAIForwardResult{ClientDisconnect: true, Usage: OpenAIUsage{InputTokens: 5}}, err: context.Canceled, want: true},
		{name: "cache_tokens", result: OpenAIForwardResult{ClientDisconnect: true, Usage: OpenAIUsage{CacheReadInputTokens: 5}}, err: context.Canceled, want: true},
		{name: "image", result: OpenAIForwardResult{ImageCount: 1}, err: context.Canceled, want: true},
		{name: "video", result: OpenAIForwardResult{VideoCount: 1}, err: context.Canceled, want: true},
		{name: "search", result: OpenAIForwardResult{WebSearchCalls: 1}, err: context.Canceled, want: true},
		{name: "audio", result: OpenAIForwardResult{AudioUsage: &AudioUsage{DurationOrUnits: 1}}, err: context.Canceled, want: true},
		{name: "cyber_zero", cyber: true, err: errors.New("blocked"), want: true},
	} {
		t.Run(tc.name, func(t *testing.T) { require.Equal(t, tc.want, ShouldRecordOpenAIUsage(&tc.result, tc.err, tc.cyber)) })
	}
}

func TestOpenAICancelledWSResultDoesNotBecomeZeroUsageOrRecoveredSuccess(t *testing.T) {
	svc, account, c, _, dialer := newWSMappedRetryFixture(t, OAuthRetrySettings{Enabled: true, MaxRetries: 5, StatusCodes: []int{502}}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.Request = c.Request.WithContext(ctx)
	dialer.onRead = cancel
	SetOpsUpstreamError(c, 502, "prior hidden overload", "")
	result, err := svc.Forward(ctx, c, account, []byte(`{"model":"gpt-6-astra","stream":true,"input":"hello"}`))
	require.Error(t, err)
	require.NotNil(t, result)
	require.False(t, ShouldRecordOpenAIUsage(result, err, false))
	MarkOpenAIForwardTerminalFailure(c, result, err)
	marked, ok := GetOpsStreamError(c)
	require.True(t, ok)
	require.Equal(t, 499, marked.IntendedStatus)
	require.Equal(t, "client_disconnected", marked.Code)
	require.Contains(t, marked.Message, "context canceled")
}

func TestOpenAIUsageOutcomeKeepsSpecificUpstreamFailure(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/responses", nil)
	MarkOpsStreamFailure(c, "service_unavailable_error", "server_is_overloaded", wsRetryOverloadMessage, 503)
	MarkOpenAIForwardTerminalFailure(c, &OpenAIForwardResult{OpenAIWSMode: true, UpstreamTerminalEvent: "response.failed"}, nil)
	marked, ok := GetOpsStreamError(c)
	require.True(t, ok)
	require.Equal(t, wsRetryOverloadMessage, marked.Message)
	require.Equal(t, 503, marked.IntendedStatus)
}
