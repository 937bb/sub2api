package service

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNormalizeOpenAICodexTurnStateProxyURLAndMask(t *testing.T) {
	normalized, err := normalizeOpenAICodexTurnStateProxyURL("proxy.example:8443:scanner:secret")
	require.NoError(t, err)
	require.Equal(t, "http", strings.SplitN(normalized, ":", 2)[0])
	require.Contains(t, normalized, "scanner:secret@proxy.example:8443")

	masked := maskOpenAICodexTurnStateProxyURL(normalized)
	require.Contains(t, masked, "scanner:%2A%2A%2A@proxy.example:8443")
	require.NotContains(t, masked, "secret")

	_, err = normalizeOpenAICodexTurnStateProxyURL("ftp://proxy.example:21")
	require.Error(t, err)
}

func TestMergeOpenAICodexTurnStateScanProxiesDeduplicatesAndCopies(t *testing.T) {
	dedicated := &OpenAICodexTurnStateProxy{ID: 7, Source: "state", ProxyURL: "http://user:pass@proxy.example:8080"}
	sharedDuplicate := &OpenAICodexTurnStateProxy{Source: "shared", SourceID: 9, ProxyURL: "http://user:pass@proxy.example:8080"}
	sharedUnique := &OpenAICodexTurnStateProxy{Source: "shared", SourceID: 10, ProxyURL: "socks5://127.0.0.1:1080"}
	invalid := &OpenAICodexTurnStateProxy{Source: "shared", ProxyURL: "invalid"}

	merged := mergeOpenAICodexTurnStateScanProxies(
		[]*OpenAICodexTurnStateProxy{dedicated},
		[]*OpenAICodexTurnStateProxy{sharedDuplicate, sharedUnique, invalid},
	)

	require.Len(t, merged, 2)
	require.Equal(t, int64(7), merged[0].ID)
	require.Equal(t, int64(10), merged[1].SourceID)
	require.NotSame(t, dedicated, merged[0])
	require.Equal(t, "http://user:pass@proxy.example:8080", dedicated.ProxyURL)
}

func TestOpenAICodexTurnStateRetryDelayCapsAtFiveMinutes(t *testing.T) {
	require.Equal(t, 5*time.Second, openAICodexTurnStateRetryDelay(1))
	require.Equal(t, 10*time.Second, openAICodexTurnStateRetryDelay(2))
	require.Equal(t, 5*time.Minute, openAICodexTurnStateRetryDelay(7))
	require.Equal(t, 5*time.Minute, openAICodexTurnStateRetryDelay(100))
}

func TestOpenAICodexTurnStateScannerDeduplicatesAccountModelJobs(t *testing.T) {
	scanner := &openAICodexTurnStateScanner{
		queue:    make(chan openAICodexTurnStateScanJob, 4),
		inFlight: make(map[openAICodexTurnStateBucketKey]struct{}),
	}

	require.True(t, scanner.Enqueue(42, " GPT-5.5 ", false))
	require.False(t, scanner.Enqueue(42, "gpt-5.5", true))
	require.True(t, scanner.Enqueue(42, "gpt-5.6-sol", false))
	require.True(t, scanner.Enqueue(43, "gpt-5.5", false))

	key, ok := newOpenAICodexTurnStateBucketKey(42, "gpt-5.5")
	require.True(t, ok)
	scanner.finish(key)
	require.True(t, scanner.Enqueue(42, "gpt-5.5", false))
}

func TestOpenAICodexTurnStateModelsMatchRejectsMismatch(t *testing.T) {
	require.False(t, openAICodexTurnStateModelsMatch("gpt-5.5", ""))
	require.True(t, openAICodexTurnStateModelsMatch(" GPT-5.5 ", "gpt-5.5"))
	require.False(t, openAICodexTurnStateModelsMatch("gpt-5.5", "gpt-5.6-luna"))
}

func TestReadOpenAICodexTurnStateOfficialModelFromSSE(t *testing.T) {
	body := strings.NewReader("event: response.created\n" +
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-5.5"}}` + "\n\n" +
		`data: {"type":"response.output_text.delta","delta":"unused"}` + "\n\n")

	require.Equal(t, "gpt-5.5", readOpenAICodexTurnStateOfficialModel(body))
	require.Empty(t, readOpenAICodexTurnStateOfficialModel(strings.NewReader(
		`data: {"type":"response.created","response":{"id":"resp_2"}}`+"\n\n")))
}

func TestOpenAICodexTurnStateScannerPersistsActualSessionAndModelScope(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	pool := newOpenAICodexTurnStatePool()
	pool.now = func() time.Time { return now }
	accountID := int64(42)
	sessionID := "codex:ts:actual-scanner-session"
	state := testOpenAICodexTurnState(openAICodexTurnStateLength332, now, 's')

	pool.observe(state, &accountID, hashOpenAICodexTurnState(sessionID), "gpt-5.5", "scanner")

	key, ok := newOpenAICodexTurnStateBucketKey(accountID, "gpt-5.5")
	require.True(t, ok)
	pool.mu.RLock()
	record := pool.preferredByBucket[key]
	pool.mu.RUnlock()
	require.NotNil(t, record)
	require.Equal(t, accountID, *record.SourceAccountID)
	require.Equal(t, "gpt-5.5", record.SourceModel)
	require.Equal(t, hashOpenAICodexTurnState(sessionID), record.SourceSessionHash)

	_, otherModel := pool.preferredForBucket(accountID, "gpt-5.6-luna")
	require.False(t, otherModel)
}
