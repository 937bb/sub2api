package repository

import (
	"context"
	"database/sql/driver"
	"regexp"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestContentModerationRepositoryEngineMetaInsert(t *testing.T) {
	for _, meta := range []*service.ContentModerationEngineMeta{nil, {Engine: "typesafe", Model: "jev-fixed", RulesVersion: "rules-v1", SkippedImages: 1}} {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		args := make([]driver.Value, 26)
		for i := range args {
			args[i] = sqlmock.AnyArg()
		}
		if meta == nil {
			args[25] = nil
		} else {
			args[25] = `{"engine":"typesafe","model":"jev-fixed","rules_version":"rules-v1","skipped_images":1}`
		}
		mock.ExpectQuery("INSERT INTO content_moderation_logs").WithArgs(args...).WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(1, time.Now()))
		err = NewContentModerationRepository(db).CreateLog(context.Background(), &service.ContentModerationLog{EngineMeta: meta})
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
		mock.ExpectClose()
		require.NoError(t, db.Close())
	}
}

func TestContentModerationRepositoryEngineMetaRead(t *testing.T) {
	for _, meta := range []any{nil, `{"engine":"typesafe","model":"jev-fixed","rules_version":"rules-v1","skipped_images":1}`} {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		columns := []string{"id", "request_id", "user_id", "user_email", "api_key_id", "api_key_name", "group_id", "group_name", "endpoint", "provider", "model", "mode", "action", "flagged", "highest_category", "highest_score", "category_scores", "threshold_snapshot", "input_excerpt", "upstream_latency_ms", "error", "violation_count", "auto_banned", "email_sent", "status", "queue_delay_ms", "matched_keyword", "created_at", "engine_meta"}
		mock.ExpectQuery("SELECT[\\s\\S]*l.engine_meta").WillReturnRows(sqlmock.NewRows(columns).AddRow(1, "req", nil, "", nil, "", nil, "", "/v1/responses", "business", "gpt-model", "observe", "allow", false, "sexual", 0.1, `{"sexual":0.1}`, `{"sexual":0.8}`, "sample", 10, "", 0, false, false, "active", nil, "", time.Now(), meta))
		logs, _, err := NewContentModerationRepository(db).ListLogs(context.Background(), service.ContentModerationLogFilter{})
		require.NoError(t, err)
		require.Len(t, logs, 1)
		require.Equal(t, "gpt-model", logs[0].Model)
		if meta == nil {
			require.Nil(t, logs[0].EngineMeta)
		} else {
			require.Equal(t, "jev-fixed", logs[0].EngineMeta.Model)
			require.Equal(t, 1, logs[0].EngineMeta.SkippedImages)
		}
		require.NoError(t, mock.ExpectationsWereMet())
		mock.ExpectClose()
		require.NoError(t, db.Close())
	}
}

func TestBuildContentModerationLogWhere_BlockedIncludesAllBlockActions(t *testing.T) {
	where, args := buildContentModerationLogWhere(service.ContentModerationLogFilter{Result: "blocked"})

	require.Empty(t, args)
	sql := strings.Join(where, " AND ")
	require.Contains(t, sql, "l.action IN ('block', 'keyword_block', 'hash_block')")
	require.NotContains(t, sql, "l.action = 'block'")
}

func TestBuildContentModerationLogWhere_FiltersProvider(t *testing.T) {
	where, args := buildContentModerationLogWhere(service.ContentModerationLogFilter{Provider: "distillation_monitor"})

	require.Contains(t, strings.Join(where, " AND "), "l.provider = $1")
	require.Equal(t, []any{"distillation_monitor"}, args)
}

