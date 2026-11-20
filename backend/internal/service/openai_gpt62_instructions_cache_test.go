package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestNormalizeGPT62InstructionsForCache_FirstTurnAndContinuation(t *testing.T) {
	account := &Account{ID: 62001, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	now := time.Unix(1795219200, 0)
	first := []byte(`{"model":"gpt-6.2-astra","prompt_cache_key":"session-1","instructions":"stable codex base","input":[{"type":"message","role":"user","content":"hello"}],"sequence":900719925474099312345}`)

	firstOut, changed, err := normalizeGPT62InstructionsForCache(first, account, now)
	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, gjson.GetBytes(firstOut, "instructions").Exists())
	require.Equal(t, "developer", gjson.GetBytes(firstOut, "input.0.role").String())
	require.Equal(t, "stable codex base", gjson.GetBytes(firstOut, "input.0.content").String())
	require.Equal(t, "user", gjson.GetBytes(firstOut, "input.1.role").String())
	require.Equal(t, "hello", gjson.GetBytes(firstOut, "input.1.content").String())
	require.Equal(t, "900719925474099312345", gjson.GetBytes(firstOut, "sequence").Raw)

	second := []byte(`{"type":"response.create","model":"gpt-6.2-astra","prompt_cache_key":"session-1","previous_response_id":"resp_1","instructions":"stable codex base","input":[{"type":"message","role":"user","content":"continue"}]}`)
	secondOut, changed, err := normalizeGPT62InstructionsForCache(second, account, now.Add(time.Minute))
	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, gjson.GetBytes(secondOut, "instructions").Exists())
	require.Len(t, gjson.GetBytes(secondOut, "input").Array(), 1)
	require.Equal(t, "user", gjson.GetBytes(secondOut, "input.0.role").String())
	require.Equal(t, "resp_1", gjson.GetBytes(secondOut, "previous_response_id").String())
}

func TestNormalizeGPT62InstructionsForCache_UnknownOrChangedContinuationKeepsInstructions(t *testing.T) {
	account := &Account{ID: 62002, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	now := time.Unix(1795219200, 0)
	unknown := []byte(`{"model":"gpt-6.2-sol","prompt_cache_key":"unknown-session","previous_response_id":"resp_old","instructions":"stable codex base","input":[{"role":"user","content":"continue"}]}`)

	out, changed, err := normalizeGPT62InstructionsForCache(unknown, account, now)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, unknown, out)

	first := []byte(`{"model":"gpt-6.2-sol","prompt_cache_key":"changed-session","instructions":"base v1","input":"hello"}`)
	_, changed, err = normalizeGPT62InstructionsForCache(first, account, now)
	require.NoError(t, err)
	require.True(t, changed)

	changedContinuation := []byte(`{"model":"gpt-6.2-sol","prompt_cache_key":"changed-session","previous_response_id":"resp_1","instructions":"base v2","input":"continue"}`)
	out, changed, err = normalizeGPT62InstructionsForCache(changedContinuation, account, now.Add(time.Minute))
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, changedContinuation, out)
}

func TestNormalizeGPT62InstructionsForCache_StringInputAndExistingDeveloper(t *testing.T) {
	account := &Account{ID: 62003, Platform: PlatformOpenAI, Type: AccountTypeSetupToken}
	now := time.Unix(1795219200, 0)
	body := []byte(`{"model":"gpt-6.2-astra","instructions":"base","input":"hello"}`)

	out, changed, err := normalizeGPT62InstructionsForCache(body, account, now)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "developer", gjson.GetBytes(out, "input.0.role").String())
	require.Equal(t, "base", gjson.GetBytes(out, "input.0.content").String())
	require.Equal(t, "user", gjson.GetBytes(out, "input.1.role").String())
	require.Equal(t, "hello", gjson.GetBytes(out, "input.1.content").String())

	existing := []byte(`{"model":"gpt-6.2-astra","prompt_cache_key":"existing","instructions":"base","input":[{"type":"message","role":"developer","content":"base"},{"role":"user","content":"hello"}]}`)
	out, changed, err = normalizeGPT62InstructionsForCache(existing, account, now)
	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, gjson.GetBytes(out, "instructions").Exists())
	require.Len(t, gjson.GetBytes(out, "input").Array(), 2)
}

func TestNormalizeCodexOutboundRequest_GPT62PreservesUserRole(t *testing.T) {
	account := &Account{ID: 62004, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	body := []byte(`{"model":"gpt-6.2-astra","prompt_cache_key":"role-session","instructions":"base","input":[{"role":"user","content":"hello"}]}`)

	out, changed, err := normalizeCodexOutboundRequest(body, account)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "developer", gjson.GetBytes(out, "input.0.role").String())
	require.Equal(t, "user", gjson.GetBytes(out, "input.1.role").String())

	legacy := []byte(`{"model":"gpt-6.1-sol","instructions":"base","input":[{"role":"user","content":"hello"}]}`)
	out, changed, err = normalizeCodexOutboundRequest(legacy, account)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "developer", gjson.GetBytes(out, "input.0.role").String())
}

func TestNormalizeGPT62InstructionsForCache_DestinationAndModelScope(t *testing.T) {
	now := time.Unix(1795219200, 0)
	body := []byte(`{"model":"gpt-6.2-astra","instructions":"base","input":"hello"}`)
	for _, account := range []*Account{
		{ID: 62005, Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
		{ID: 62006, Platform: PlatformAnthropic, Type: AccountTypeOAuth},
		nil,
	} {
		out, changed, err := normalizeGPT62InstructionsForCache(body, account, now)
		require.NoError(t, err)
		require.False(t, changed)
		require.Equal(t, body, out)
	}

	nonGPT62 := []byte(`{"model":"gpt-6.1-sol","instructions":"base","input":"hello"}`)
	out, changed, err := normalizeGPT62InstructionsForCache(nonGPT62, &Account{ID: 62007, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, now)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, nonGPT62, out)
}
