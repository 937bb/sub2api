package repository

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestCodexTurnStateSharedCacheTTL(t *testing.T) {
	mr := miniredis.RunT(t)
	first := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = first.Close() }()
	second := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = second.Close() }()
	producer, consumer := &gatewayCache{rdb: first}, &gatewayCache{rdb: second}
	ctx := context.Background()
	require.NoError(t, producer.BindOpenAICodexTurnStateOwner(ctx, "digest-a", "owner-a", time.Minute))
	require.NoError(t, producer.BindOpenAICodexTurnStateOwner(ctx, "digest-b", "owner-b", time.Minute))
	got, err := consumer.GetOpenAICodexTurnStateOwner(ctx, "digest-a")
	require.NoError(t, err)
	require.Equal(t, "owner-a", got)
	got, err = consumer.GetOpenAICodexTurnStateOwner(ctx, "digest-b")
	require.NoError(t, err)
	require.Equal(t, "owner-b", got)
	mr.FastForward(2 * time.Minute)
	got, err = consumer.GetOpenAICodexTurnStateOwner(ctx, "digest-a")
	require.NoError(t, err)
	require.Empty(t, got)
	require.NoError(t, producer.BindOpenAICodexTurnStateOwner(ctx, "bad", "owner", 0))
	require.False(t, mr.Exists(codexTurnStateOwnerPrefix+"bad"))
}
