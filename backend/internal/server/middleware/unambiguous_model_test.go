package middleware

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/gin-gonic/gin"
)

func TestUnambiguousModelAdmission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, compressed := range []bool{false, true} {
		for _, tc := range []struct {
			name, body string
			status     int
		}{
			{"reject", `{"model":"gpt-6-astra","model":"gpt-6.1-sol","messages":[]}`, 400},
			{"preserve", `{ "model": "gpt-6-astra", "messages": [{"role":"user","content":"hi"}] }`, 200},
			{"bom_reject", "\xef\xbb\xbf" + `{"model":"a","model":"b"}`, 400},
			{"nested_content", `{"model":"x","metadata":{"model":"a","model":"b"}}`, 200},
		} {
			t.Run(tc.name+map[bool]string{false: "_plain", true: "_gzip"}[compressed], func(t *testing.T) {
				router := gin.New()
				router.Use(UnambiguousModel())
				called := false
				router.POST("/v1/responses", func(c *gin.Context) {
					called = true
					got, err := httputil.ReadRequestBodyWithPrealloc(c.Request)
					if err != nil || string(got) != tc.body {
						t.Errorf("body changed: %q, %v", got, err)
					}
					c.Status(200)
				})
				var payload io.Reader = strings.NewReader(tc.body)
				if compressed {
					var buf bytes.Buffer
					gz := gzip.NewWriter(&buf)
					_, _ = gz.Write([]byte(tc.body))
					_ = gz.Close()
					payload = &buf
				}
				req := httptest.NewRequest("POST", "/v1/responses", payload)
				req.Header.Set("Content-Type", "application/json")
				if compressed {
					req.Header.Set("Content-Encoding", "gzip")
				}
				res := httptest.NewRecorder()
				router.ServeHTTP(res, req)
				if res.Code != tc.status || called != (tc.status == 200) {
					t.Fatalf("status=%d called=%v body=%s", res.Code, called, res.Body)
				}
				if tc.status == 400 && !strings.Contains(res.Body.String(), "ambiguous_model") {
					t.Fatal(res.Body)
				}
			})
		}
	}
}

func TestUnambiguousModelBodyLimitAndGET(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 10); c.Next() }, UnambiguousModel())
	r.POST("/v1/responses", func(c *gin.Context) { t.Fatal("oversized body reached handler") })
	r.GET("/v1/responses", func(c *gin.Context) { c.Status(204) })
	res := httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"over_limit"}`)))
	if res.Code != 413 {
		t.Fatalf("status=%d", res.Code)
	}
	res = httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequest("GET", "/v1/responses", nil))
	if res.Code != 204 {
		t.Fatalf("GET status=%d", res.Code)
	}
}
