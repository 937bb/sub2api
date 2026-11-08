package httputil

import (
	"bufio"
	"io"
	"net"
	"net/http"
)

// WithRejectedBodyDrain consumes bounded HTTP/1 uploads before an early error.
// Otherwise net/http can close with unread socket data while an unbuffered
// reverse proxy is still writing, replacing the application's error with a 502.
// Install inside MaxBytesHandler and inside WithBodyReadTimeout so the discard
// retains the same byte and socket-read limits as a normal request body read.
func WithRejectedBodyDrain(next http.Handler, maxBytes int64) http.Handler {
	if maxBytes <= 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor != 1 || r.Body == nil || r.Body == http.NoBody {
			next.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(&rejectionDrainWriter{ResponseWriter: w, request: r, maxBytes: maxBytes}, r)
	})
}

type rejectionDrainWriter struct {
	http.ResponseWriter
	request  *http.Request
	maxBytes int64
	wrote    bool
}

func (w *rejectionDrainWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *rejectionDrainWriter) WriteHeader(status int) {
	if !w.wrote && status >= 200 {
		w.wrote = true
		if status >= 400 && w.request.ContentLength <= w.maxBytes {
			// LimitReader is a second bound for callers outside MaxBytesHandler.
			// Never retain rejected content or replace the original error status.
			_, _ = io.Copy(io.Discard, io.LimitReader(w.request.Body, w.maxBytes))
		}
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *rejectionDrainWriter) Write(p []byte) (int, error) {
	w.wrote = true
	return w.ResponseWriter.Write(p)
}

func (w *rejectionDrainWriter) FlushError() error {
	w.wrote = true
	return http.NewResponseController(w.ResponseWriter).Flush()
}

func (w *rejectionDrainWriter) Flush() { _ = w.FlushError() }

func (w *rejectionDrainWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return http.NewResponseController(w.ResponseWriter).Hijack()
}

func (w *rejectionDrainWriter) CloseNotify() <-chan bool {
	if notifier, ok := w.ResponseWriter.(http.CloseNotifier); ok { //nolint:staticcheck // Gin still exposes the legacy interface.
		return notifier.CloseNotify()
	}
	return make(chan bool)
}