func TestContentModerationRepositoryListDistillationCandidatesNormalizesRouteSuffix(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	start := time.Now().Add(-15 * time.Minute)
	end := time.Now()
	mock.ExpectQuery(`REGEXP_REPLACE\([\s\S]*s2a-route`).
		WithArgs(sqlmock.AnyArg(), start, end, 100, int64(300_000)).
		WillReturnRows(sqlmock.NewRows([]string{
			"user_id", "email", "group_id", "group_name", "dominant_model", "request_count",
			"input_tokens", "output_tokens", "cache_read_tokens", "non_stream_count", "missing_sessions",
			"distinct_sessions", "distinct_api_keys", "distinct_ips", "distinct_user_agents", "explicit_signals",
			"first_request_at", "last_request_at",
		}))
	repo := NewContentModerationRepository(db).(service.DistillationMonitorRepository)

	items, err := repo.ListDistillationCandidates(context.Background(), []int64{56}, start, end, 100, 300_000)

	require.NoError(t, err)
	require.Empty(t, items)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestContentModerationRepositoryListDistillationAssessmentsIncludesLowScores(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	now := time.Now()
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM distillation_assessments`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`FROM distillation_assessments da`).
		WithArgs(20, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "email", "status", "group_id", "group_name", "dominant_model",
			"window_start", "window_end", "score", "evidence_classes", "mode", "metrics", "auto_disabled", "created_at",
		}).AddRow(
			9, 1156, "user@example.com", "active", 56, "G", "gpt-test",
			now.Add(-15*time.Minute), now, 53, 2, "observe", `{"requests":5605}`, false, now,
		))
	repo := NewContentModerationRepository(db).(service.DistillationMonitorRepository)

	items, page, err := repo.ListDistillationAssessments(context.Background(), service.DistillationAssessmentFilter{
		Pagination: pagination.PaginationParams{Page: 1, PageSize: 20},
	})

	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, 53, items[0].Score)
	require.Equal(t, float64(5605), items[0].Metrics["requests"])
	require.Equal(t, int64(1), page.Total)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestContentModerationRepositoryCountFlaggedByUserSince_ExcludesHashBlock(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := NewContentModerationRepository(db)
	since := time.Now().Add(-time.Hour)
	mock.ExpectQuery(regexp.QuoteMeta("AND action <> 'hash_block'")).
		WithArgs(int64(1001), since, false).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	count, err := repo.CountFlaggedByUserSince(context.Background(), 1001, since, false)

	require.NoError(t, err)
	require.Equal(t, 2, count)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestContentModerationRepositoryCountFlaggedByUserSince_ExcludesCyberPolicyWhenRequested(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := NewContentModerationRepository(db)
	since := time.Now().Add(-time.Hour)
	mock.ExpectQuery(regexp.QuoteMeta("AND ($3::bool IS FALSE OR action <> 'cyber_policy')")).
		WithArgs(int64(1001), since, true).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

	count, err := repo.CountFlaggedByUserSince(context.Background(), 1001, since, true)

	require.NoError(t, err)
	require.Equal(t, 3, count)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestContentModerationRepositoryCountFlaggedByUserSince_ExcludesCyberLogOnly(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	repo := NewContentModerationRepository(db)
	since := time.Now().Add(-time.Hour)
	mock.ExpectQuery(regexp.QuoteMeta("AND mode <> 'cyber_log_only'\n  AND mode <> 'risk_control_log_only'")).
		WithArgs(int64(12), since, false).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	count, err := repo.CountFlaggedByUserSince(context.Background(), 12, since, false)
	require.NoError(t, err)
	require.Zero(t, count)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestContentModerationRepositoryCountFlaggedByUserSince_ExcludesDistillationLogs(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	repo := NewContentModerationRepository(db)
	since := time.Now().Add(-time.Hour)
	mock.ExpectQuery(regexp.QuoteMeta("AND mode <> 'distillation_observe'\n  AND mode <> 'distillation_enforce'")).
		WithArgs(int64(12), since, false).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	count, err := repo.CountFlaggedByUserSince(context.Background(), 12, since, false)

	require.NoError(t, err)
	require.Zero(t, count)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestContentModerationRepositoryCountsOnlyAdjacentEnforceAssessments(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	repo := NewContentModerationRepository(db).(service.DistillationMonitorRepository)
	window := 15 * time.Minute
	windowEnd := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery("SELECT window_end[\\s\\S]*mode = 'enforce'").
		WithArgs(int64(7), int64(8), windowEnd, 85, 3, 3).
		WillReturnRows(sqlmock.NewRows([]string{"window_end"}).
			AddRow(windowEnd).
			AddRow(windowEnd.Add(-window)).
			AddRow(windowEnd.Add(-3 * window)))

	count, err := repo.CountConsecutiveDistillationAssessments(context.Background(), 7, 8, windowEnd, window, 85, 3, 3)

	require.NoError(t, err)
	require.Equal(t, 2, count)
	require.NoError(t, mock.ExpectationsWereMet())
}
