package service

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSIWCCacheSessionAffinityIsStableAndIsolated(t *testing.T) {
	body := []byte(`{"model":"gpt-6-astra","prompt_cache_key":"thread-one","input":[{"role":"developer","content":"stable instructions"},{"role":"user","content":"first question"}]}`)
	build := func(body []byte, apiKeyID int64, subject, clientID, token, session, requestID string) string {
		t.Helper()
		c := newOpenAIRejectedFieldTestContext(body)
		c.Set("api_key", &APIKey{ID: apiKeyID})
		c.Request.Header.Set("session_id", session)
		c.Request.Header.Set("X-Request-ID", requestID)
		c.Request.Header.Set("Cookie", "private-cookie")
		account := siwcFixtureAccount()
		account.Credentials["subject"], account.Credentials["client_id"] = subject, clientID
		req, err := buildSIWCResponsesRequest(context.Background(), c, account, body, token, "")
		require.NoError(t, err)
		_, err = uuid.Parse(req.Header.Get("session_id"))
		require.NoError(t, err, "SIWC needs stable session affinity even when the caller only sends a body cache key")
		require.NotEqual(t, session, req.Header.Get("session_id"))
		for _, header := range []string{"conversation_id", "Cookie", "X-Request-ID", "ChatGPT-Account-Id", "Originator"} {
			require.Empty(t, req.Header.Get(header))
		}
		return req.Header.Get("session_id")
	}
	first := build(body, 100, "subject-a", "client-a", "token-a", "", "request-1")
	require.Equal(t, first, build(body, 100, "subject-a", "client-a", "refreshed-token", "", "request-2"))
	require.NotEqual(t, first, build(body, 101, "subject-a", "client-a", "token-a", "", "request-1"))
	require.NotEqual(t, first, build(body, 100, "subject-b", "client-a", "token-a", "", "request-1"))
	require.NotEqual(t, first, build(body, 100, "subject-a", "client-b", "token-a", "", "request-1"))
	explicit := build(body, 100, "subject-a", "client-a", "token-a", "header-thread", "request-1")
	changedKey := []byte(strings.Replace(string(body), "thread-one", "thread-two", 1))
	require.Equal(t, explicit, build(changedKey, 100, "subject-a", "client-a", "token-a", "header-thread", "request-2"))
	require.NotEqual(t, first, build(changedKey, 100, "subject-a", "client-a", "token-a", "", "request-2"))

	noKey := []byte(`{"model":"gpt-6-astra","input":[{"role":"developer","content":"stable instructions"},{"role":"user","content":"first question"}]}`)
	continued := []byte(`{"model":"gpt-6-astra","input":[{"role":"developer","content":"stable instructions"},{"role":"user","content":"first question"},{"role":"assistant","content":"answer"},{"role":"developer","content":"turn-specific update"},{"role":"user","content":"next question"}]}`)
	require.Equal(t, build(noKey, 100, "subject-a", "client-a", "token-a", "", "request-1"), build(continued, 100, "subject-a", "client-a", "token-a", "", "request-2"))
}

func TestSIWCTopLevelToolsPreserveGrowingHistoryPrefix(t *testing.T) {
	first := []byte(`{"model":"gpt-6-astra","input":[{"role":"developer","content":"instructions"},{"role":"user","content":"read the file"}],"tools":[{"type":"function","name":"read_file","parameters":{"type":"object"}},{"type":"custom","name":"patch"}]}`)
	var next map[string]any
	require.NoError(t, json.Unmarshal(first, &next))
	input, ok := next["input"].([]any)
	require.True(t, ok)
	next["input"] = append(input,
		map[string]any{"type": "function_call", "name": "read_file", "call_id": "call_1", "arguments": "{}"},
		map[string]any{"type": "function_call_output", "call_id": "call_1", "output": "file contents"},
		map[string]any{"role": "user", "content": "continue"},
	)
	nextBody, err := json.Marshal(next)
	require.NoError(t, err)
	a, err := normalizeSIWCResponsesBody(first)
	require.NoError(t, err)
	b, err := normalizeSIWCResponsesBody(nextBody)
	require.NoError(t, err)
	prefix := gjson.GetBytes(a, "input").Array()
	continued := gjson.GetBytes(b, "input").Array()
	for i := range prefix {
		require.JSONEq(t, prefix[i].Raw, continued[i].Raw, "normalization must not move the tool declaration after each new turn")
	}
	require.Equal(t, "read_file", gjson.GetBytes(b, `input.#(type=="additional_tools").tools.0.name`).String())
	require.Equal(t, "call_1", gjson.GetBytes(b, `input.#(type=="function_call_output").call_id`).String())
	again, err := normalizeSIWCResponsesBody(b)
	require.NoError(t, err)
	require.JSONEq(t, string(b), string(again))
}

func TestSIWCSessionFallbackDoesNotRewriteCallerCacheOptions(t *testing.T) {
	body := []byte(`{"model":"gpt-6-astra","input":[{"role":"user","content":"first question"}]}`)
	c := newOpenAIRejectedFieldTestContext(body)
	c.Set("api_key", &APIKey{ID: 100})
	req, err := buildSIWCResponsesRequest(context.Background(), c, siwcFixtureAccount(), body, "token", "")
	require.NoError(t, err)
	require.NotEmpty(t, req.Header.Get("session_id"))
	sent, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	for _, key := range []string{"prompt_cache_options", "prompt_cache_key", "prompt_cache_retention"} {
		require.False(t, gjson.GetBytes(sent, key).Exists())
	}
}
