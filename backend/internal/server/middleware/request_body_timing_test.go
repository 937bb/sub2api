package middleware

import (
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

type timedBodyFixture struct {
	closed bool
	err    error
}

func (b *timedBodyFixture) Read(p []byte) (int, error) { return copy(p, "hi"), b.err }
func (b *timedBodyFixture) Close() error               { b.closed = true; return b.err }

func TestRequestBodyTimingPreservesPartialReadAndCloseErrors(t *testing.T) {
	want := errors.New("read interrupted")
	body := &timedBodyFixture{err: want}
	req := &http.Request{Body: body, ContentLength: 2}
	metric := observeRequestBody(req)
	buf := make([]byte, 8)
	n, err := req.Body.Read(buf)
	require.Equal(t, 2, n)
	require.Equal(t, "hi", string(buf[:n]))
	require.ErrorIs(t, err, want)
	require.EqualValues(t, 2, metric.bytesRead.Load())
	require.GreaterOrEqual(t, metric.readNanos.Load(), int64(0))
	require.EqualValues(t, 2, req.ContentLength)
	require.ErrorIs(t, req.Body.Close(), want)
	require.True(t, body.closed)
}

func TestRequestBodyTimingSkipsEmptyBody(t *testing.T) {
	require.Nil(t, observeRequestBody(nil))
	require.Nil(t, observeRequestBody(&http.Request{}))
	req := &http.Request{Body: http.NoBody}
	require.Nil(t, observeRequestBody(req))
	_, err := req.Body.Read(make([]byte, 1))
	require.ErrorIs(t, err, io.EOF)
}
