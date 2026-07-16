//go:build unit

package web

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsFingerprintedEmbeddedAssetPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "javascript", path: "assets/index-AbCd1234.js", want: true},
		{name: "css", path: "assets/app-a1B2c3D4.css", want: true},
		{name: "url safe hash", path: "assets/app-aB1-2_Cd.css", want: true},
		{name: "nested", path: "assets/vendor/chunk-AbCd1234.js", want: true},
		{name: "leading slash", path: "/assets/index-AbCd1234.js", want: true},
		{name: "unhashed", path: "assets/index.js"},
		{name: "short hash", path: "assets/index-abc123.js"},
		{name: "missing extension", path: "assets/index-AbCd1234"},
		{name: "invalid hash", path: "assets/index-AbCd12!4.js"},
		{name: "trailing slash", path: "assets/index-AbCd1234.js/"},
		{name: "double slash", path: "assets//index-AbCd1234.js"},
		{name: "dot segment", path: "assets/vendor/../index-AbCd1234.js"},
		{name: "backslash", path: `assets\index-AbCd1234.js`},
		{name: "outside assets", path: "downloads/index-AbCd1234.js"},
		{name: "similar prefix", path: "assets-v2/index-AbCd1234.js"},
		{name: "root mutable", path: "logo.png"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, isFingerprintedEmbeddedAssetPath(tt.path))
		})
	}
}

func TestApplyEmbeddedAssetCacheHeaders(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		path string
		want string
	}{
		{path: "assets/index-AbCd1234.js", want: immutableAssetCacheControl},
		{path: "assets/index.js", want: mutableAssetCacheControl},
		{path: "logo.png", want: mutableAssetCacheControl},
		{path: "index.html", want: mutableAssetCacheControl},
	} {
		t.Run(tt.path, func(t *testing.T) {
			header := make(http.Header)
			applyEmbeddedAssetCacheHeaders(header, tt.path)
			assert.Equal(t, tt.want, header.Get("Cache-Control"))
		})
	}

	assert.NotPanics(t, func() { applyEmbeddedAssetCacheHeaders(nil, "assets/index-AbCd1234.js") })
}
