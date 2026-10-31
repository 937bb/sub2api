//go:build integration

package repository

import (
	"context"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAccountProxyPoolPersistence(t *testing.T) {
	tx := testEntTx(t)
	ctx := dbent.NewTxContext(context.Background(), tx)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	proxies := newProxyRepositoryWithSQL(tx.Client(), tx)
	p1 := &service.Proxy{Name: "primary", Protocol: "http", Host: "127.0.0.1", Port: 8011, Status: service.StatusActive}
	p2 := &service.Proxy{Name: "backup", Protocol: "socks5", Host: "127.0.0.1", Port: 8012, Status: service.StatusActive}
	require.NoError(t, proxies.Create(ctx, p1))
	require.NoError(t, proxies.Create(ctx, p2))
	a := &service.Account{Name: "pool", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive, Concurrency: 1, Priority: 50, Credentials: map[string]any{"access_token": "test"}, ProxyID: &p1.ID, Extra: map[string]any{service.AccountProxyFallbackIDsKey: []int64{p2.ID}}}
	require.NoError(t, repo.Create(ctx, a))
	got, err := repo.GetByID(ctx, a.ID)
	require.NoError(t, err)
	require.Equal(t, p1.ID, *got.ProxyID)
	require.Len(t, got.FallbackProxies, 1)
	require.Equal(t, p2.ID, got.FallbackProxies[0].ID)
	batch, err := repo.GetByIDs(ctx, []int64{a.ID})
	require.NoError(t, err)
	require.Len(t, batch, 1)
	require.Len(t, batch[0].FallbackProxies, 1)
	count, err := proxies.CountAccountsByProxyID(ctx, p2.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	p2.Status = "inactive"
	require.NoError(t, proxies.Update(ctx, p2))
	got, err = repo.GetByID(ctx, a.ID)
	require.NoError(t, err)
	require.Equal(t, "inactive", got.FallbackProxies[0].Status)
	_, err = tx.Client().Proxy.UpdateOneID(p2.ID).SetStatus(service.StatusActive).Save(ctx)
	require.NoError(t, err)
	require.NoError(t, proxies.Delete(ctx, p2.ID))
	got, err = repo.GetByID(ctx, a.ID)
	require.NoError(t, err)
	require.Empty(t, got.FallbackProxies)
	require.Equal(t, p1.ID, *got.ProxyID)
}
