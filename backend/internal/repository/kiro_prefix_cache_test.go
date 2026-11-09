package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestKiroPrefixRedisSharingExpiryAndIsolation(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	a := NewGatewayCache(client).(service.KiroPrefixCache)
	b := NewGatewayCache(client).(service.KiroPrefixCache)
	ctx := context.Background()
	policy := service.KiroCachePolicy{Enabled: true, Mode: "prefix", AutoPrefix: true, TargetPercent: 90, TTLSeconds: 300, MaxEntries: 2}
	plan := kiro.PlanPrefix([]byte(`{"messages":[{"role":"user","content":"hello"}]}`), "model", true, 300)
	match, err := a.LookupKiroPrefix(ctx, 7, "tenant:key:account", plan, policy)
	require.NoError(t, err)
	require.Nil(t, match)
	require.NoError(t, a.CommitKiroPrefix(ctx, 7, "tenant:key:account", plan, nil, policy))
	match, err = b.LookupKiroPrefix(ctx, 7, "tenant:key:account", plan, policy)
	require.NoError(t, err)
	require.Equal(t, plan.Writes[0], *match)
	for _, scope := range []string{"other:key:account", "tenant:other:account", "tenant:key:other"} {
		hit, err := b.LookupKiroPrefix(ctx, 7, scope, plan, policy)
		require.NoError(t, err)
		require.Nil(t, hit)
	}
	hit, err := b.LookupKiroPrefix(ctx, 8, "tenant:key:account", plan, policy)
	require.NoError(t, err)
	require.Nil(t, hit)
	server.FastForward(250 * time.Second)
	require.NoError(t, b.CommitKiroPrefix(ctx, 7, "tenant:key:account", &kiro.PrefixPlan{}, match, policy))
	server.FastForward(100 * time.Second)
	hit, err = a.LookupKiroPrefix(ctx, 7, "tenant:key:account", plan, policy)
	require.NoError(t, err)
	require.NotNil(t, hit, "successful read renews its own TTL")
	server.FastForward(201 * time.Second)
	hit, err = a.LookupKiroPrefix(ctx, 7, "tenant:key:account", plan, policy)
	require.NoError(t, err)
	require.Nil(t, hit)
	for _, scope := range []string{"a", "b", "c"} {
		require.NoError(t, a.CommitKiroPrefix(ctx, 7, scope, plan, nil, policy))
	}
	_, index := kiroPrefixKeys(7, "", policy)
	require.EqualValues(t, 2, client.ZCard(ctx, index).Val(), "group capacity is shared across accounts")
}
