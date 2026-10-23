package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type contentModerationRepository struct {
	db *sql.DB
}

func NewContentModerationRepository(db *sql.DB) service.ContentModerationRepository {
	return &contentModerationRepository{db: db}
}

func (r *contentModerationRepository) CreateLog(ctx context.Context, log *service.ContentModerationLog) error {
	if log == nil {
		return nil
	}
	categoryScores, err := json.Marshal(log.CategoryScores)
	if err != nil {
		return fmt.Errorf("marshal moderation category scores: %w", err)
	}
	thresholdSnapshot, err := json.Marshal(log.ThresholdSnapshot)
	if err != nil {
		return fmt.Errorf("marshal moderation thresholds: %w", err)
	}
	var engineMeta any
	if log.EngineMeta != nil {
		raw, err := json.Marshal(log.EngineMeta)
		if err != nil {
			return fmt.Errorf("marshal moderation engine metadata: %w", err)
		}
		engineMeta = string(raw)
	}
	var userID any
	if log.UserID != nil {
		userID = *log.UserID
	}
	var apiKeyID any
	if log.APIKeyID != nil {
		apiKeyID = *log.APIKeyID
	}
	var groupID any
	if log.GroupID != nil {
		groupID = *log.GroupID
	}
	var latency any
	if log.UpstreamLatencyMS != nil {
		latency = *log.UpstreamLatencyMS
	}
	err = r.db.QueryRowContext(ctx, `
INSERT INTO content_moderation_logs (
    request_id, user_id, user_email, api_key_id, api_key_name, group_id, group_name,
    endpoint, provider, model, mode, action, flagged, highest_category, highest_score,
    category_scores, threshold_snapshot, input_excerpt, upstream_latency_ms, error,
    violation_count, auto_banned, email_sent, queue_delay_ms, matched_keyword, engine_meta
) VALUES (
    $1, $2, $3, $4, $5, $6, $7,
    $8, $9, $10, $11, $12, $13, $14, $15,
    $16::jsonb, $17::jsonb, $18, $19, $20,
    $21, $22, $23, $24, $25, $26::jsonb
) RETURNING id, created_at`,
		log.RequestID, userID, log.UserEmail, apiKeyID, log.APIKeyName, groupID, log.GroupName,
		log.Endpoint, log.Provider, log.Model, log.Mode, log.Action, log.Flagged, log.HighestCategory, log.HighestScore,
		string(categoryScores), string(thresholdSnapshot), log.InputExcerpt, latency, log.Error,
		log.ViolationCount, log.AutoBanned, log.EmailSent, nullableIntPtr(log.QueueDelayMS), log.MatchedKeyword, engineMeta,
	).Scan(&log.ID, &log.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert content moderation log: %w", err)
	}
	return nil
}

