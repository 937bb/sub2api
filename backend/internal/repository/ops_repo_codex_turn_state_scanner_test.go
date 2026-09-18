package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestCodexTurnStateAccountStatusCTEUsesRecentSchedulableAccountsAndModels(t *testing.T) {
	query := codexTurnStateAccountStatusCTE()
	require.Contains(t, query, "status = 'active' AND schedulable IS TRUE AND last_used_at >= $1")
	require.Contains(t, query, "u.created_at >= $1")
	require.Contains(t, query, "c.last_seen_at >= $1")
	require.Contains(t, query, "sc.updated_at >= $1")
	require.NotContains(t, query, "SELECT id AS account_id, 'gpt-5.5'")
}

func TestListRecentlyUsedOpenAICodexAccountIDs(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	usedSince := time.Date(2026, 9, 18, 17, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id\nFROM accounts") + ".*" + regexp.QuoteMeta("last_used_at >= $1")).
		WithArgs(usedSince).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(42)).AddRow(int64(84)))

	repo := &opsRepository{db: db}
	ids, err := repo.ListRecentlyUsedOpenAICodexAccountIDs(context.Background(), usedSince)
	require.NoError(t, err)
	require.Equal(t, []int64{42, 84}, ids)
	require.NoError(t, mock.ExpectationsWereMet())
}
