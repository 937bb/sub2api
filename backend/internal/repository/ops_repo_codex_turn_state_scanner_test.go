package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
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
	require.Contains(t, query, "c.value_length = ANY($4::integer[]) AND c.expires_at > NOW()")
	require.Contains(t, query, "c.issued_at <= NOW() AND c.issued_at + INTERVAL '1 hour' > NOW()")
	require.Contains(t, query, "ORDER BY array_position($4::integer[], c.value_length), c.expires_at DESC, c.last_seen_at DESC, c.state_hash DESC LIMIT 1")
}

func TestCodexTurnStateScanUpsertRejectsLostLease(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectExec(`(?s)INSERT INTO codex_turn_state_scans.*owned.lease_id = \$12 AND owned.lease_until > NOW\(\).*ON CONFLICT.*codex_turn_state_scans.lease_id = \$12 AND codex_turn_state_scans.lease_until > NOW\(\)`).
		WithArgs(int64(42), "gpt-5.5", "ready", 1, nil, "", 332, "", nil, nil, nil, "previous-owner").
		WillReturnResult(sqlmock.NewResult(0, 0))
	repo := &opsRepository{db: db}
	err = repo.UpsertOpenAICodexTurnStateScan(context.Background(), &service.OpenAICodexTurnStateScan{
		AccountID: 42, Model: "gpt-5.5", Status: "ready", AttemptCount: 1, LastStateLength: 332, LeaseID: "previous-owner",
	})
	require.ErrorContains(t, err, "lease lost")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCodexTurnStateStatusQueriesBindConfiguredLengths(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectQuery(`(?s)WITH eligible_accounts.*array_position\(\$4::integer\[\], c.value_length\).*SELECT COUNT`).
		WithArgs(sqlmock.AnyArg(), "{42}", `{"gpt-5.5"}`, "{356,292}").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`(?s)WITH eligible_accounts.*LIMIT \$5 OFFSET \$6`).
		WithArgs(sqlmock.AnyArg(), "{42}", `{"gpt-5.5"}`, "{356,292}", 20, 0).
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "account_name", "account_type", "plan_type", "model", "effective_status", "state_length", "issued_at", "expires_at", "last_attempt_at", "last_success_at", "attempt_count", "last_proxy_id", "last_proxy_url", "last_error"}))
	repo := &opsRepository{db: db}
	_, err = repo.ListOpenAICodexTurnStateAccountStatuses(context.Background(), []int64{42}, []string{"gpt-5.5"}, []int{356, 292}, 1, 20)
	require.NoError(t, err)
	mock.ExpectQuery(`(?s)WITH eligible.*c.value_length = ANY\(\$3::integer\[\]\).*array_position\(\$3::integer\[\], c.value_length\).*selected.expires_at > NOW\(\) \+ INTERVAL '5 minutes'`).
		WithArgs(sqlmock.AnyArg(), `{"gpt-5.5"}`, "{356,292}").
		WillReturnRows(sqlmock.NewRows([]string{"oauth_accounts", "ready_accounts", "missing_accounts", "ready_model_slots", "total_model_slots", "running_jobs", "enabled_proxies", "healthy_proxies", "shared_proxies", "last_scan_at"}).AddRow(1, 0, 1, 0, 1, 0, 0, 0, 0, nil))
	_, err = repo.GetOpenAICodexTurnStateOperationsSummary(context.Background(), []string{"gpt-5.5"}, []int{356, 292})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
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
