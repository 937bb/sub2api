package service

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestCodexCapturedTurnMetadataScopedWithoutRoutingChanges(t *testing.T) {
	account := accountDeviceFixture(11, "fixture-owner")
	raw := `{"installation_id":"client-device","session_id":"legacy-session","thread_id":"legacy-thread","turn_id":"turn-one","root_turn_id":"turn-one","context_window_id":"context-one","counter":9007199254740993,"sandbox_mode":"read-only"}`
	h := http.Header{"x-codex-turn-metadata": {raw}, "X-CODEX-TURN-METADATA": {raw}, "thread-id": {"legacy-thread"}, "X-CLIENT-REQUEST-ID": {"client-request"}}
	applyCodexAccountIdentityHeaders(h, account, 77)
	metadata := h.Get(openAIWSTurnMetadataHeader)
	require.Equal(t, gjson.Get(metadata, "turn_id").String(), gjson.Get(metadata, "root_turn_id").String())
	require.NotEqual(t, "context-one", gjson.Get(metadata, "context_window_id").String())
	require.Equal(t, "9007199254740993", gjson.Get(metadata, "counter").Raw)
	require.Equal(t, "read-only", gjson.Get(metadata, "sandbox_mode").String())
	require.Equal(t, scopeCodexAccountIdentityValue(account, 77, "thread", "legacy-thread"), h.Get("thread-id"))
	require.Equal(t, scopeCodexAccountIdentityValue(account, 77, "request", "client-request"), h.Get("x-client-request-id"))
	for _, name := range []string{openAIWSTurnMetadataHeader, "thread-id", "x-client-request-id"} {
		count := 0
		for key := range h {
			if strings.EqualFold(key, name) {
				count++
			}
		}
		require.Equal(t, 1, count, name)
	}
	cm := map[string]any{"x-codex-turn-metadata": raw}
	body, err := json.Marshal(map[string]any{"input": []any{map[string]any{"role": "assistant", "content": "old conversation"}}, "client_metadata": cm})
	require.NoError(t, err)
	rewritten, changed, err := applyCodexAccountIdentityClientMetadataRaw(body, account, 77)
	require.NoError(t, err)
	require.True(t, changed)
	require.JSONEq(t, metadata, gjson.GetBytes(rewritten, "client_metadata.x-codex-turn-metadata").String())
	require.Equal(t, gjson.GetBytes(body, "input").Raw, gjson.GetBytes(rewritten, "input").Raw)
	require.NotContains(t, string(rewritten), "conversation_binding")
	for _, mode := range []string{"session", "full"} {
		a := newTestOAuthAccount(11, map[string]any{codexFingerprintModeExtraKey: mode})
		ids := resolveCodexFingerprintIDsFromRequest(a, http.Header{})
		header := http.Header{}
		header.Set(openAIWSTurnMetadataHeader, metadata)
		applyCodexFingerprintHeaders(header, ids)
		out := header.Get(openAIWSTurnMetadataHeader)
		require.Equal(t, gjson.Get(out, "turn_id").String(), gjson.Get(out, "root_turn_id").String())
		require.Equal(t, "9007199254740993", gjson.Get(out, "counter").Raw)
		payload := map[string]any{"x-codex-turn-metadata": metadata, "turn_id": "same", "root_turn_id": "same"}
		require.True(t, applyCodexFingerprintClientMetadata(map[string]any{"client_metadata": payload}, ids))
		require.Equal(t, payload["turn_id"], payload["root_turn_id"])
		embedded, ok := payload["x-codex-turn-metadata"].(string)
		require.True(t, ok)
		require.JSONEq(t, out, embedded)
		encoded := []byte(`{"client_metadata":{"counter":9007199254740993,"turn_id":"root","root_turn_id":"root"}}`)
		rawResult, _, err := applyCodexFingerprintClientMetadataRaw(encoded, ids)
		require.NoError(t, err)
		require.Equal(t, "9007199254740993", gjson.GetBytes(rawResult, "client_metadata.counter").Raw)
		require.Equal(t, gjson.GetBytes(rawResult, "client_metadata.turn_id").String(), gjson.GetBytes(rawResult, "client_metadata.root_turn_id").String())
	}
}

func TestCodexMetadataPreservesDistinctRootAndMissingFields(t *testing.T) {
	a := accountDeviceFixture(11, "owner")
	for _, raw := range []string{`{"turn_id":"child","root_turn_id":"root","context_window_id":"context"}`, `{"turn_id":"","request_kind":"prewarm"}`} {
		h := http.Header{}
		h.Set(openAIWSTurnMetadataHeader, raw)
		applyCodexAccountIdentityHeaders(h, a, 77)
		next := h.Get(openAIWSTurnMetadataHeader)
		if gjson.Get(raw, "root_turn_id").Exists() {
			require.NotEqual(t, gjson.Get(next, "turn_id").String(), gjson.Get(next, "root_turn_id").String())
		} else {
			require.False(t, gjson.Get(next, "root_turn_id").Exists())
			require.Empty(t, gjson.Get(next, "turn_id").String())
		}
	}
	invalid := `{"turn_id":"one"} trailing`
	h := http.Header{}
	h.Set(openAIWSTurnMetadataHeader, invalid)
	applyCodexAccountIdentityHeaders(h, a, 77)
	require.Equal(t, invalid, h.Get(openAIWSTurnMetadataHeader))
}
