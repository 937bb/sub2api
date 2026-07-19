package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompletedResponseSnapshotRestoresExactIndependentResponse(t *testing.T) {
	original := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header: http.Header{
			"X-Request-Id": {"request-1"},
			"Retry-After":  {"7"},
		},
		Body: io.NopCloser(strings.NewReader("ignored")),
	}
	body := []byte(`{"error":"bounded"}`)
	snapshot := snapshotCompletedResponse(original, body)

	original.Header.Set("X-Request-Id", "mutated")
	body[0] = 'x'
	first := snapshot.response()
	first.Header.Set("Retry-After", "mutated")
	got, err := io.ReadAll(first.Body)
	require.NoError(t, err)
	require.NoError(t, first.Body.Close())

	second := snapshot.response()
	secondBody, err := io.ReadAll(second.Body)
	require.NoError(t, err)
	require.NoError(t, second.Body.Close())
	require.Equal(t, http.StatusTooManyRequests, second.StatusCode)
	require.Equal(t, "request-1", second.Header.Get("X-Request-Id"))
	require.Equal(t, "7", second.Header.Get("Retry-After"))
	require.Equal(t, `{"error":"bounded"}`, string(got))
	require.Equal(t, got, secondBody)
}

func TestEmptyCompletedResponseSnapshotDoesNotMaskCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	restored, terminal := (completedResponseSnapshot{}).ifRetryCanceled(ctx)

	require.False(t, terminal)
	require.Nil(t, restored)
}

func TestCompletedResponseSnapshotCancellationIsTerminal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	snapshot := snapshotCompletedResponse(&http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"X-Request-Id": {"prior"}},
	}, []byte("prior body"))

	restored, terminal := snapshot.ifRetryCanceled(ctx)
	require.True(t, terminal)
	require.Equal(t, http.StatusBadRequest, restored.StatusCode)
	require.Equal(t, "prior", restored.Header.Get("X-Request-Id"))
	got, err := io.ReadAll(restored.Body)
	require.NoError(t, err)
	require.NoError(t, restored.Body.Close())
	require.Equal(t, "prior body", string(got))
}
