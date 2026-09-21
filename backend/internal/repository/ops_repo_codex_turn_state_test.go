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

func TestNormalizeOpenAICodexTurnStateFilter(t *testing.T) {
	filter := normalizeOpenAICodexTurnStateFilter(&service.OpenAICodexTurnStateFilter{
		Status:   " EXPIRED ",
		Page:     0,
		PageSize: 999,
	})
	require.Equal(t, "expired", filter.Status)
	require.Equal(t, 1, filter.Page)
	require.Equal(t, 200, filter.PageSize)

	defaultFilter := normalizeOpenAICodexTurnStateFilter(&service.OpenAICodexTurnStateFilter{Status: "invalid"})
	require.Equal(t, "active", defaultFilter.Status)
	require.Equal(t, 1, defaultFilter.Page)
	require.Equal(t, 20, defaultFilter.PageSize)
}

func TestBuildOpenAICodexTurnStateWhere(t *testing.T) {
	accountID := int64(42)
	where, args := buildOpenAICodexTurnStateWhere(&service.OpenAICodexTurnStateFilter{
		Status:    "expired",
		AccountID: &accountID,
	})
	require.Equal(t, "WHERE c.expires_at <= NOW() AND c.source_account_id = $1", where)
	require.Equal(t, []any{int64(42)}, args)

	where, args = buildOpenAICodexTurnStateWhere(&service.OpenAICodexTurnStateFilter{Status: "all"})
	require.Empty(t, where)
	require.Empty(t, args)
}

