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
	require.Contains(t, query, "status = 'active' AND schedulable IS TRUE")
	require.Contains(t, query, "$2::bigint[] IS NOT NULL")
	require.Contains(t, query, "status = 'active' AND schedulable IS TRUE AND last_used_at >= $1")
	require.Contains(t, query, "$2::bigint[] IS NULL OR id = ANY($2)")
	require.Contains(t, query, "UNNEST($3::text[])")
	require.Contains(t, query, "CROSS JOIN target_models")
	require.Contains(t, query, "u.created_at >= $1")
	require.Contains(t, query, "c.last_seen_at >= $1")
	require.Contains(t, query, "sc.updated_at >= $1")
	require.Contains(t, query, "LIKE 'gpt-5%'")
	require.NotContains(t, query, "SELECT a.id, 'gpt-5.5'")
}

func TestCodexTurnStateAccountStatusCTEIncludesExplicitUnavailableAccounts(t *testing.T) {
	query := codexTurnStateAccountStatusCTE()
	require.Regexp(t, `(?s)AND \(\s*\$2::bigint\[\] IS NOT NULL\s*OR \(\s*status = 'active'`, query)
}

func TestCodexTurnStateAccountStatusCTESelectsNewestExpiryWithinAccountAndModel(t *testing.T) {
	query := codexTurnStateAccountStatusCTE()
	require.Contains(t, query, "c.source_account_id = a.id AND c.source_model = am.model")
	require.Contains(t, query, "c.value_length IN (292, 332) AND c.expires_at > NOW()")
	require.Contains(t, query, "ORDER BY c.value_length DESC, c.expires_at DESC, c.last_seen_at DESC, c.id DESC LIMIT 1")
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
