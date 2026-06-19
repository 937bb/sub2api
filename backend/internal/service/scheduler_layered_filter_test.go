//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFilterByMinPriority(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		result := filterByMinPriority(nil)
		require.Empty(t, result)
	})

	t.Run("single account", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, Priority: 5}, loadInfo: &AccountLoadInfo{}},
		}
		result := filterByMinPriority(accounts)
		require.Len(t, result, 1)
		require.Equal(t, int64(1), result[0].account.ID)
	})

	t.Run("multiple accounts same priority", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, Priority: 3}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 2, Priority: 3}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 3, Priority: 3}, loadInfo: &AccountLoadInfo{}},
		}
		result := filterByMinPriority(accounts)
		require.Len(t, result, 3)
	})

	t.Run("filters to min priority only", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, Priority: 5}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 2, Priority: 1}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 3, Priority: 3}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 4, Priority: 1}, loadInfo: &AccountLoadInfo{}},
		}
		result := filterByMinPriority(accounts)
		require.Len(t, result, 2)
		require.Equal(t, int64(2), result[0].account.ID)
		require.Equal(t, int64(4), result[1].account.ID)
	})
}

func TestFilterByMinLoadRate(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		result := filterByMinLoadRate(nil)
		require.Empty(t, result)
	})

	t.Run("single account", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{LoadRate: 50}},
		}
		result := filterByMinLoadRate(accounts)
		require.Len(t, result, 1)
		require.Equal(t, int64(1), result[0].account.ID)
	})

	t.Run("multiple accounts same load rate", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{LoadRate: 20}},
			{account: &Account{ID: 2}, loadInfo: &AccountLoadInfo{LoadRate: 20}},
			{account: &Account{ID: 3}, loadInfo: &AccountLoadInfo{LoadRate: 20}},
		}
		result := filterByMinLoadRate(accounts)
		require.Len(t, result, 3)
	})

	t.Run("filters to min load rate only", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{LoadRate: 80}},
			{account: &Account{ID: 2}, loadInfo: &AccountLoadInfo{LoadRate: 10}},
			{account: &Account{ID: 3}, loadInfo: &AccountLoadInfo{LoadRate: 50}},
			{account: &Account{ID: 4}, loadInfo: &AccountLoadInfo{LoadRate: 10}},
		}
		result := filterByMinLoadRate(accounts)
		require.Len(t, result, 2)
		require.Equal(t, int64(2), result[0].account.ID)
		require.Equal(t, int64(4), result[1].account.ID)
	})

	t.Run("zero load rate", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1}, loadInfo: &AccountLoadInfo{LoadRate: 0}},
			{account: &Account{ID: 2}, loadInfo: &AccountLoadInfo{LoadRate: 50}},
			{account: &Account{ID: 3}, loadInfo: &AccountLoadInfo{LoadRate: 0}},
		}
		result := filterByMinLoadRate(accounts)
		require.Len(t, result, 2)
		require.Equal(t, int64(1), result[0].account.ID)
		require.Equal(t, int64(3), result[1].account.ID)
	})
}

