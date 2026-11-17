package repository

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestGatewayCacheReserveOpenAIWSDialSlotSharedAcrossInstances(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	first := &gatewayCache{rdb: client}
	second := &gatewayCache{rdb: client}
	interval := time.Second

	wait, err := first.ReserveOpenAIWSDialSlot(context.Background(), 9001, interval)
	require.NoError(t, err)
	require.Zero(t, wait)

	wait, err = second.ReserveOpenAIWSDialSlot(context.Background(), 9001, interval)
	require.NoError(t, err)
	require.GreaterOrEqual(t, wait, 900*time.Millisecond)
	require.LessOrEqual(t, wait, interval)

	wait, err = second.ReserveOpenAIWSDialSlot(context.Background(), 9002, interval)
	require.NoError(t, err)
	require.Zero(t, wait)
}