func TestDeleteOpenAICodexTurnStatesExpiredBefore(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	cutoff := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM codex_turn_states WHERE expires_at <= $1")).
		WithArgs(cutoff).
		WillReturnResult(sqlmock.NewResult(0, 17))

	repo := &opsRepository{db: db}
	require.NoError(t, repo.DeleteOpenAICodexTurnStatesExpiredBefore(context.Background(), cutoff))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertOpenAICodexTurnStatePersistsAccountModelAndIssuedAt(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	accountID := int64(42)
	issuedAt := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	record := &service.OpenAICodexTurnStateRecord{
		StateValue:        "state",
		StateHash:         "hash",
		ValueLength:       292,
		SourceAccountID:   &accountID,
		SourceSessionHash: "session",
		SourceSessionID:   "harvest-session",
		RouteIPv6:         "2a02:ae02:1a:2c00::1234",
		SourceModel:       "gpt-5.6-codex",
		SourceTransport:   "scanner",
		IssuedAt:          issuedAt,
		FirstSeenAt:       issuedAt.Add(time.Minute),
		LastSeenAt:        issuedAt.Add(2 * time.Minute),
		ExpiresAt:         issuedAt.Add(time.Hour),
	}
	mock.ExpectExec(regexp.QuoteMeta(upsertOpenAICodexTurnStateSQL)).
		WithArgs(
			record.StateValue,
			record.StateHash,
			record.ValueLength,
			record.SourceAccountID,
			record.SourceSessionHash,
			record.SourceSessionID,
			record.SourceProxyID,
			record.SourceProxyURL,
			record.SourceExitIP,
			record.RouteIPv6,
			record.SourceModel,
			record.SourceTransport,
			record.IssuedAt,
			record.FirstSeenAt,
			record.LastSeenAt,
			record.ExpiresAt,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	repo := &opsRepository{db: db}
	require.NoError(t, repo.UpsertOpenAICodexTurnState(context.Background(), record))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertOpenAICodexTurnStateDoesNotMoveStateAcrossAccountModelScope(t *testing.T) {
	require.Contains(t, upsertOpenAICodexTurnStateSQL,
		"EXCLUDED.source_account_id IS NOT DISTINCT FROM codex_turn_states.source_account_id")
	require.Contains(t, upsertOpenAICodexTurnStateSQL,
		"EXCLUDED.source_model IS NOT DISTINCT FROM codex_turn_states.source_model")
}

func TestLoadPreferredOpenAICodexTurnStateUsesBucketAndConfiguredOrder(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	now := time.Now().UTC().Truncate(time.Second)
	issuedAt := now.Add(-10 * time.Minute)
	query := `(?s)SELECT id, state_value.*WHERE source_account_id = \$1 AND source_model = \$2.*value_length = ANY\(\$3::integer\[\]\) AND expires_at > \$4.*issued_at <= \$4 AND issued_at \+ INTERVAL '1 hour' > \$4.*source_transport = 'scanner' AND source_session_id IS NOT NULL.*ORDER BY array_position\(\$3::integer\[\], value_length\).*LIMIT 1`
	columns := []string{"id", "state_value", "state_hash", "value_length", "source_account_id", "source_session_hash", "source_session_id", "source_proxy_id", "source_proxy_url", "source_exit_ip", "route_ipv6", "source_model", "source_transport", "issued_at", "first_seen_at", "last_seen_at", "expires_at"}
	mock.ExpectQuery(query).
		WithArgs(int64(42), "gpt-5.5", "{356,292}", now).
		WillReturnRows(sqlmock.NewRows(columns).AddRow(3, "state", "hash", 356, 42, "session", "harvest-session", nil, "http://proxy.example:8080", "8.8.8.8", "2a02:ae02:1a:2c00::1234", "gpt-5.5", "scanner", issuedAt, issuedAt, now, issuedAt.Add(time.Hour)))
	repo := &opsRepository{db: db}
	record, err := repo.LoadPreferredOpenAICodexTurnState(context.Background(), 42, " GPT-5.5 ", []int{356, 292}, now)
	require.NoError(t, err)
	require.Equal(t, int64(42), *record.SourceAccountID)
	require.Equal(t, "gpt-5.5", record.SourceModel)
	require.Equal(t, "2a02:ae02:1a:2c00::1234", record.RouteIPv6)
	require.Equal(t, issuedAt.Add(time.Hour), record.ExpiresAt)
	mock.ExpectQuery(query).
		WithArgs(int64(84), "gpt-5.5", "{356,292}", now).
		WillReturnRows(sqlmock.NewRows(columns))
	record, err = repo.LoadPreferredOpenAICodexTurnState(context.Background(), 84, "gpt-5.5", []int{356, 292}, now)
	require.NoError(t, err)
	require.Nil(t, record)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertOpenAICodexTurnStateConflictsRequireSamePersistedBucket(t *testing.T) {
	for _, alreadyStored := range []bool{true, false} {
		name := "foreign_or_incompatible_row"
		if alreadyStored {
			name = "same_bucket_newer_row"
		}
		t.Run(name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			now := time.Now().UTC().Truncate(time.Second)
			accountID := int64(42)
			record := &service.OpenAICodexTurnStateRecord{
				StateValue: "state", StateHash: "hash", ValueLength: 332,
				SourceAccountID: &accountID, SourceModel: "gpt-5.5", SourceTransport: "http",
				IssuedAt: now, FirstSeenAt: now, LastSeenAt: now, ExpiresAt: now.Add(time.Hour),
			}
			mock.ExpectExec(regexp.QuoteMeta(upsertOpenAICodexTurnStateSQL)).WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectQuery(`(?s)SELECT EXISTS.*state_hash = \$1 AND source_account_id IS NOT DISTINCT FROM \$2::bigint.*source_model IS NOT DISTINCT FROM NULLIF\(\$3::text, ''\).*last_seen_at >= \$6.*issued_at IS NOT DISTINCT FROM \$7::timestamptz AND expires_at >= \$8`).
				WithArgs("hash", accountID, "gpt-5.5", "state", 332, now, now, now.Add(time.Hour)).
				WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(alreadyStored))
			err = (&opsRepository{db: db}).UpsertOpenAICodexTurnState(context.Background(), record)
			if alreadyStored {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "conflicting record")
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