func TestSelectByLRU(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-1 * time.Hour)
	muchEarlier := now.Add(-2 * time.Hour)

	t.Run("empty slice", func(t *testing.T) {
		result := selectByLRU(nil, false)
		require.Nil(t, result)
	})

	t.Run("single account", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, LastUsedAt: &now}, loadInfo: &AccountLoadInfo{}},
		}
		result := selectByLRU(accounts, false)
		require.NotNil(t, result)
		require.Equal(t, int64(1), result.account.ID)
	})

	t.Run("selects least recently used", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, LastUsedAt: &now}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 2, LastUsedAt: &muchEarlier}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 3, LastUsedAt: &earlier}, loadInfo: &AccountLoadInfo{}},
		}
		result := selectByLRU(accounts, false)
		require.NotNil(t, result)
		require.Equal(t, int64(2), result.account.ID)
	})

	t.Run("nil LastUsedAt preferred over non-nil", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, LastUsedAt: &now}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 2, LastUsedAt: nil}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 3, LastUsedAt: &earlier}, loadInfo: &AccountLoadInfo{}},
		}
		result := selectByLRU(accounts, false)
		require.NotNil(t, result)
		require.Equal(t, int64(2), result.account.ID)
	})

	t.Run("multiple nil LastUsedAt random selection", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, LastUsedAt: nil, Type: "session"}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 2, LastUsedAt: nil, Type: "session"}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 3, LastUsedAt: nil, Type: "session"}, loadInfo: &AccountLoadInfo{}},
		}
		// 多次调用应该随机选择，验证结果都在候选范围内
		validIDs := map[int64]bool{1: true, 2: true, 3: true}
		for i := 0; i < 10; i++ {
			result := selectByLRU(accounts, false)
			require.NotNil(t, result)
			require.True(t, validIDs[result.account.ID], "selected ID should be one of the candidates")
		}
	})

	t.Run("multiple same LastUsedAt random selection", func(t *testing.T) {
		sameTime := now
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, LastUsedAt: &sameTime}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 2, LastUsedAt: &sameTime}, loadInfo: &AccountLoadInfo{}},
		}
		// 多次调用应该随机选择
		validIDs := map[int64]bool{1: true, 2: true}
		for i := 0; i < 10; i++ {
			result := selectByLRU(accounts, false)
			require.NotNil(t, result)
			require.True(t, validIDs[result.account.ID], "selected ID should be one of the candidates")
		}
	})

	t.Run("preferOAuth selects from OAuth accounts when multiple nil", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, LastUsedAt: nil, Type: "session"}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 2, LastUsedAt: nil, Type: AccountTypeOAuth}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 3, LastUsedAt: nil, Type: AccountTypeOAuth}, loadInfo: &AccountLoadInfo{}},
		}
		// preferOAuth 时，应该从 OAuth 类型中选择
		oauthIDs := map[int64]bool{2: true, 3: true}
		for i := 0; i < 10; i++ {
			result := selectByLRU(accounts, true)
			require.NotNil(t, result)
			require.True(t, oauthIDs[result.account.ID], "should select from OAuth accounts")
		}
	})

	t.Run("preferOAuth falls back to all when no OAuth", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, LastUsedAt: nil, Type: "session"}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 2, LastUsedAt: nil, Type: "session"}, loadInfo: &AccountLoadInfo{}},
		}
		// 没有 OAuth 时，从所有候选中选择
		validIDs := map[int64]bool{1: true, 2: true}
		for i := 0; i < 10; i++ {
			result := selectByLRU(accounts, true)
			require.NotNil(t, result)
			require.True(t, validIDs[result.account.ID])
		}
	})

	t.Run("preferOAuth only affects same LastUsedAt accounts", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, LastUsedAt: &earlier, Type: "session"}, loadInfo: &AccountLoadInfo{}},
			{account: &Account{ID: 2, LastUsedAt: &now, Type: AccountTypeOAuth}, loadInfo: &AccountLoadInfo{}},
		}
		result := selectByLRU(accounts, true)
		require.NotNil(t, result)
		// 有不同 LastUsedAt 时，按时间选择最早的，不受 preferOAuth 影响
		require.Equal(t, int64(1), result.account.ID)
	})
}

func TestSelectByLRUWithAffinity(t *testing.T) {
	accounts := []accountWithLoad{
		{account: &Account{ID: 1, LastUsedAt: nil}, loadInfo: &AccountLoadInfo{}},
		{account: &Account{ID: 2, LastUsedAt: nil}, loadInfo: &AccountLoadInfo{}},
		{account: &Account{ID: 3, LastUsedAt: nil}, loadInfo: &AccountLoadInfo{}},
	}

	first := selectByLRUWithAffinity(accounts, false, "sub:42|device:a|session:s1")
	second := selectByLRUWithAffinity(accounts, false, "sub:42|device:a|session:s1")
	require.NotNil(t, first)
	require.NotNil(t, second)
	require.Equal(t, first.account.ID, second.account.ID)

	expectedIdx := stableAffinityIndex("sub:42|device:a|session:s1", len(accounts))
	require.Equal(t, accounts[expectedIdx].account.ID, first.account.ID)
}

func TestSchedulerAffinitySeed(t *testing.T) {
	metadata := FormatMetadataUserID(
		"a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
		"",
		"123e4567-e89b-42d3-a456-426614174000",
		"2.1.161",
	)

	seed := schedulerAffinitySeed(metadata, 99, "fallback-session")
	require.Contains(t, seed, "sub:99")
	require.Contains(t, seed, "device:a1b2c3d4")
	require.Contains(t, seed, "session:123e4567-e89b-42d3-a456-426614174000")

	require.Equal(t, "sub:99|session:fallback-session", schedulerAffinitySeed("", 99, "fallback-session"))
}

