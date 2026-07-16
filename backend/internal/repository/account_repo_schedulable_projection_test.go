package repository

import (
	"context"
	"fmt"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

type captureProjectionQueryMatcher struct {
	actual *string
}

func (m captureProjectionQueryMatcher) Match(_, actual string) error {
	if m.actual == nil {
		return fmt.Errorf("query capture target is nil")
	}
	*m.actual = actual
	return nil
}

func TestListSchedulableAccountLoadsUsesNarrowProjection(t *testing.T) {
	var capturedSQL string
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(captureProjectionQueryMatcher{actual: &capturedSQL}))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	driver := entsql.OpenDB(dialect.Postgres, db)
	client := dbent.NewClient(dbent.Driver(driver))
	t.Cleanup(func() { _ = client.Close() })
	repo := newAccountRepositoryWithSQL(client, db, nil)

	mock.ExpectQuery("schedulable projection").
		WillReturnRows(sqlmock.NewRows([]string{"id", "concurrency", "load_factor"}).
			AddRow(int64(11), 3, nil).
			AddRow(int64(12), 2, 0).
			AddRow(int64(13), 2, 7))

	loads, err := repo.ListSchedulableAccountLoads(context.Background())
	require.NoError(t, err)
	require.Equal(t, []int{3, 2, 7}, []int{loads[0].MaxConcurrency, loads[1].MaxConcurrency, loads[2].MaxConcurrency})
	require.NoError(t, mock.ExpectationsWereMet())

	normalized := normalizeSQLWhitespace(capturedSQL)
	selectClause, _, found := strings.Cut(normalized, " FROM ")
	require.True(t, found, "unexpected SQL: %s", normalized)
	require.Equal(t, 2, strings.Count(selectClause, ","), "projection must select exactly three columns: %s", selectClause)
	for _, column := range []string{`"id"`, `"concurrency"`, `"load_factor"`} {
		require.Contains(t, selectClause, column)
	}
	for _, forbidden := range []string{"credentials", "extra", "proxy_id", "account_groups", "proxies"} {
		require.NotContains(t, normalized, forbidden)
	}
	for _, predicate := range []string{"status", "schedulable", "temp_unschedulable_until", "expires_at", "auto_pause_on_expired", "overload_until", "rate_limit_reset_at", "deleted_at"} {
		require.Contains(t, normalized, predicate)
	}
	require.Contains(t, normalized, `ORDER BY "accounts"."priority" ASC`)
}
