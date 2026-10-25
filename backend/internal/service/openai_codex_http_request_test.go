package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"
)

func TestApplyCodexNativeSessionHeaders(t *testing.T) {
	headers := make(http.Header)
	headers.Set("Session_id", "session-v7")
	headers.Set("Conversation_id", "child-thread-v7")
	headers.Set("X-Client-Request-Id", "stale-request")

	applyCodexNativeSessionHeaders(headers, nil)

	require.Equal(t, "session-v7", headers.Get("Session-Id"))
	require.Equal(t, "child-thread-v7", headers.Get("Thread-Id"))
	require.Equal(t, "child-thread-v7", headers.Get("X-Client-Request-Id"))
	require.Empty(t, headers.Get("Session_id"))
	require.Empty(t, headers.Get("Conversation_id"))
}

func TestApplyCodexNativeSessionHeadersPrefersResolvedHeaderIdentity(t *testing.T) {
	headers := make(http.Header)
	headers.Set("Session_id", "legacy-session")
	headers.Set("Thread-Id", "child-thread")

	applyCodexNativeSessionHeaders(headers, []byte(`{"prompt_cache_key":"body-session"}`))

	require.Equal(t, "legacy-session", headers.Get("Session-Id"))
	require.Equal(t, "child-thread", headers.Get("Thread-Id"))
	require.Equal(t, "child-thread", headers.Get("X-Client-Request-Id"))
}

func TestApplyCodexNativeSessionHeadersLegacyEscapeHatch(t *testing.T) {
	t.Setenv("CODEX_SESSION_HEADER_MODE", "legacy")
	headers := make(http.Header)
	headers.Set("Session_id", "legacy-session")

	applyCodexNativeSessionHeaders(headers, nil)

	require.Equal(t, "legacy-session", headers.Get("Session_id"))
	require.Empty(t, headers.Get("Session-Id"))
	require.Empty(t, headers.Get("Thread-Id"))
	require.Empty(t, headers.Get("X-Client-Request-Id"))
}

func TestIsolateOpenAIUpstreamSessionIDProducesStableUUIDv7(t *testing.T) {
	account := &Account{
		Platform:  PlatformOpenAI,
		Type:      AccountTypeOAuth,
		CreatedAt: time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC),
		Credentials: map[string]any{
			"chatgpt_account_id": "chatgpt-account",
		},
	}
	first := isolateOpenAIUpstreamSessionID(77, account, "client-session")
	second := isolateOpenAIUpstreamSessionID(77, account, "client-session")
	require.Equal(t, first, second)
	parsed, err := uuid.Parse(first)
	require.NoError(t, err)
	require.Equal(t, uuid.Version(7), parsed.Version())
}

func TestCompressCodexHTTPRequest(t *testing.T) {
	body := []byte(`{"model":"gpt-6.1-sol","stream":true}`)
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, chatgptCodexURL, bytes.NewReader(body))
	require.NoError(t, err)
	require.NoError(t, CompressOpenAICodexHTTPRequest(request))
	require.Equal(t, "zstd", request.Header.Get("Content-Encoding"))
	encoded, err := io.ReadAll(request.Body)
	require.NoError(t, err)
	require.NotEqual(t, body, encoded)
	decoder, err := zstd.NewReader(nil)
	require.NoError(t, err)
	defer decoder.Close()
	decoded, err := decoder.DecodeAll(encoded, nil)
	require.NoError(t, err)
	require.Equal(t, body, decoded)
}

func TestCompressCodexHTTPRequestSkipsCompactAndCanBeDisabled(t *testing.T) {
	compact, err := http.NewRequest(http.MethodPost, chatgptCodexURL+"/compact", bytes.NewReader([]byte(`{}`)))
	require.NoError(t, err)
	require.NoError(t, CompressOpenAICodexHTTPRequest(compact))
	require.Empty(t, compact.Header.Get("Content-Encoding"))

	t.Setenv("CODEX_REQUEST_COMPRESSION", "off")
	request, err := http.NewRequest(http.MethodPost, chatgptCodexURL, bytes.NewReader([]byte(`{}`)))
	require.NoError(t, err)
	require.NoError(t, CompressOpenAICodexHTTPRequest(request))
	require.Empty(t, request.Header.Get("Content-Encoding"))
}
