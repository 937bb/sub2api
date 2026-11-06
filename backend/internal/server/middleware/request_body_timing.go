package middleware

import (
	"io"
	"net/http"
	"sync/atomic"
	"time"
)

// requestBodyTiming measures only Read calls; it never buffers or logs content.
type requestBodyTiming struct {
	io.ReadCloser
	readNanos atomic.Int64
	bytesRead atomic.Int64
}

func observeRequestBody(req *http.Request) *requestBodyTiming {
	if req == nil || req.Body == nil || req.Body == http.NoBody {
		return nil
	}
	body := &requestBodyTiming{ReadCloser: req.Body}
	req.Body = body
	return body
}

func (b *requestBodyTiming) Read(p []byte) (int, error) {
	started := time.Now()
	n, err := b.ReadCloser.Read(p)
	b.readNanos.Add(time.Since(started).Nanoseconds())
	b.bytesRead.Add(int64(n))
	return n, err
}
