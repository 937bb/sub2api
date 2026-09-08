package service

import (
	"bytes"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Stage only error responses. Successful streaming bytes are never retained.
type oauthRetryWriter struct {
	gin.ResponseWriter
	status int
	staged bool
	body   bytes.Buffer
}

func (w *oauthRetryWriter) WriteHeader(code int) {
	if !w.ResponseWriter.Written() && code >= 400 {
		w.status = code
		return
	}
	w.ResponseWriter.WriteHeader(code)
}
func (w *oauthRetryWriter) Status() int {
	if w.status >= 400 {
		return w.status
	}
	return w.ResponseWriter.Status()
}
func (w *oauthRetryWriter) Written() bool { return w.staged || w.ResponseWriter.Written() }
func (w *oauthRetryWriter) Size() int {
	if w.staged {
		return w.body.Len()
	}
	return w.ResponseWriter.Size()
}
func (w *oauthRetryWriter) WriteHeaderNow() {
	if w.status >= 400 && !w.ResponseWriter.Written() {
		w.staged = true
		return
	}
	w.ResponseWriter.WriteHeaderNow()
}
func (w *oauthRetryWriter) Write(p []byte) (int, error) {
	if w.status >= 400 && !w.ResponseWriter.Written() {
		w.staged = true
		if w.body.Len()+len(p) <= 64*1024 {
			return w.body.Write(p)
		}
		if err := w.commit(); err != nil {
			return 0, err
		}
	}
	return w.ResponseWriter.Write(p)
}
func (w *oauthRetryWriter) WriteString(p string) (int, error) { return w.Write([]byte(p)) }
func (w *oauthRetryWriter) Flush() {
	if w.status >= 400 && !w.ResponseWriter.Written() {
		w.staged = true
		return
	}
	w.ResponseWriter.Flush()
}
func (w *oauthRetryWriter) commit() error {
	if w.status < http.StatusBadRequest || w.ResponseWriter.Written() {
		return nil
	}
	w.ResponseWriter.WriteHeader(w.status)
	w.ResponseWriter.WriteHeaderNow()
	_, err := w.ResponseWriter.Write(w.body.Bytes())
	w.body.Reset()
	return err
}
