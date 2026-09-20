package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type turnStateBucketSyncRepo struct {
	OpenAICodexTurnStateStore
	record        *OpenAICodexTurnStateRecord
	readError     error
	writeError    error
	readCalls     int
	writeCalls    int
	accountID     int64
	model         string
	targetLengths []int
}

func (r *turnStateBucketSyncRepo) LoadPreferredOpenAICodexTurnState(_ context.Context, accountID int64, model string, lengths []int, _ time.Time) (*OpenAICodexTurnStateRecord, error) {
	r.readCalls++
	r.accountID, r.model, r.targetLengths = accountID, model, append([]int(nil), lengths...)
	return cloneOpenAICodexTurnStateRecord(r.record), r.readError
}

func (r *turnStateBucketSyncRepo) UpsertOpenAICodexTurnState(_ context.Context, record *OpenAICodexTurnStateRecord) error {
	r.writeCalls++
	if r.writeError != nil {
		return r.writeError
	}
	r.record = cloneOpenAICodexTurnStateRecord(record)
	return nil
}

func TestOpenAICodexTurnStatePoolSyncAvoidsDuplicateScanAcrossInstances(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	repo := &turnStateBucketSyncRepo{}
	firstGateway, secondGateway := &OpenAIGatewayService{}, &OpenAIGatewayService{}
	first, second := firstGateway.getOpenAICodexTurnStatePool(), secondGateway.getOpenAICodexTurnStatePool()
	for _, pool := range []*openAICodexTurnStatePool{first, second} {
		pool.repo = repo
		pool.now = func() time.Time { return now }
		pool.setTargetLengths([]int{356, 292})
	}
	scanner := &openAICodexTurnStateScanner{gateway: secondGateway}
	settings := defaultOpenAICodexTurnStateScanSettings()
	settings.TargetLengths = []int{356, 292}
	scanner.settings.Store(settings)
	accountID := int64(42)
	require.True(t, scanner.stateNeedsRefresh(accountID, "gpt-5.5", now))
	issuedAt := now.Add(-10 * time.Minute)
	state := testOpenAICodexTurnState(356, issuedAt, 'a')
	require.NoError(t, first.observeDurably(context.Background(), state, accountID, "session", "gpt-5.5", "http"))
	require.Equal(t, 1, repo.writeCalls)
	require.Empty(t, first.queue)
	_, ok := second.preferredForBucket(accountID, "gpt-5.5")
	require.False(t, ok)
	require.NoError(t, second.refreshBucket(context.Background(), accountID, " GPT-5.5 "))
	require.Equal(t, accountID, repo.accountID)
	require.Equal(t, "gpt-5.5", repo.model)
	require.Equal(t, []int{356, 292}, repo.targetLengths)
	require.False(t, scanner.stateNeedsRefresh(accountID, "gpt-5.5", now))
	selected, ok := second.preferredForBucket(accountID, "gpt-5.5")
	require.True(t, ok)
	require.Equal(t, state, selected)
	expiresAt, ok := second.preferredExpiryForBucket(accountID, "gpt-5.5")
	require.True(t, ok)
	require.Equal(t, issuedAt.Add(time.Hour), expiresAt)
	require.Empty(t, second.queue)

	now = now.Add(5 * time.Minute)
	require.NoError(t, first.observeDurably(context.Background(), state, accountID, "session", "gpt-5.5", "http"))
	require.NoError(t, second.refreshBucket(context.Background(), accountID, "gpt-5.5"))
	expiresAt, ok = second.preferredExpiryForBucket(accountID, "gpt-5.5")
	require.True(t, ok)
	require.Equal(t, issuedAt.Add(time.Hour), expiresAt)
	_, ok = second.preferredForBucket(84, "gpt-5.5")
	require.False(t, ok)
	_, ok = second.preferredForBucket(accountID, "gpt-6-astra")
	require.False(t, ok)
}

func TestOpenAICodexTurnStatePoolDurableFailureDoesNotPublish(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	repo := &turnStateBucketSyncRepo{writeError: errors.New("database unavailable")}
	pool := newOpenAICodexTurnStatePool()
	pool.repo = repo
	state := testOpenAICodexTurnState(332, now, 'a')
	err := pool.observeDurably(context.Background(), state, 42, "session", "gpt-5.5", "http")
	require.ErrorContains(t, err, "database unavailable")
	require.Empty(t, pool.entries)
	require.Empty(t, pool.queue)
	require.Nil(t, repo.record)

	pool.repo = nil
	require.NoError(t, pool.observeDurably(context.Background(), state, 42, "session", "gpt-5.5", "http"))
	_, ok := pool.preferredForBucket(42, "gpt-5.5")
	require.True(t, ok)
}

func TestOpenAICodexTurnStatePoolRefreshPreservesLocalStateOnEmptyAndError(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	pool := newOpenAICodexTurnStatePool()
	accountID := int64(42)
	state := testOpenAICodexTurnState(332, now, 'a')
	pool.observe(state, &accountID, "session", "gpt-5.5", "http")
	repo := &turnStateBucketSyncRepo{}
	pool.repo = repo
	require.NoError(t, pool.refreshBucket(context.Background(), accountID, "gpt-5.5"))
	selected, ok := pool.preferredForBucket(accountID, "gpt-5.5")
	require.True(t, ok)
	require.Equal(t, state, selected)
	repo.readError = errors.New("database unavailable")
	require.ErrorContains(t, pool.refreshBucket(context.Background(), accountID, "gpt-5.5"), "database unavailable")
	selected, ok = pool.preferredForBucket(accountID, "gpt-5.5")
	require.True(t, ok)
	require.Equal(t, state, selected)
}

func TestOpenAICodexTurnStatePoolRefreshRejectsForeignAndCorruptRecords(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	accountID := int64(42)
	state := testOpenAICodexTurnState(332, now, 'a')
	valid := newObservedOpenAICodexTurnStateRecord(state, &accountID, "session", "gpt-5.5", "http", now)
	for name, mutate := range map[string]func(*OpenAICodexTurnStateRecord){
		"account":   func(r *OpenAICodexTurnStateRecord) { foreign := int64(84); r.SourceAccountID = &foreign },
		"model":     func(r *OpenAICodexTurnStateRecord) { r.SourceModel = "gpt-6-astra" },
		"hash":      func(r *OpenAICodexTurnStateRecord) { r.StateHash = "unrelated" },
		"length":    func(r *OpenAICodexTurnStateRecord) { r.ValueLength = 292 },
		"timestamp": func(r *OpenAICodexTurnStateRecord) { r.IssuedAt = now.Add(time.Minute) },
	} {
		t.Run(name, func(t *testing.T) {
			record := cloneOpenAICodexTurnStateRecord(valid)
			mutate(record)
			pool := newOpenAICodexTurnStatePool()
			pool.repo = &turnStateBucketSyncRepo{record: record}
			require.Error(t, pool.refreshBucket(context.Background(), accountID, "gpt-5.5"))
			require.Empty(t, pool.entries)
		})
	}
	pool := newOpenAICodexTurnStatePool()
	pool.now = func() time.Time { return now }
	record := cloneOpenAICodexTurnStateRecord(valid)
	record.ExpiresAt = now.Add(10 * time.Minute)
	pool.repo = &turnStateBucketSyncRepo{record: record}
	require.NoError(t, pool.refreshBucket(context.Background(), accountID, "gpt-5.5"))
	expiry, ok := pool.preferredExpiryForBucket(accountID, "gpt-5.5")
	require.True(t, ok)
	require.Equal(t, record.ExpiresAt, expiry)
}
