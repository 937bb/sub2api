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
		SourceModel:       "gpt-5.6-codex",
		SourceTransport:   "http",
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
