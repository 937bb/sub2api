package repository

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestBuildContentModerationLogWhere_BlockedIncludesAllBlockActions(t *testing.T) {
	where, args := buildContentModerationLogWhere(service.ContentModerationLogFilter{Result: "blocked"})

	require.Empty(t, args)
	sql := strings.Join(where, " AND ")
	require.Contains(t, sql, "l.action IN ('block', 'keyword_block', 'hash_block')")
	require.NotContains(t, sql, "l.action = 'block'")
}

func TestContentModerationRepositoryCreateLog_PersistsMatchedKeyword(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := NewContentModerationRepository(db)
	log := &service.ContentModerationLog{
		RequestID:         "req-keyword",
		Action:            service.ContentModerationActionKeywordBlock,
		Flagged:           true,
		MatchedKeyword:    "secret-token",
		CategoryScores:    map[string]float64{"keyword": 1},
		ThresholdSnapshot: map[string]float64{},
	}
	mock.ExpectQuery(`(?s)INSERT INTO content_moderation_logs .*matched_keyword.*\$25`).
		WithArgs(
			"req-keyword", nil, "", nil, "", nil, "",
			"", "", "", "", service.ContentModerationActionKeywordBlock, true, "", float64(0),
			`{"keyword":1}`, `{}`, "", nil, "",
			0, false, false, nil, "secret-token",
		).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(int64(42), time.Now()))

	err = repo.CreateLog(context.Background(), log)

	require.NoError(t, err)
	require.Equal(t, int64(42), log.ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestContentModerationRepositoryCountFlaggedByUserSince_ExcludesHashBlock(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := NewContentModerationRepository(db)
	since := time.Now().Add(-time.Hour)
	mock.ExpectQuery(regexp.QuoteMeta("AND action <> 'hash_block'")).
		WithArgs(int64(1001), since).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	count, err := repo.CountFlaggedByUserSince(context.Background(), 1001, since)

	require.NoError(t, err)
	require.Equal(t, 2, count)
	require.NoError(t, mock.ExpectationsWereMet())
}
