package service

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestCodexMetadataPreservesNumbersAcrossIdentityRewrites(t *testing.T) {
	account := &Account{ID: 77, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Credentials: map[string]any{"chatgpt_account_id": "test-account"},
		Extra:       map[string]any{codexFingerprintSeedExtraKey: testCodexFingerprintSeed}}
	const metadata = `{"session_id":"client-session","timezone":"Asia/Shanghai","counter":9007199254740993,"nested":{"value":0.1234567890123456789}}`
	const body = `{"input":[{"role":"user","content":"unchanged"}],"client_metadata":{"session_id":"client-session","timezone":"Asia/Shanghai","counter":9007199254740993,"nested":{"value":0.1234567890123456789},"x-codex-turn-metadata":"{\"session_id\":\"client-session\",\"counter\":9007199254740993,\"nested\":{\"value\":0.1234567890123456789}}"}}`
	assertNumbers := func(t *testing.T, raw string) {
		t.Helper()
		require.Equal(t, "9007199254740993", gjson.Get(raw, "counter").Raw)
		require.Equal(t, "0.1234567890123456789", gjson.Get(raw, "nested.value").Raw)
	}
	for _, fingerprint := range []bool{false, true} {
		name := "account"
		if fingerprint {
			name = "fingerprint-and-account"
		}
		t.Run(name, func(t *testing.T) {
			next := []byte(body)
			var err error
			if fingerprint {
				ids := resolveCodexFingerprintIDs(account, "client-session", codexFingerprintFull)
				require.NotNil(t, ids)
				next, _, err = applyCodexFingerprintClientMetadataRaw(next, ids)
				require.NoError(t, err)
			}
			next, changed, err := applyCodexAccountIdentityClientMetadataRaw(next, account, 42)
			require.NoError(t, err)
			require.True(t, changed)
			cm := gjson.GetBytes(next, "client_metadata")
			assertNumbers(t, cm.Raw)
			assertNumbers(t, cm.Get(openAIWSTurnMetadataHeader).String())
			require.Equal(t, codexClientTimezone, cm.Get("timezone").String())
			require.NotEqual(t, "client-session", cm.Get("session_id").String())
			require.Equal(t, gjson.Get(body, "input").Raw, gjson.GetBytes(next, "input").Raw)
		})
	}
	t.Run("header", func(t *testing.T) {
		headers := make(http.Header)
		headers.Set(openAIWSTurnMetadataHeader, metadata)
		applyCodexAccountIdentityHeaders(headers, account, 42)
		assertNumbers(t, headers.Get(openAIWSTurnMetadataHeader))
		require.Equal(t, codexClientTimezone, gjson.Get(headers.Get(openAIWSTurnMetadataHeader), "timezone").String())
	})
	t.Run("turn-metadata-merge", func(t *testing.T) {
		next, ok := mergeCodexTurnMetadata(metadata, map[string]any{"session_id": "updated-session"})
		require.True(t, ok)
		assertNumbers(t, next)
	})
	t.Run("invalid-turn-metadata", func(t *testing.T) {
		for _, raw := range []string{metadata + `{}`, metadata + ` trailing`, `{"counter":`} {
			next, ok := mergeCodexTurnMetadata(raw, map[string]any{"session_id": "updated-session"})
			require.True(t, ok)
			require.JSONEq(t, `{"session_id":"updated-session"}`, next)
		}
	})
}
