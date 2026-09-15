package service

import (
	"bytes"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Stage only error responses. Successful streaming bytes are never retained.
type oauthRetryWriter struct {
	gin.ResponseWriter
	context              *gin.Context
	status               int
	staged               bool
	body                 bytes.Buffer
	streamErrorCommitted bool
	onlyAfterHeartbeat   bool
}

func (w *oauthRetryWriter) canStageError() bool {
	if w.onlyAfterHeartbeat && !w.ResponseWriter.Written() {
		return false
	}
	return !openAIStreamWriterHasCommittedOutput(w.context, w.ResponseWriter)
}

func (w *oauthRetryWriter) WriteHeader(code int) {
	if w.canStageError() && code >= 400 {
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
		// Include physical heartbeat bytes so adjusted-size consumers can
		// subtract them without accidentally subtracting the staged error.
		return max(0, w.ResponseWriter.Size()) + w.body.Len()
	}
	return w.ResponseWriter.Size()
}
func (w *oauthRetryWriter) WriteHeaderNow() {
	if w.status >= 400 && w.canStageError() {
		w.staged = true
		return
	}
	w.ResponseWriter.WriteHeaderNow()
}
func (w *oauthRetryWriter) Write(p []byte) (int, error) {
	if w.streamErrorCommitted {
		return len(p), nil
	}
	if w.status >= 400 && w.canStageError() {
		w.staged = true
		if w.body.Len()+len(p) <= 64*1024 {
			return w.body.Write(p)
		}
		if err := w.commit(); err != nil {
			return 0, err
		}
		if w.streamErrorCommitted {
			return len(p), nil
		}
	}
	return w.ResponseWriter.Write(p)
}
func (w *oauthRetryWriter) WriteString(p string) (int, error) { return w.Write([]byte(p)) }
func (w *oauthRetryWriter) Flush() {
	if w.status >= 400 && w.canStageError() {
		w.staged = true
		return
	}
	w.ResponseWriter.Flush()
}
func (w *oauthRetryWriter) commit() error {
	if w.status < http.StatusBadRequest || w.streamErrorCommitted {
		return nil
	}
	if w.ResponseWriter.Written() {
		if !w.staged || !w.canStageError() {
			return nil
		}
		// HTTP status is fixed at 200 by the keepalive. Never append a naked
		// JSON HTTP error into this SSE body or expose an intermediate attempt.
		w.streamErrorCommitted = true
		err := writeOpenAIErrorAfterHeartbeat(w.context, w.ResponseWriter, w.status, w.body.Bytes())
		w.body.Reset()
		return err
	}
	w.ResponseWriter.WriteHeader(w.status)
	w.ResponseWriter.WriteHeaderNow()
	_, err := w.ResponseWriter.Write(w.body.Bytes())
	w.body.Reset()
	return err
}