func (r *contentModerationRepository) ListLogs(ctx context.Context, filter service.ContentModerationLogFilter) ([]service.ContentModerationLog, *pagination.PaginationResult, error) {
	where, args := buildContentModerationLogWhere(filter)
	whereSQL := "WHERE " + strings.Join(where, " AND ")

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM content_moderation_logs l "+whereSQL, args...).Scan(&total); err != nil {
		return nil, nil, fmt.Errorf("count content moderation logs: %w", err)
	}

	params := filter.Pagination
	if params.Page <= 0 {
		params.Page = 1
	}
	if params.PageSize <= 0 {
		params.PageSize = 20
	}
	if params.PageSize > 100 {
		params.PageSize = 100
	}
	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, params.Limit(), params.Offset())
	rows, err := r.db.QueryContext(ctx, `
SELECT
    l.id, l.request_id, l.user_id, l.user_email, l.api_key_id, l.api_key_name, l.group_id, l.group_name,
    l.endpoint, l.provider, l.model, l.mode, l.action, l.flagged, l.highest_category, l.highest_score,
    l.category_scores, l.threshold_snapshot, l.input_excerpt, l.upstream_latency_ms, l.error,
    l.violation_count, l.auto_banned, l.email_sent, COALESCE(u.status, ''), l.queue_delay_ms, l.matched_keyword, l.created_at, l.engine_meta
FROM content_moderation_logs l
LEFT JOIN users u ON u.id = l.user_id `+whereSQL+`
ORDER BY l.created_at DESC, l.id DESC
LIMIT $`+fmt.Sprint(len(queryArgs)-1)+` OFFSET $`+fmt.Sprint(len(queryArgs)),
		queryArgs...,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("list content moderation logs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]service.ContentModerationLog, 0)
	for rows.Next() {
		var item service.ContentModerationLog
		var userID, apiKeyID, groupID, latency, queueDelay sql.NullInt64
		var scoresRaw, thresholdsRaw, engineRaw []byte
		if err := rows.Scan(
			&item.ID,
			&item.RequestID,
			&userID,
			&item.UserEmail,
			&apiKeyID,
			&item.APIKeyName,
			&groupID,
			&item.GroupName,
			&item.Endpoint,
			&item.Provider,
			&item.Model,
			&item.Mode,
			&item.Action,
			&item.Flagged,
			&item.HighestCategory,
			&item.HighestScore,
			&scoresRaw,
			&thresholdsRaw,
			&item.InputExcerpt,
			&latency,
			&item.Error,
			&item.ViolationCount,
			&item.AutoBanned,
			&item.EmailSent,
			&item.UserStatus,
			&queueDelay,
			&item.MatchedKeyword,
			&item.CreatedAt,
			&engineRaw,
		); err != nil {
			return nil, nil, fmt.Errorf("scan content moderation log: %w", err)
		}
		if userID.Valid {
			v := userID.Int64
			item.UserID = &v
		}
		if apiKeyID.Valid {
			v := apiKeyID.Int64
			item.APIKeyID = &v
		}
		if groupID.Valid {
			v := groupID.Int64
			item.GroupID = &v
		}
		if latency.Valid {
			v := int(latency.Int64)
			item.UpstreamLatencyMS = &v
		}
		if queueDelay.Valid {
			v := int(queueDelay.Int64)
			item.QueueDelayMS = &v
		}
		item.CategoryScores = map[string]float64{}
		_ = json.Unmarshal(scoresRaw, &item.CategoryScores)
		item.ThresholdSnapshot = map[string]float64{}
		_ = json.Unmarshal(thresholdsRaw, &item.ThresholdSnapshot)
		if len(engineRaw) > 0 {
			if err := json.Unmarshal(engineRaw, &item.EngineMeta); err != nil {
				return nil, nil, fmt.Errorf("decode moderation engine metadata: %w", err)
			}
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate content moderation logs: %w", err)
	}
	return items, paginationResultFromTotal(total, params), nil
}

func (r *contentModerationRepository) CountFlaggedByUserSince(ctx context.Context, userID int64, since time.Time, excludeCyberPolicy bool) (int, error) {
	if userID <= 0 {
		return 0, nil
	}
	// SQL 中的 'cyber_policy' 字面量须与 service.ContentModerationActionCyberPolicy 保持一致。
	// 'cyber_log_only' matches service.ContentModerationModeCyberLogOnly; these
	// events remain evidence but never become penalties after allowlist removal.
	var count int
	err := r.db.QueryRowContext(ctx, `
WITH last_auto_ban AS (
    SELECT MAX(created_at) AS at
    FROM content_moderation_logs
    WHERE user_id = $1 AND auto_banned = TRUE
)
SELECT COUNT(*)
FROM content_moderation_logs
WHERE user_id = $1
  AND flagged = TRUE
  AND action <> 'hash_block'
  AND mode <> 'cyber_log_only'
  AND mode <> 'risk_control_log_only'
  AND mode <> 'distillation_observe'
  AND mode <> 'distillation_enforce'
  AND ($3::bool IS FALSE OR action <> 'cyber_policy')
  AND created_at >= $2
  AND created_at > COALESCE((SELECT at FROM last_auto_ban), '-infinity'::timestamptz)
`, userID, since, excludeCyberPolicy).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count user content moderation flagged logs: %w", err)
	}
	return count, nil
}

func (r *contentModerationRepository) CreateDistillationRequestFeature(ctx context.Context, feature service.DistillationRequestFeature) error {
	_, err := r.db.ExecContext(ctx, `
INSERT INTO distillation_request_features (
    request_id, user_id, api_key_id, group_id, model, prompt_simhash,
    prompt_length, message_count, explicit_signal, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
`, feature.RequestID, feature.UserID, feature.APIKeyID, feature.GroupID, feature.Model,
		feature.PromptSimHash, feature.PromptLength, feature.MessageCount, feature.ExplicitSignal, feature.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert distillation request feature: %w", err)
	}
	return nil
}

func (r *contentModerationRepository) ListDistillationCandidates(ctx context.Context, groupIDs []int64, start, end time.Time, minRequests int, minOutputTokens int64) ([]service.DistillationCandidate, error) {
	if len(groupIDs) == 0 {
		return []service.DistillationCandidate{}, nil
	}
	rows, err := r.db.QueryContext(ctx, `
WITH usage_window AS (
    SELECT
        ul.user_id,
        ul.group_id,
        COUNT(*)::INT AS request_count,
        COALESCE(SUM(ul.input_tokens), 0)::BIGINT AS input_tokens,
        COALESCE(SUM(ul.output_tokens), 0)::BIGINT AS output_tokens,
        COALESCE(SUM(ul.cache_read_tokens), 0)::BIGINT AS cache_read_tokens,
        COUNT(*) FILTER (
            WHERE ul.request_type = 1
               OR (ul.request_type = 0 AND ul.stream = FALSE AND ul.openai_ws_mode = FALSE)
        )::INT AS non_stream_count,
        COUNT(*) FILTER (WHERE NULLIF(BTRIM(ul.session_id), '') IS NULL)::INT AS missing_sessions,
        COUNT(DISTINCT NULLIF(BTRIM(ul.session_id), ''))::INT AS distinct_sessions,
        COUNT(DISTINCT ul.api_key_id)::INT AS distinct_api_keys,
        COUNT(DISTINCT NULLIF(BTRIM(ul.ip_address), ''))::INT AS distinct_ips,
        COUNT(DISTINCT NULLIF(BTRIM(REGEXP_REPLACE(
            ul.user_agent,
            '[[:space:]]+s2a-route/[^[:space:]]+.*$',
            '',
            'i'
        )), ''))::INT AS distinct_user_agents,
        COALESCE(MODE() WITHIN GROUP (
            ORDER BY COALESCE(NULLIF(BTRIM(ul.requested_model), ''), NULLIF(BTRIM(ul.model), ''), '')
        ), '') AS dominant_model,
        MIN(ul.created_at) AS first_request_at,
        MAX(ul.created_at) AS last_request_at
    FROM usage_logs ul
    WHERE ul.group_id = ANY($1)
      AND ul.created_at >= $2
      AND ul.created_at < $3
      AND ul.output_tokens > 0
    GROUP BY ul.user_id, ul.group_id
    HAVING COUNT(*) >= $4 AND COALESCE(SUM(ul.output_tokens), 0) >= $5
), feature_window AS (
    SELECT
        user_id,
        group_id,
        COUNT(*) FILTER (WHERE explicit_signal = TRUE)::INT AS explicit_signals
    FROM distillation_request_features
    WHERE group_id = ANY($1)
      AND created_at >= $2
      AND created_at < $3
    GROUP BY user_id, group_id
)
SELECT
    w.user_id,
    COALESCE(u.email, ''),
    w.group_id,
    COALESCE(g.name, ''),
    w.dominant_model,
    w.request_count,
    w.input_tokens,
    w.output_tokens,
    w.cache_read_tokens,
    w.non_stream_count,
    w.missing_sessions,
    w.distinct_sessions,
    w.distinct_api_keys,
    w.distinct_ips,
    w.distinct_user_agents,
    COALESCE(f.explicit_signals, 0),
    w.first_request_at,
    w.last_request_at
FROM usage_window w
LEFT JOIN feature_window f ON f.user_id = w.user_id AND f.group_id = w.group_id
LEFT JOIN users u ON u.id = w.user_id
LEFT JOIN groups g ON g.id = w.group_id
ORDER BY w.output_tokens DESC, w.request_count DESC
`, pq.Array(groupIDs), start, end, minRequests, minOutputTokens)
	if err != nil {
		return nil, fmt.Errorf("list distillation candidates: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]service.DistillationCandidate, 0)
	for rows.Next() {
		var item service.DistillationCandidate
		if err := rows.Scan(
			&item.UserID, &item.UserEmail, &item.GroupID, &item.GroupName, &item.DominantModel,
			&item.RequestCount, &item.InputTokens, &item.OutputTokens, &item.CacheReadTokens,
			&item.NonStreamCount, &item.MissingSessions, &item.DistinctSessions,
			&item.DistinctAPIKeys, &item.DistinctIPs, &item.DistinctUserAgents,
			&item.ExplicitSignals, &item.FirstRequestAt, &item.LastRequestAt,
		); err != nil {
			return nil, fmt.Errorf("scan distillation candidate: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate distillation candidates: %w", err)
	}
	return items, nil
}

func (r *contentModerationRepository) ListDistillationPromptSimHashes(ctx context.Context, userID, groupID int64, start, end time.Time, limit int) ([]int64, error) {
	if limit <= 0 {
		return []int64{}, nil
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT prompt_simhash
FROM distillation_request_features
WHERE user_id = $1
  AND group_id = $2
  AND created_at >= $3
  AND created_at < $4
  AND prompt_simhash <> 0
ORDER BY created_at DESC
LIMIT $5
`, userID, groupID, start, end, limit)
	if err != nil {
		return nil, fmt.Errorf("list distillation prompt hashes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	hashes := make([]int64, 0)
	for rows.Next() {
		var hash int64
		if err := rows.Scan(&hash); err != nil {
			return nil, fmt.Errorf("scan distillation prompt hash: %w", err)
		}
		hashes = append(hashes, hash)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate distillation prompt hashes: %w", err)
	}
	return hashes, nil
}

func (r *contentModerationRepository) CreateDistillationAssessment(ctx context.Context, assessment service.DistillationAssessment) (bool, error) {
	metrics, err := json.Marshal(assessment.Metrics)
	if err != nil {
		return false, fmt.Errorf("marshal distillation assessment metrics: %w", err)
	}
	var id int64
	err = r.db.QueryRowContext(ctx, `
INSERT INTO distillation_assessments (
    user_id, group_id, window_start, window_end, score, evidence_classes,
    mode, metrics, auto_disabled
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9)
ON CONFLICT (user_id, group_id, window_start) DO NOTHING
RETURNING id
`, assessment.UserID, assessment.GroupID, assessment.WindowStart, assessment.WindowEnd,
		assessment.Score, assessment.EvidenceClasses, assessment.Mode, string(metrics), assessment.AutoDisabled).Scan(&id)
	if err == nil {
		return true, nil
	}
	if err == sql.ErrNoRows {
		return false, nil
	}
	return false, fmt.Errorf("insert distillation assessment: %w", err)
}

func (r *contentModerationRepository) ListDistillationAssessments(ctx context.Context, filter service.DistillationAssessmentFilter) ([]service.DistillationAssessmentRecord, *pagination.PaginationResult, error) {
	params := filter.Pagination
	if params.Page <= 0 {
		params.Page = 1
	}
	if params.PageSize <= 0 {
		params.PageSize = 20
	}
	if params.PageSize > 100 {
		params.PageSize = 100
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM distillation_assessments`).Scan(&total); err != nil {
		return nil, nil, fmt.Errorf("count distillation assessments: %w", err)
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT
    da.id,
    da.user_id,
    COALESCE(u.email, ''),
    COALESCE(u.status, ''),
    da.group_id,
    COALESCE(g.name, ''),
    COALESCE(da.metrics->>'dominant_model', ''),
    da.window_start,
    da.window_end,
    da.score,
    da.evidence_classes,
    da.mode,
    da.metrics,
    da.auto_disabled,
    da.created_at
FROM distillation_assessments da
LEFT JOIN users u ON u.id = da.user_id
LEFT JOIN groups g ON g.id = da.group_id
ORDER BY da.created_at DESC, da.id DESC
LIMIT $1 OFFSET $2
`, params.Limit(), params.Offset())
	if err != nil {
		return nil, nil, fmt.Errorf("list distillation assessments: %w", err)
	}
	defer func() { _ = rows.Close() }()
	items := make([]service.DistillationAssessmentRecord, 0)
	for rows.Next() {
		var item service.DistillationAssessmentRecord
		var metricsRaw []byte
		if err := rows.Scan(
			&item.ID, &item.UserID, &item.UserEmail, &item.UserStatus,
			&item.GroupID, &item.GroupName, &item.DominantModel,
			&item.WindowStart, &item.WindowEnd, &item.Score, &item.EvidenceClasses,
			&item.Mode, &metricsRaw, &item.AutoDisabled, &item.CreatedAt,
		); err != nil {
			return nil, nil, fmt.Errorf("scan distillation assessment: %w", err)
		}
		item.Metrics = map[string]any{}
		if err := json.Unmarshal(metricsRaw, &item.Metrics); err != nil {
			return nil, nil, fmt.Errorf("decode distillation assessment metrics: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate distillation assessments: %w", err)
	}
	return items, paginationResultFromTotal(total, params), nil
}

func (r *contentModerationRepository) CountConsecutiveDistillationAssessments(ctx context.Context, userID, groupID int64, windowEnd time.Time, window time.Duration, threshold, minEvidenceClasses, required int) (int, error) {
	if required <= 0 || window <= 0 {
		return 0, nil
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT window_end
FROM distillation_assessments
WHERE user_id = $1
  AND group_id = $2
  AND window_end <= $3
  AND score >= $4
  AND evidence_classes >= $5
  AND mode = 'enforce'
ORDER BY window_end DESC
LIMIT $6
`, userID, groupID, windowEnd, threshold, minEvidenceClasses, required)
	if err != nil {
		return 0, fmt.Errorf("list consecutive distillation assessments: %w", err)
	}
	defer func() { _ = rows.Close() }()

	count := 0
	expected := windowEnd
	for rows.Next() {
		var actual time.Time
		if err := rows.Scan(&actual); err != nil {
			return 0, fmt.Errorf("scan consecutive distillation assessment: %w", err)
		}
		if !actual.Equal(expected) {
			break
		}
		count++
		expected = expected.Add(-window)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate consecutive distillation assessments: %w", err)
	}
	return count, nil
}

func (r *contentModerationRepository) MarkDistillationAssessmentDisabled(ctx context.Context, userID, groupID int64, windowStart time.Time) error {
	_, err := r.db.ExecContext(ctx, `
UPDATE distillation_assessments
SET auto_disabled = TRUE
WHERE user_id = $1 AND group_id = $2 AND window_start = $3
`, userID, groupID, windowStart)
	if err != nil {
		return fmt.Errorf("mark distillation assessment disabled: %w", err)
	}
	return nil
}

func (r *contentModerationRepository) CleanupDistillationMonitor(ctx context.Context, featuresBefore, assessmentsBefore time.Time) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM distillation_request_features WHERE created_at < $1`, featuresBefore); err != nil {
		return fmt.Errorf("delete expired distillation request features: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, `DELETE FROM distillation_assessments WHERE created_at < $1`, assessmentsBefore); err != nil {
		return fmt.Errorf("delete expired distillation assessments: %w", err)
	}
	return nil
}

func (r *contentModerationRepository) UpdateLogEmailSent(ctx context.Context, id int64, sent bool) error {
	_, err := r.db.ExecContext(ctx, `UPDATE content_moderation_logs SET email_sent = $1 WHERE id = $2`, sent, id)
	if err != nil {
		return fmt.Errorf("update content moderation log email_sent: %w", err)
	}
	return nil
}

func (r *contentModerationRepository) CleanupExpiredLogs(ctx context.Context, hitBefore time.Time, nonHitBefore time.Time) (*service.ContentModerationCleanupResult, error) {
	result := &service.ContentModerationCleanupResult{FinishedAt: time.Now()}
	if r == nil || r.db == nil {
		return result, nil
	}
	hitExec, err := r.db.ExecContext(ctx, `
DELETE FROM content_moderation_logs
WHERE flagged = TRUE AND created_at < $1
`, hitBefore)
	if err != nil {
		return nil, fmt.Errorf("delete expired hit content moderation logs: %w", err)
	}
	result.DeletedHit, _ = hitExec.RowsAffected()

	nonHitExec, err := r.db.ExecContext(ctx, `
DELETE FROM content_moderation_logs
WHERE flagged = FALSE AND created_at < $1
`, nonHitBefore)
	if err != nil {
		return nil, fmt.Errorf("delete expired non-hit content moderation logs: %w", err)
	}
	result.DeletedNonHit, _ = nonHitExec.RowsAffected()

	result.FinishedAt = time.Now()
	return result, nil
}

func nullableIntPtr(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func buildContentModerationLogWhere(filter service.ContentModerationLogFilter) ([]string, []any) {
	where := []string{"l.id IS NOT NULL"}
	args := make([]any, 0)
	add := func(expr string, value any) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(expr, len(args)))
	}
	switch strings.ToLower(strings.TrimSpace(filter.Result)) {
	case "hit", "flagged":
		where = append(where, "l.flagged = TRUE")
	case "blocked", "block":
		where = append(where, "l.action IN ('block', 'keyword_block', 'hash_block')")
	case "pass", "allow":
		where = append(where, "l.flagged = FALSE AND l.error = ''")
	case "error":
		where = append(where, "l.error <> ''")
	}
	if filter.GroupID != nil {
		add("l.group_id = $%d", *filter.GroupID)
	}
	if endpoint := strings.TrimSpace(filter.Endpoint); endpoint != "" {
		add("l.endpoint = $%d", endpoint)
	}
	if provider := strings.TrimSpace(filter.Provider); provider != "" {
		add("l.provider = $%d", provider)
	}
	if search := strings.TrimSpace(filter.Search); search != "" {
		like := "%" + search + "%"
		args = append(args, like, like, like, like, like)
		idx := len(args) - 4
		where = append(where, fmt.Sprintf("(l.request_id ILIKE $%d OR l.user_email ILIKE $%d OR l.api_key_name ILIKE $%d OR l.model ILIKE $%d OR l.input_excerpt ILIKE $%d)", idx, idx+1, idx+2, idx+3, idx+4))
	}
	if filter.From != nil && !filter.From.IsZero() {
		add("l.created_at >= $%d", *filter.From)
	}
	if filter.To != nil && !filter.To.IsZero() {
		add("l.created_at <= $%d", *filter.To)
	}
	return where, args
}
