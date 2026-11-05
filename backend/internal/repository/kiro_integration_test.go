//go:build integration

package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestKiroProxyConcurrentAssignment(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := newAccountRepositoryWithSQL(client, integrationDB, nil)
	name := fmt.Sprintf("kiro-proxy-%d", time.Now().UnixNano())
	p, err := client.Proxy.Create().SetName(name).SetProtocol("http").SetHost(name + ".invalid").SetPort(8000).SetStatus("active").SetFallbackMode("none").Save(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM scheduler_outbox WHERE account_id IN (SELECT id FROM accounts WHERE proxy_id=$1)", p.ID)
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM accounts WHERE proxy_id=$1", p.ID)
		_, _ = integrationDB.ExecContext(ctx, "DELETE FROM proxies WHERE id=$1", p.ID)
	})
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			a := &service.Account{Name: fmt.Sprint(name, i), Platform: service.PlatformKiro, Type: service.AccountTypeOAuth, Credentials: map[string]any{}, Extra: map[string]any{}, ProxyID: &p.ID, Status: service.StatusActive}
			results <- repo.Create(ctx, a)
		}(i)
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	require.Equal(t, 1, success)
	other := &service.Account{Name: name + "-other", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Credentials: map[string]any{}, Extra: map[string]any{}, ProxyID: &p.ID, Status: service.StatusActive}
	require.Error(t, repo.Create(ctx, other), "other platforms cannot reuse a Kiro endpoint")
	var id int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT id FROM accounts WHERE proxy_id=$1", p.ID).Scan(&id))
	_, err = integrationDB.ExecContext(ctx, "UPDATE accounts SET proxy_id=NULL WHERE id=$1", id)
	require.Error(t, err)
	_, err = integrationDB.ExecContext(ctx, "UPDATE proxies SET expires_at=NOW()-interval '1 minute',fallback_mode='direct' WHERE id=$1", p.ID)
	require.NoError(t, err)
	proxies := newProxyRepositoryWithSQL(client, integrationDB)
	_, err = proxies.SweepExpiredProxies(ctx, time.Now())
	require.NoError(t, err)
	var after int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT proxy_id FROM accounts WHERE id=$1", id).Scan(&after))
	require.Equal(t, p.ID, after)
}

type KiroCacheSuite struct{ IntegrationRedisSuite }

func (s *KiroCacheSuite) TestRealResponseHitTargetAndIsolation() {
	cache := NewGatewayCache(s.rdb).(service.KiroResponseCache)
	policy := service.KiroCachePolicy{Enabled: true, TargetPercent: 50, TTLSeconds: 60, MaxEntries: 2}
	hits := 0
	for i := 0; i < 10; i++ {
		value, err := cache.GetKiroResponse(s.ctx, 12345, "tenant-a", policy)
		require.NoError(s.T(), err)
		if len(value) > 0 {
			hits++
		}
		require.NoError(s.T(), cache.PutKiroResponse(s.ctx, 12345, "tenant-a", []byte("response"), policy))
	}
	require.Equal(s.T(), 5, hits)
	value, err := cache.GetKiroResponse(s.ctx, 12345, "tenant-b", policy)
	require.NoError(s.T(), err)
	require.Empty(s.T(), value)
	value, err = cache.GetKiroResponse(s.ctx, 12346, "tenant-a", policy)
	require.NoError(s.T(), err)
	require.Empty(s.T(), value)
	for _, key := range []string{"b", "c"} {
		require.NoError(s.T(), cache.PutKiroResponse(s.ctx, 12345, key, []byte("response"), policy))
	}
	_, index, _ := kiroCacheKeys(12345, "", policy)
	require.EqualValues(s.T(), 2, s.rdb.ZCard(s.ctx, index).Val())
}
func TestKiroCacheIntegration(t *testing.T) { suite.Run(t, new(KiroCacheSuite)) }
