package service

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestWSTurnMetadataFallbackPreservesEveryExplicitFrame(t *testing.T) {
	for _, value := range []any{`{"request_kind":"prewarm","turn_id":""}`, `{"request_kind":"turn","turn_id":"t1","root_turn_id":"t1"}`, `{"request_kind":"turn","turn_id":"t1","root_turn_id":"t1","tool":"result"}`, "", nil, "malformed", 42} {
		metadata := map[string]any{openAIWSTurnMetadataHeader: value, "turn_id": "t1"}
		payload := map[string]any{"client_metadata": metadata}
		before, err := json.Marshal(payload)
		require.NoError(t, err)
		setOpenAIWSTurnMetadata(payload, `{"request_kind":"prewarm"}`)
		after, err := json.Marshal(payload)
		require.NoError(t, err)
		require.Equal(t, string(before), string(after))
	}
	for _, metadata := range []any{map[string]any{"turn_id": "t1"}, map[string]string{"turn_id": "t1"}} {
		payload := map[string]any{"client_metadata": metadata}
		before, err := json.Marshal(metadata)
		require.NoError(t, err)
		setOpenAIWSTurnMetadata(payload, `{"request_kind":"turn"}`)
		result, ok := payload["client_metadata"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, `{"request_kind":"turn"}`, result[openAIWSTurnMetadataHeader])
		after, err := json.Marshal(metadata)
		require.NoError(t, err)
		require.Equal(t, string(before), string(after), "fallback must not mutate source map")
	}
}

func TestWSFrameExecutionScopeOverridesHandshakeOnlyForWS(t *testing.T) {
	c := newOpenAIWSExecutionScopeTestContext(map[string]string{openAIWSTurnMetadataHeader: `{"request_kind":"prewarm","thread_id":"parent"}`})
	body := []byte(`{"client_metadata":{"x-codex-turn-metadata":"{\"request_kind\":\"memory\",\"thread_id\":\"child\"}"}}`)
	require.Equal(t, "parent", resolveOpenAIWSClientThreadID(c, body))
	require.Empty(t, resolveOpenAIWSExecutionLane(c, body), "HTTP keeps header precedence")
	c.Request.Header.Set("Upgrade", "websocket")
	require.Equal(t, "child", resolveOpenAIWSClientThreadID(c, body))
	require.Equal(t, "kind=memory", resolveOpenAIWSExecutionLane(c, body))
	require.JSONEq(t, `{"request_kind":"memory","thread_id":"child"}`, openAIWSFrameTurnMetadata(c, body))
	require.Empty(t, openAIWSFrameTurnMetadata(c, []byte(`{"client_metadata":{"x-codex-turn-metadata":""}}`)))
	require.JSONEq(t, `{"request_kind":"prewarm","thread_id":"parent"}`, openAIWSFrameTurnMetadata(c, []byte(`{}`)))
}

func TestWSReconnectMetadataRefreshWithoutCacheKey(t *testing.T) {
	headers := http.Header{}
	headers.Set(openAIWSTurnMetadataHeader, `{"request_kind":"prewarm"}`)
	updateOpenAIWSFrameTurnMetadataHeader(headers, []byte(`{"client_metadata":{"x-codex-turn-metadata":"{\"request_kind\":\"turn\",\"turn_id\":\"already-scoped\"}"}}`))
	require.JSONEq(t, `{"request_kind":"turn","turn_id":"already-scoped"}`, headers.Get(openAIWSTurnMetadataHeader))
	updateOpenAIWSFrameTurnMetadataHeader(headers, []byte(`{"client_metadata":{"x-codex-turn-metadata":""}}`))
	require.Empty(t, headers.Get(openAIWSTurnMetadataHeader))
}

func TestOpenAIWSRequestTraceMetadataUsesFrameClientMetadata(t *testing.T) {
	c := newOpenAIWSExecutionScopeTestContext(map[string]string{
		"traceparent": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		"tracestate":  "vendor=one",
		"baggage":     "tenant=alpha",
	})
	payload := map[string]any{"type": "response.create", "client_metadata": map[string]any{"keep": "yes"}}
	require.True(t, applyOpenAIWSRequestTraceMetadata(payload, c))
	metadata, ok := payload["client_metadata"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", metadata[openAIWSRequestTraceparentMetadataKey])
	require.Equal(t, "vendor=one", metadata[openAIWSRequestTracestateMetadataKey])
	require.Equal(t, "yes", metadata["keep"])
	_, hasBaggage := metadata["baggage"]
	require.False(t, hasBaggage, "baggage is not an official WS frame metadata key")

	// Explicit values from the frame win over inbound headers.
	payload = map[string]any{"client_metadata": map[string]any{
		openAIWSRequestTraceparentMetadataKey: "frame-parent",
		openAIWSRequestTracestateMetadataKey:  "frame-state",
	}}
	require.False(t, applyOpenAIWSRequestTraceMetadata(payload, c))
	metadata = payload["client_metadata"].(map[string]any)
	require.Equal(t, "frame-parent", metadata[openAIWSRequestTraceparentMetadataKey])
	require.Equal(t, "frame-state", metadata[openAIWSRequestTracestateMetadataKey])
}

func TestOpenAIWSRequestTraceMetadataRawPreservesMalformedMetadata(t *testing.T) {
	c := newOpenAIWSExecutionScopeTestContext(map[string]string{"traceparent": "00-abc-def-01"})
	original := []byte(`{"type":"response.create","client_metadata":"opaque"}`)
	updated, changed, err := applyOpenAIWSRequestTraceMetadataRaw(original, c)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, string(original), string(updated))
}

func TestStampOpenAIWSStreamRequestStartMSOverwritesPerFrameTimestamp(t *testing.T) {
	now := time.UnixMilli(1794723456789)
	payload := map[string]any{"client_metadata": map[string]any{
		openAIWSStreamRequestStartMSMetadataKey: "100",
		"keep":                                  "yes",
	}}
	require.True(t, stampOpenAIWSStreamRequestStartMS(payload, now))
	metadata := payload["client_metadata"].(map[string]any)
	require.Equal(t, strconv.FormatInt(now.UnixMilli(), 10), metadata[openAIWSStreamRequestStartMSMetadataKey])
	require.Equal(t, "yes", metadata["keep"])

	raw, changed, err := stampOpenAIWSStreamRequestStartMSRaw([]byte(`{"type":"response.create","client_metadata":{"keep":"raw"}}`), now)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, strconv.FormatInt(now.UnixMilli(), 10), gjson.GetBytes(raw, "client_metadata."+openAIWSStreamRequestStartMSMetadataKey).String())
	require.Equal(t, "raw", gjson.GetBytes(raw, "client_metadata.keep").String())
}