type sessionLimitCacheForSchedulerFilterTest struct {
	SessionLimitCache
	counts map[int64]int
}

func (s sessionLimitCacheForSchedulerFilterTest) GetActiveSessionCountBatch(ctx context.Context, accountIDs []int64, idleTimeouts map[int64]time.Duration) (map[int64]int, error) {
	out := make(map[int64]int, len(accountIDs))
	for _, id := range accountIDs {
		out[id] = s.counts[id]
	}
	return out, nil
}

func TestFilterByMinActiveSessions(t *testing.T) {
	svc := &GatewayService{sessionLimitCache: sessionLimitCacheForSchedulerFilterTest{
		counts: map[int64]int{1: 3, 2: 1, 3: 1},
	}}
	accounts := []accountWithLoad{
		{account: &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeOAuth, Extra: map[string]any{"max_sessions": 5}}, loadInfo: &AccountLoadInfo{}},
		{account: &Account{ID: 2, Platform: PlatformAnthropic, Type: AccountTypeOAuth, Extra: map[string]any{"max_sessions": 5}}, loadInfo: &AccountLoadInfo{}},
		{account: &Account{ID: 3, Platform: PlatformAnthropic, Type: AccountTypeOAuth, Extra: map[string]any{"max_sessions": 5}}, loadInfo: &AccountLoadInfo{}},
	}

	result := svc.filterByMinActiveSessions(context.Background(), accounts)
	require.Len(t, result, 2)
	require.Equal(t, int64(2), result[0].account.ID)
	require.Equal(t, int64(3), result[1].account.ID)
}

func TestLayeredFilterIntegration(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-1 * time.Hour)
	muchEarlier := now.Add(-2 * time.Hour)

	t.Run("full layered selection", func(t *testing.T) {
		// 模拟真实场景：多个账号，不同优先级、负载率、最后使用时间
		accounts := []accountWithLoad{
			// 优先级 1，负载 50%
			{account: &Account{ID: 1, Priority: 1, LastUsedAt: &now}, loadInfo: &AccountLoadInfo{LoadRate: 50}},
			// 优先级 1，负载 20%（最低）
			{account: &Account{ID: 2, Priority: 1, LastUsedAt: &earlier}, loadInfo: &AccountLoadInfo{LoadRate: 20}},
			// 优先级 1，负载 20%（最低），更早使用
			{account: &Account{ID: 3, Priority: 1, LastUsedAt: &muchEarlier}, loadInfo: &AccountLoadInfo{LoadRate: 20}},
			// 优先级 2（较低优先）
			{account: &Account{ID: 4, Priority: 2, LastUsedAt: &muchEarlier}, loadInfo: &AccountLoadInfo{LoadRate: 0}},
		}

		// 1. 取优先级最小的集合 → ID: 1, 2, 3
		step1 := filterByMinPriority(accounts)
		require.Len(t, step1, 3)

		// 2. 取负载率最低的集合 → ID: 2, 3
		step2 := filterByMinLoadRate(step1)
		require.Len(t, step2, 2)

		// 3. LRU 选择 → ID: 3（muchEarlier 最早）
		selected := selectByLRU(step2, false)
		require.NotNil(t, selected)
		require.Equal(t, int64(3), selected.account.ID)
	})

	t.Run("all same priority and load rate", func(t *testing.T) {
		accounts := []accountWithLoad{
			{account: &Account{ID: 1, Priority: 1, LastUsedAt: &now}, loadInfo: &AccountLoadInfo{LoadRate: 50}},
			{account: &Account{ID: 2, Priority: 1, LastUsedAt: &earlier}, loadInfo: &AccountLoadInfo{LoadRate: 50}},
			{account: &Account{ID: 3, Priority: 1, LastUsedAt: &muchEarlier}, loadInfo: &AccountLoadInfo{LoadRate: 50}},
		}

		step1 := filterByMinPriority(accounts)
		require.Len(t, step1, 3)

		step2 := filterByMinLoadRate(step1)
		require.Len(t, step2, 3)

		// LRU 选择最早的
		selected := selectByLRU(step2, false)
		require.NotNil(t, selected)
		require.Equal(t, int64(3), selected.account.ID)
	})
}
