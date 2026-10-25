package service

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/klauspost/compress/zstd"
	"github.com/tidwall/gjson"
)

const (
	codexSessionHeaderModeNative = "native"
	codexSessionHeaderModeLegacy = "legacy"
	codexOriginalPromptCacheKey  = "codex_original_prompt_cache_key"
)

func stageCodexOriginalPromptCacheKey(c *gin.Context, value string) {
	if c != nil {
		c.Set(codexOriginalPromptCacheKey, strings.TrimSpace(value))
	}
}

func codexSessionHeaderMode() string {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("CODEX_SESSION_HEADER_MODE")), codexSessionHeaderModeLegacy) {
		return codexSessionHeaderModeLegacy
	}
	return codexSessionHeaderModeNative
}

// applyCodexNativeSessionHeaders projects the resolved request identity onto
// the same three headers emitted by the official Codex HTTP client.
func applyCodexNativeSessionHeaders(headers http.Header, body []byte) {
	if headers == nil {
		return
	}
	sessionID := firstNonEmptyCodexHeader(headers, "Session_id", "Session-Id")
	if sessionID == "" {
		sessionID = strings.TrimSpace(gjson.GetBytes(body, "prompt_cache_key").String())
	}
	if sessionID == "" {
		return
	}
	if codexSessionHeaderMode() == codexSessionHeaderModeLegacy {
		headers.Set("Session_id", sessionID)
		headers.Del("Session-Id")
		headers.Del("Thread-Id")
		headers.Del("X-Client-Request-Id")
		return
	}

	threadID := firstNonEmptyCodexHeader(headers, "Thread-Id", "Conversation_id", "X-Client-Request-Id")
	if threadID == "" {
		metadata := strings.TrimSpace(headers.Get(openAIWSTurnMetadataHeader))
		if gjson.Valid(metadata) {
			threadID = strings.TrimSpace(gjson.Get(metadata, "thread_id").String())
		}
	}
	if threadID == "" {
		threadID = sessionID
	}

	headers.Set("Session-Id", sessionID)
	headers.Set("Thread-Id", threadID)
	headers.Set("X-Client-Request-Id", threadID)
	headers.Del("Session_id")
	headers.Del("Conversation_id")
}

func firstNonEmptyCodexHeader(headers http.Header, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(headers.Get(name)); value != "" {
			return value
		}
	}
	return ""
}

func codexRequestCompressionEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("CODEX_REQUEST_COMPRESSION"))) {
	case "off", "none", "false", "0", "plain":
		return false
	default:
		return true
	}
}

var (
	codexZstdEncoderOnce sync.Once
	codexZstdEncoder     *zstd.Encoder
	codexZstdEncoderErr  error
)

func sharedCodexZstdEncoder() (*zstd.Encoder, error) {
	codexZstdEncoderOnce.Do(func() {
		codexZstdEncoder, codexZstdEncoderErr = zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
	})
	return codexZstdEncoder, codexZstdEncoderErr
}

// compressCodexHTTPRequest mirrors the official client: only the ChatGPT HTTP
// /responses endpoint is zstd-compressed. Compact and WebSocket payloads keep
// their original framing.
func CompressOpenAICodexHTTPRequest(request *http.Request) error {
	if request == nil || request.Body == nil || !codexRequestCompressionEnabled() {
		return nil
	}
	if request.URL == nil || !strings.EqualFold(strings.TrimSpace(request.URL.Hostname()), "chatgpt.com") ||
		!strings.HasSuffix(strings.TrimRight(request.URL.Path, "/"), "/responses") {
		return nil
	}
	if strings.TrimSpace(request.Header.Get("Content-Encoding")) != "" {
		return nil
	}
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return fmt.Errorf("read request body: %w", err)
	}
	if err := request.Body.Close(); err != nil {
		return fmt.Errorf("close request body: %w", err)
	}
	if len(body) == 0 {
		request.Body = io.NopCloser(bytes.NewReader(nil))
		request.ContentLength = 0
		return nil
	}
	encoder, err := sharedCodexZstdEncoder()
	if err != nil {
		return fmt.Errorf("create zstd encoder: %w", err)
	}
	encoded := encoder.EncodeAll(body, nil)
	request.Body = io.NopCloser(bytes.NewReader(encoded))
	request.ContentLength = int64(len(encoded))
	saved := append([]byte(nil), encoded...)
	request.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(saved)), nil
	}
	request.Header.Set("Content-Encoding", "zstd")
	return nil
}
