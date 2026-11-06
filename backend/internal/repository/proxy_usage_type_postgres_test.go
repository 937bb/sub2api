//go:build unit

package repository

import (
	"context"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestProxyUsagePostgresMigrationAndFiltering(t *testing.T) {
	db := scopedAdminKeyTestDB(t)
	_, err := db.Exec(`CREATE TABLE proxies(
 id BIGSERIAL PRIMARY KEY,name TEXT DEFAULT 'proxy',protocol TEXT DEFAULT 'http',host TEXT DEFAULT '192.0.2.1',port INT DEFAULT 8080,
 username TEXT DEFAULT 'user',password TEXT DEFAULT 'secret',status TEXT DEFAULT 'active',
 expires_at TIMESTAMPTZ,deleted_at TIMESTAMPTZ,created_at TIMESTAMPTZ DEFAULT NOW(),updated_at TIMESTAMPTZ DEFAULT NOW(),
 fallback_mode TEXT DEFAULT 'none',backup_proxy_id BIGINT,expiry_warn_days INT DEFAULT 7);
 CREATE TABLE accounts(id BIGSERIAL PRIMARY KEY,proxy_id BIGINT,platform TEXT,extra JSONB DEFAULT '{}',deleted_at TIMESTAMPTZ);
 INSERT INTO proxies(name) VALUES('kiro'),('mixed'),('unbound');
 INSERT INTO proxies(name,host,port,username) VALUES('ipv6','127.0.0.1',26445,'b-account-123');
 INSERT INTO accounts(proxy_id,platform) VALUES(1,'kiro'),(2,'kiro'),(2,'openai');`)
	require.NoError(t, err)
	migration, err := migrations.FS.ReadFile("252_proxy_usage_type.sql")
	require.NoError(t, err)
	for range 2 {
		_, err = db.Exec(string(migration))
		require.NoError(t, err)
	}
	var types string
	require.NoError(t, db.QueryRow(`SELECT string_agg(usage_type,',' ORDER BY id) FROM proxies`).Scan(&types))
	require.Equal(t, "kiro,general,general,pro", types)
	_, err = db.Exec(`UPDATE proxies SET usage_type='pro' WHERE id=1`)
	require.NoError(t, err)
	_, err = db.Exec(string(migration))
	require.NoError(t, err)
	require.NoError(t, db.QueryRow(`SELECT usage_type FROM proxies WHERE id=1`).Scan(&types))
	require.Equal(t, "pro", types)
	_, err = db.Exec(`UPDATE proxies SET usage_type='kiro' WHERE id=1`)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE proxies SET usage_type='invalid' WHERE id=1`)
	require.Error(t, err)

	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	repo := newProxyRepositoryWithSQL(client, db)
	ctx := context.Background()
	route := &service.Proxy{Name: "new ipv6", Host: "127.0.0.1", Port: 26445, Protocol: "http", Username: "b-account-456", Status: service.StatusActive}
	require.NoError(t, repo.Create(ctx, route))
	require.Equal(t, "pro", route.UsageType)
	stored, err := repo.GetByID(ctx, route.ID)
	require.NoError(t, err)
	require.Equal(t, "pro", stored.UsageType)
	for _, kind := range []string{"kiro", "general", "pro"} {
		expected := int64(2)
		if kind == "kiro" {
			expected = 1
		}
		params := pagination.PaginationParams{Page: 1, PageSize: 1, SortBy: "account_count", SortOrder: "desc"}
		items, page, err := repo.ListWithFilters(ctx, params, "", "", "", kind)
		require.NoError(t, err)
		require.Equal(t, expected, page.Total)
		require.Len(t, items, 1)
		require.Equal(t, kind, items[0].UsageType)
		counted, page, err := repo.ListWithFiltersAndAccountCount(ctx, params, "", "", "", kind)
		require.NoError(t, err)
		require.Equal(t, expected, page.Total)
		require.Len(t, counted, 1)
		require.Equal(t, kind, counted[0].UsageType)
		options, page, err := repo.ListOptions(ctx, params, "", kind)
		require.NoError(t, err)
		require.Equal(t, expected, page.Total)
		require.Len(t, options, 1)
		require.Equal(t, kind, options[0].UsageType)
		require.Empty(t, options[0].Username)
		require.Empty(t, options[0].Password)
		samples, total, err := repo.SampleOptions(ctx, "", kind)
		require.NoError(t, err)
		require.Equal(t, expected, total)
		for _, sample := range samples {
			require.Equal(t, kind, sample.UsageType)
			require.Empty(t, sample.Username)
			require.Empty(t, sample.Password)
		}
	}
}
