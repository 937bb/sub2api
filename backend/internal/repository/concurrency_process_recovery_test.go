package repository

import (
	"context"
	"fmt"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestProcessRecoveryOnlyRemovesExactDeadOwner(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()
	c := NewConcurrencyCache(client, 30, 1800).(*concurrencyCache)
	ctx := context.Background()
	keys := []string{accountSlotKey(10), userSlotKey(2), apiKeySlotKeyPrefix + "1", openAIWSIngressLeaseKeyPrefix + "1"}
	for _, key := range keys {
		for i := 0; i < 520; i++ {
			require.NoError(t, client.ZAdd(ctx, key, redis.Z{Score: 1, Member: fmt.Sprintf("rdead-%d", i)}).Err())
		}
		require.NoError(t, client.ZAdd(ctx, key, redis.Z{Score: 1, Member: "ralive-1"}, redis.Z{Score: 1, Member: "rdeadother-2"}).Err())
	}
	require.NoError(t, client.Set(ctx, "wait:account:10", 7, 0).Err())
	require.NoError(t, client.ZAdd(ctx, accountActiveIndexKey, redis.Z{Score: 1, Member: "10"}).Err())
	require.Error(t, c.CleanupProcessSlots(ctx, "*"))
	require.NoError(t, c.CleanupProcessSlots(ctx, "rdead"))
	for _, key := range keys {
		values, err := client.ZRange(ctx, key, 0, -1).Result()
		require.NoError(t, err)
		require.ElementsMatch(t, []string{"ralive-1", "rdeadother-2"}, values)
	}
	require.Equal(t, "7", client.Get(ctx, "wait:account:10").Val())
	require.EqualValues(t, 1, client.ZCard(ctx, accountActiveIndexKey).Val())
	require.NoError(t, c.CleanupProcessSlots(ctx, "rdead"))
}
