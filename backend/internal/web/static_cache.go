//go:build embed || unit

package web

import (
	"net/http"
	"path"
	"strings"
)

const (
	immutableAssetCacheControl = "public, max-age=31536000, immutable"
	mutableAssetCacheControl   = "no-cache"
)

// isFingerprintedEmbeddedAssetPath recognizes Vite's default eight-character
// content hash only within the assets tree.
func isFingerprintedEmbeddedAssetPath(cleanPath string) bool {
	cleanPath = strings.TrimPrefix(cleanPath, "/")
	if cleanPath == "" || cleanPath != path.Clean(cleanPath) || strings.Contains(cleanPath, "\\") || !strings.HasPrefix(cleanPath, "assets/") {
		return false
	}

	filename := path.Base(cleanPath)
	extension := path.Ext(filename)
	stem := strings.TrimSuffix(filename, extension)
	const fingerprintLength = 8
	delimiter := len(stem) - fingerprintLength - 1
	if extension == "" || delimiter < 1 || stem[delimiter] != '-' {
		return false
	}

	for _, char := range stem[delimiter+1:] {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '_' || char == '-' {
			continue
		}
		return false
	}
	return true
}

func applyEmbeddedAssetCacheHeaders(header http.Header, cleanPath string) {
	if header == nil {
		return
	}
	if isFingerprintedEmbeddedAssetPath(cleanPath) {
		header.Set("Cache-Control", immutableAssetCacheControl)
		return
	}
	header.Set("Cache-Control", mutableAssetCacheControl)
}
