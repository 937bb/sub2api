package httputil

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"errors"
	"net/http"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func TestNormalizeLenientJSONRequestBody(t *testing.T) {
	valid := []byte(`{"value":"valid\\njson"}`)
	got, err := NormalizeLenientJSONRequestBody(valid, 1024)
	if err != nil || &got[0] != &valid[0] {
		t.Fatalf("valid JSON must be zero-copy: got %q, err %v", got, err)
	}

	tests := []struct {
		name    string
		body    string
		want    string
		wantErr error
	}{
		{"even slashes permit repair", "{\"v\":\"\\\\\x01\"}", "{\"v\":\"\\\\\\u0001\"}", nil},
		{"odd slash leaves escape pending", "{\"v\":\"\\\x01\"}", "", errRawJSONControlAfterEscape},
		{"three slashes leave escape pending", "{\"v\":\"\\\\\\\x01\"}", "", errRawJSONControlAfterEscape},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeLenientJSONRequestBody([]byte(tt.body), 1024)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if err == nil && string(got) != tt.want {
				t.Fatalf("body = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReadLenientJSONRequestBodyCompressedLimit(t *testing.T) {
	payload := []byte(`{"value":"0123456789"}`)
	encoders := map[string]func(*bytes.Buffer) error{
		"gzip": func(dst *bytes.Buffer) error {
			w := gzip.NewWriter(dst)
			_, _ = w.Write(payload)
			return w.Close()
		},
		"deflate": func(dst *bytes.Buffer) error {
			w := zlib.NewWriter(dst)
			_, _ = w.Write(payload)
			return w.Close()
		},
		"zstd": func(dst *bytes.Buffer) error {
			w, err := zstd.NewWriter(dst)
			if err != nil {
				return err
			}
			_, _ = w.Write(payload)
			return w.Close()
		},
	}
	for encoding, encode := range encoders {
		t.Run(encoding, func(t *testing.T) {
			var compressed bytes.Buffer
			if err := encode(&compressed); err != nil {
				t.Fatal(err)
			}
			req, err := http.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(compressed.Bytes()))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Encoding", encoding)
			_, err = ReadLenientJSONRequestBodyWithPrealloc(req, int64(len(payload)-1))
			var maxErr *http.MaxBytesError
			if !errors.As(err, &maxErr) || maxErr.Limit != int64(len(payload)-1) {
				t.Fatalf("error = %#v, want MaxBytesError limit %d", err, len(payload)-1)
			}
		})
	}
}
