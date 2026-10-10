package middleware

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestExcelBPSImageAdmissionSaturatedImagesAllowText(t *testing.T) {
	const text = `{"model":"gpt-6-astra","input":"hello"}`
	const image = `{"input":[{"content":[{"type":"input_image","image_url":"data:image/png;base64,AAAA"}]}]}`
	var compressed bytes.Buffer
	zipper := gzip.NewWriter(&compressed)
	_, err := zipper.Write([]byte(text))
	require.NoError(t, err)
	require.NoError(t, zipper.Close())
	for _, tt := range []struct {
		name, encoding string
		body           []byte
		length         int64
	}{
		{"known length", "", []byte(text), int64(len(text))},
		{"chunked", "", []byte(text), -1},
		{"gzip", "gzip", compressed.Bytes(), int64(compressed.Len())},
	} {
		t.Run(tt.name, func(t *testing.T) {
			entered := make(chan struct{})
			release := make(chan struct{})
			done := make(chan struct{})
			var once sync.Once
			t.Cleanup(func() { once.Do(func() { close(release) }); <-done })
			r := bpsImageTestRouter(bpsImageTestSettings{enabled: true, maxRequests: 1, budgetMiB: 512}, func(c *gin.Context) {
				if c.GetHeader("Hold") == "true" {
					close(entered)
					<-release
				} else {
					body, readErr := io.ReadAll(c.Request.Body)
					require.NoError(t, readErr)
					require.JSONEq(t, text, string(body))
				}
				c.Status(http.StatusNoContent)
			})
			go func() {
				defer close(done)
				first := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(image))
				first.Header.Set("Hold", "true")
				r.ServeHTTP(httptest.NewRecorder(), first)
			}()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("image request did not reach upstream handler")
			}
			for _, path := range []string{"/responses", "/v1/responses", "/v1/chat/completions", "/v1/messages"} {
				request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(tt.body))
				request.ContentLength = tt.length
				request.Header.Set("Content-Encoding", tt.encoding)
				response := httptest.NewRecorder()
				r.ServeHTTP(response, request)
				require.Equal(t, http.StatusNoContent, response.Code, response.Body.String())
			}
			response := httptest.NewRecorder()
			r.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(image)))
			require.Equal(t, http.StatusServiceUnavailable, response.Code)
			require.Contains(t, response.Body.String(), "basispoints_image_request_busy")
		})
	}
}

type imageAdmissionNotifyingReader struct {
	io.ReadCloser
	started chan struct{}
	once    sync.Once
}

func (r *imageAdmissionNotifyingReader) Read(p []byte) (int, error) {
	r.once.Do(func() { close(r.started) })
	return r.ReadCloser.Read(p)
}

func TestExcelBPSImageAdmissionBoundsConcurrentBodyReads(t *testing.T) {
	const body = `{"input":"hello"}`
	reader, writer := io.Pipe()
	started := make(chan struct{})
	done := make(chan struct{})
	t.Cleanup(func() { _ = reader.Close(); _ = writer.Close(); <-done })
	r := bpsImageTestRouter(bpsImageTestSettings{enabled: true, maxRequests: 1}, func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	go func() {
		defer close(done)
		request := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		request.Body = &imageAdmissionNotifyingReader{ReadCloser: reader, started: started}
		request.ContentLength = int64(len(body))
		r.ServeHTTP(httptest.NewRecorder(), request)
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("request did not start reading")
	}
	response := httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body)))
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.Contains(t, response.Body.String(), "request_body_capacity_busy")
	require.NotContains(t, response.Body.String(), "basispoints_image_request_busy")
	_, err := io.WriteString(writer, body)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("body reader did not finish")
	}
	response = httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body)))
	require.Equal(t, http.StatusNoContent, response.Code, response.Body.String())
}
