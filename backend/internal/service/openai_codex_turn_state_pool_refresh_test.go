package service

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpenAICodexTurnStatePool_ReobservingOlderStateKeepsNewerExpiry(t *testing.T) {
	for _, length := range []int{292, 332} {
		t.Run(strconv.Itoa(length), func(t *testing.T) {
			now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
			pool := newOpenAICodexTurnStatePool()
			pool.now = func() time.Time { return now }
			accountID, otherAccountID := int64(42), int64(84)
			model, otherModel := "gpt-5.5", "gpt-6-astra"
			older := testOpenAICodexTurnState(length, now.Add(-40*time.Minute), 'a')
			newerIssuedAt := now.Add(-10 * time.Minute)
			newer := testOpenAICodexTurnState(length, newerIssuedAt, 'b')
			otherAccount := testOpenAICodexTurnState(length, now, 'c')
			otherModelState := testOpenAICodexTurnState(length, now, 'd')

			pool.observe(older, &accountID, "old-session", model, "scanner")
			now = now.Add(time.Minute)
			pool.observe(newer, &accountID, "new-session", model, "scanner")
			pool.observe(otherAccount, &otherAccountID, "other-account", model, "scanner")
			pool.observe(otherModelState, &accountID, "other-model", otherModel, "scanner")
			now = now.Add(time.Minute)
			pool.observe(older, &accountID, "old-session-repeated", model, "scanner")

			assertPreferred := func() {
				t.Helper()
				selected, ok := pool.preferredForBucket(accountID, model)
				require.True(t, ok)
				require.Equal(t, newer, selected)
				expiry, ok := pool.preferredExpiryForBucket(accountID, model)
				require.True(t, ok)
				require.Equal(t, newerIssuedAt.Add(openAICodexTurnStateTTL), expiry)
				selected, ok = pool.preferredForBucket(otherAccountID, model)
				require.True(t, ok)
				require.Equal(t, otherAccount, selected)
				selected, ok = pool.preferredForBucket(accountID, otherModel)
				require.True(t, ok)
				require.Equal(t, otherModelState, selected)
			}
			assertPreferred()

			pool.mu.Lock()
			pool.rebuildAccountsLocked()
			pool.mu.Unlock()
			assertPreferred()
		})
	}
}

func TestOpenAICodexTurnStatePool_RefreshUsesHealthyFallbackWithinBucket(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	pool := newOpenAICodexTurnStatePool()
	pool.now = func() time.Time { return now }
	accountID := int64(42)
	model := "gpt-5.5"
	deadline := now.Add(15 * time.Minute)
	preferred := testOpenAICodexTurnState(332, now.Add(-55*time.Minute), 'a')
	fallback := testOpenAICodexTurnState(292, now.Add(-5*time.Minute), 'b')
	pool.observe(preferred, &accountID, "preferred", model, "scanner")
	require.False(t, pool.hasReusableStateBeyond(accountID, model, deadline))
	pool.observe(fallback, &accountID, "fallback", model, "scanner")
	require.True(t, pool.hasReusableStateBeyond(accountID, model, deadline))
	require.False(t, pool.hasReusableStateBeyond(84, model, deadline))
	require.False(t, pool.hasReusableStateBeyond(accountID, "gpt-6-astra", deadline))
	require.False(t, pool.hasReusableStateBeyond(accountID, model, now.Add(55*time.Minute)))
	selected, ok := pool.preferredForBucket(accountID, model)
	require.True(t, ok)
	require.Equal(t, preferred, selected)

	now = now.Add(40 * time.Minute)
	require.False(t, pool.hasReusableStateBeyond(accountID, model, now.Add(15*time.Minute)))
}

func TestOpenAICodexTurnStatePool_292FallbackDoesNotSatisfy332Upgrade(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	p := newOpenAICodexTurnStatePool()
	p.now = func() time.Time { return now }
	accountID := int64(42)
	model := "gpt-5.5"
	state292 := testOpenAICodexTurnState(openAICodexTurnStateLength292, now.Add(-5*time.Minute), 'a')
	p.observe(state292, &accountID, "fallback", model, "scanner")
	deadline := now.Add(openAICodexTurnStateScanRefreshBefore)

	require.True(t, p.hasReusableStateOfLengthBeyond(accountID, model, openAICodexTurnStateLength292, deadline))
	require.False(t, p.hasReusableStateOfLengthBeyond(accountID, model, openAICodexTurnStateLength332, deadline))
}
