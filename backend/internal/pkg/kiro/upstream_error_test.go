package kiro

import (
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestRefreshErrorsExposeOnlyAllowlistedReasons(t *testing.T) {
	for _, tc := range []struct {
		status       int
		body, reason string
	}{
		{400, `{"error":"invalid_grant","error_description":"private-refresh-token"}`, "invalid_grant"},
		{401, `{"message":"private-access-token"}`, "unauthorized"},
		{403, `{"message":"private-proxy-password"}`, "forbidden"},
		{429, `private-refresh-token`, "rate_limited"},
		{400, `{"error":"private-refresh-token"}`, ""},
		{400, strings.Repeat(" ", 32<<10) + `{"error":"invalid_grant"}`, ""},
	} {
		err := ResponseError(OperationRefresh, &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))})
		require.Equal(t, tc.reason, err.Reason)
		require.NotContains(t, err.Error(), "private-")
		require.Contains(t, err.Error(), "credential refresh HTTP")
	}
	err := TransportError(OperationModels, &url.Error{Op: "Post", URL: "https://secret@private.invalid", Err: context.DeadlineExceeded})
	require.Equal(t, "timeout", err.Reason)
	require.NotContains(t, err.Error(), "private.invalid")
	require.Equal(t, "canceled", TransportError(OperationRefresh, context.Canceled).Reason)
	require.Equal(t, "transport", TransportError(OperationRefresh, errors.New("secret")).Reason)
}
