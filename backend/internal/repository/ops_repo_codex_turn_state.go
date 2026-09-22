package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

const upsertOpenAICodexTurnStateSQL = `
INSERT INTO codex_turn_states (
  state_value, state_hash, value_length, source_account_id, source_session_hash,
	  source_session_id, source_proxy_id, source_proxy_url, source_exit_ip, route_ipv6, route_cookie,
	  source_model, source_transport, issued_at, first_seen_at, last_seen_at, expires_at
	) VALUES ($1,$2,$3,$4,NULLIF($5,''),NULLIF($6,''),$7,NULLIF($8,''),NULLIF($9,''),NULLIF($10,''),NULLIF($11,''),NULLIF($12,''),$13,$14,$15,$16,$17)
ON CONFLICT (state_hash) DO UPDATE SET
  state_value = EXCLUDED.state_value,
  value_length = EXCLUDED.value_length,
  source_account_id = EXCLUDED.source_account_id,
  source_session_hash = EXCLUDED.source_session_hash,
  source_session_id = EXCLUDED.source_session_id,
  source_proxy_id = EXCLUDED.source_proxy_id,
  source_proxy_url = EXCLUDED.source_proxy_url,
  source_exit_ip = EXCLUDED.source_exit_ip,
	  route_ipv6 = EXCLUDED.route_ipv6,
	  route_cookie = EXCLUDED.route_cookie,
  source_model = EXCLUDED.source_model,
  source_transport = EXCLUDED.source_transport,
  issued_at = EXCLUDED.issued_at,
  last_seen_at = EXCLUDED.last_seen_at,
  expires_at = EXCLUDED.expires_at
WHERE EXCLUDED.last_seen_at >= codex_turn_states.last_seen_at
  AND EXCLUDED.source_account_id IS NOT DISTINCT FROM codex_turn_states.source_account_id
  AND EXCLUDED.source_model IS NOT DISTINCT FROM codex_turn_states.source_model`

func (r *opsRepository) UpsertOpenAICodexTurnState(ctx context.Context, record *service.OpenAICodexTurnStateRecord) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("nil ops repository")
	}
	if record == nil || strings.TrimSpace(record.StateValue) == "" || strings.TrimSpace(record.StateHash) == "" {
		return fmt.Errorf("invalid Codex turn-state record")
	}
	result, err := r.db.ExecContext(ctx, upsertOpenAICodexTurnStateSQL,
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
		record.RouteCookie,
		record.SourceModel,
		record.SourceTransport,
		nullableOpenAICodexTurnStateTime(record.IssuedAt),
		record.FirstSeenAt,
		record.LastSeenAt,
		record.ExpiresAt,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		// A delayed observation can lose to a newer one from the same bucket.
		// Treat that as durable only after confirming the existing row's scope
		// and lifetime. A hash collision across buckets must remain an error.
		var alreadyStored bool
		if err := r.db.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1 FROM codex_turn_states
  WHERE state_hash = $1 AND source_account_id IS NOT DISTINCT FROM $2::bigint
    AND source_model IS NOT DISTINCT FROM NULLIF($3::text, '')
    AND state_value = $4 AND value_length = $5 AND last_seen_at >= $6
    AND issued_at IS NOT DISTINCT FROM $7::timestamptz AND expires_at >= $8
)`, record.StateHash, record.SourceAccountID, record.SourceModel, record.StateValue,
			record.ValueLength, record.LastSeenAt, nullableOpenAICodexTurnStateTime(record.IssuedAt), record.ExpiresAt).Scan(&alreadyStored); err != nil {
			return fmt.Errorf("verify persisted Codex turn-state: %w", err)
		}
		if !alreadyStored {
			return fmt.Errorf("codex turn-state was not persisted because a conflicting record exists")
		}
	}
	return nil
}

func (r *opsRepository) BatchUpsertOpenAICodexTurnStates(ctx context.Context, records []*service.OpenAICodexTurnStateRecord) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("nil ops repository")
	}
	if len(records) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, record := range records {
		if record == nil || strings.TrimSpace(record.StateValue) == "" || strings.TrimSpace(record.StateHash) == "" {
			return fmt.Errorf("invalid Codex turn-state record")
		}
		if _, err := tx.ExecContext(ctx, upsertOpenAICodexTurnStateSQL,
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
			record.RouteCookie,
			record.SourceModel,
			record.SourceTransport,
			nullableOpenAICodexTurnStateTime(record.IssuedAt),
			record.FirstSeenAt,
			record.LastSeenAt,
			record.ExpiresAt,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *opsRepository) DeleteOpenAICodexTurnStates(ctx context.Context, ids []int64) ([]string, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	if len(ids) == 0 {
		return []string{}, nil
	}
	rows, err := r.db.QueryContext(ctx, `
DELETE FROM codex_turn_states
WHERE id = ANY($1)
RETURNING state_hash`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	hashes := make([]string, 0, len(ids))
	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			return nil, err
		}
		hashes = append(hashes, hash)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return hashes, nil
}

func nullableOpenAICodexTurnStateTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}

func (r *opsRepository) DeleteOpenAICodexTurnStatesExpiredBefore(ctx context.Context, cutoff time.Time) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("nil ops repository")
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM codex_turn_states WHERE expires_at <= $1`, cutoff)
	return err
}

func (r *opsRepository) ExpireOpenAICodexTurnStates(ctx context.Context, accountID int64, model string, stateHashes []string, expiredAt time.Time) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("nil ops repository")
	}
	model = strings.ToLower(strings.TrimSpace(model))
	if accountID <= 0 || model == "" || len(stateHashes) == 0 {
		return nil
	}
	_, err := r.db.ExecContext(ctx, `
UPDATE codex_turn_states
SET expires_at = LEAST(expires_at, $4)
WHERE source_account_id = $1 AND source_model = $2 AND state_hash = ANY($3)`,
		accountID, model, pq.Array(stateHashes), expiredAt)
	return err
}

func (r *opsRepository) LoadActiveOpenAICodexTurnStates(ctx context.Context, now time.Time) ([]*service.OpenAICodexTurnStateRecord, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT id, state_value, state_hash, value_length, source_account_id,
       COALESCE(source_session_hash, ''), COALESCE(source_session_id, ''), source_proxy_id,
	       COALESCE(source_proxy_url, ''), COALESCE(source_exit_ip, ''), COALESCE(route_ipv6, ''), COALESCE(route_cookie, ''), COALESCE(source_model, ''), source_transport,
       COALESCE(issued_at, 'epoch'::timestamptz),
       first_seen_at, last_seen_at, expires_at
FROM codex_turn_states
WHERE expires_at > $1
ORDER BY value_length DESC, last_seen_at DESC, id DESC`, now)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			// Query result cleanup errors do not change an already materialized snapshot.
		}
	}()

	records := make([]*service.OpenAICodexTurnStateRecord, 0)
	for rows.Next() {
		record := &service.OpenAICodexTurnStateRecord{}
		if err := rows.Scan(
			&record.ID,
			&record.StateValue,
			&record.StateHash,
			&record.ValueLength,
			&record.SourceAccountID,
			&record.SourceSessionHash,
			&record.SourceSessionID,
			&record.SourceProxyID,
			&record.SourceProxyURL,
			&record.SourceExitIP,
			&record.RouteIPv6,
			&record.RouteCookie,
			&record.SourceModel,
			&record.SourceTransport,
			&record.IssuedAt,
			&record.FirstSeenAt,
			&record.LastSeenAt,
			&record.ExpiresAt,
		); err != nil {
			return nil, err
		}
		record.Active = true
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func (r *opsRepository) LoadPreferredOpenAICodexTurnState(ctx context.Context, accountID int64, model string, targetLengths []int, now time.Time) (*service.OpenAICodexTurnStateRecord, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	model = strings.ToLower(strings.TrimSpace(model))
	if accountID <= 0 || model == "" || len(targetLengths) == 0 {
		return nil, fmt.Errorf("invalid Codex turn-state bucket")
	}
	record := &service.OpenAICodexTurnStateRecord{}
	err := r.db.QueryRowContext(ctx, `
SELECT id, state_value, state_hash, value_length, source_account_id,
       COALESCE(source_session_hash, ''), COALESCE(source_session_id, ''), source_proxy_id,
	       COALESCE(source_proxy_url, ''), COALESCE(source_exit_ip, ''), COALESCE(route_ipv6, ''), COALESCE(route_cookie, ''), source_model, source_transport,
       issued_at, first_seen_at, last_seen_at, expires_at
FROM codex_turn_states
WHERE source_account_id = $1 AND source_model = $2
  AND value_length = ANY($3::integer[]) AND expires_at > $4
  AND issued_at <= $4 AND issued_at + INTERVAL '4 minutes' > $4
  AND source_transport = 'scanner' AND source_session_id IS NOT NULL
ORDER BY array_position($3::integer[], value_length),
         expires_at DESC, last_seen_at DESC, state_hash DESC
LIMIT 1`, accountID, model, pq.Array(targetLengths), now).Scan(
		&record.ID, &record.StateValue, &record.StateHash, &record.ValueLength, &record.SourceAccountID,
		&record.SourceSessionHash, &record.SourceSessionID, &record.SourceProxyID,
		&record.SourceProxyURL, &record.SourceExitIP, &record.RouteIPv6, &record.RouteCookie, &record.SourceModel, &record.SourceTransport,
		&record.IssuedAt, &record.FirstSeenAt, &record.LastSeenAt, &record.ExpiresAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	record.Active = true
	return record, nil
}

func (r *opsRepository) ListOpenAICodexTurnStates(ctx context.Context, filter *service.OpenAICodexTurnStateFilter) (*service.OpenAICodexTurnStateList, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	filter = normalizeOpenAICodexTurnStateFilter(filter)
	where, args := buildOpenAICodexTurnStateWhere(filter)

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM codex_turn_states c "+where, args...).Scan(&total); err != nil {
		return nil, err
	}
	args = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)
	query := fmt.Sprintf(`
SELECT c.id, c.state_value, c.state_hash, c.value_length, c.source_account_id,
       COALESCE(a.name, ''), COALESCE(c.source_session_hash, ''), COALESCE(c.source_session_id, ''), c.source_proxy_id,
	       COALESCE(c.source_proxy_url, ''), COALESCE(c.source_exit_ip, ''), COALESCE(c.route_ipv6, ''), COALESCE(c.route_cookie, ''), COALESCE(c.source_model, ''), c.source_transport,
       COALESCE(c.issued_at, 'epoch'::timestamptz),
       c.first_seen_at, c.last_seen_at, c.expires_at, (c.expires_at > NOW())
FROM codex_turn_states c
LEFT JOIN accounts a ON a.id = c.source_account_id
%s
ORDER BY c.value_length DESC, c.last_seen_at DESC, c.id DESC
LIMIT $%d OFFSET $%d`, where, len(args)-1, len(args))
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]*service.OpenAICodexTurnStateRecord, 0, filter.PageSize)
	for rows.Next() {
		record := &service.OpenAICodexTurnStateRecord{}
		if err := rows.Scan(
			&record.ID,
			&record.StateValue,
			&record.StateHash,
			&record.ValueLength,
			&record.SourceAccountID,
			&record.SourceAccountName,
			&record.SourceSessionHash,
			&record.SourceSessionID,
			&record.SourceProxyID,
			&record.SourceProxyURL,
			&record.SourceExitIP,
			&record.RouteIPv6,
			&record.RouteCookie,
			&record.SourceModel,
			&record.SourceTransport,
			&record.IssuedAt,
			&record.FirstSeenAt,
			&record.LastSeenAt,
			&record.ExpiresAt,
			&record.Active,
		); err != nil {
			return nil, err
		}
		items = append(items, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &service.OpenAICodexTurnStateList{Items: items, Total: total, Page: filter.Page, PageSize: filter.PageSize}, nil
}

func (r *opsRepository) GetOpenAICodexTurnStateSummary(ctx context.Context, now time.Time) (*service.OpenAICodexTurnStateSummary, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	summary := &service.OpenAICodexTurnStateSummary{ReuseTTLSeconds: int64((240 * time.Second) / time.Second)}
	if err := r.db.QueryRowContext(ctx, `
SELECT COUNT(*) FILTER (WHERE expires_at > $1),
       COUNT(*) FILTER (WHERE expires_at <= $1)
FROM codex_turn_states`, now).Scan(&summary.ActiveCount, &summary.ExpiredCount); err != nil {
		return nil, err
	}

	record := &service.OpenAICodexTurnStateRecord{}
	err := r.db.QueryRowContext(ctx, `
SELECT c.id, c.state_value, c.state_hash, c.value_length, c.source_account_id,
       COALESCE(a.name, ''), COALESCE(c.source_session_hash, ''), COALESCE(c.source_session_id, ''), c.source_proxy_id,
	       COALESCE(c.source_proxy_url, ''), COALESCE(c.source_exit_ip, ''), COALESCE(c.route_ipv6, ''), COALESCE(c.route_cookie, ''), COALESCE(c.source_model, ''), c.source_transport,
       COALESCE(c.issued_at, 'epoch'::timestamptz),
       c.first_seen_at, c.last_seen_at, c.expires_at
FROM codex_turn_states c
LEFT JOIN accounts a ON a.id = c.source_account_id
WHERE c.expires_at > $1
ORDER BY c.value_length DESC, c.last_seen_at DESC, c.id DESC
LIMIT 1`, now).Scan(
		&record.ID,
		&record.StateValue,
		&record.StateHash,
		&record.ValueLength,
		&record.SourceAccountID,
		&record.SourceAccountName,
		&record.SourceSessionHash,
		&record.SourceSessionID,
		&record.SourceProxyID,
		&record.SourceProxyURL,
		&record.SourceExitIP,
		&record.RouteIPv6,
		&record.RouteCookie,
		&record.SourceModel,
		&record.SourceTransport,
		&record.IssuedAt,
		&record.FirstSeenAt,
		&record.LastSeenAt,
		&record.ExpiresAt,
	)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if err == nil {
		record.Active = true
		summary.LongestActive = record
	}
	return summary, nil
}

func normalizeOpenAICodexTurnStateFilter(filter *service.OpenAICodexTurnStateFilter) *service.OpenAICodexTurnStateFilter {
	if filter == nil {
		filter = &service.OpenAICodexTurnStateFilter{}
	}
	copyFilter := *filter
	copyFilter.Status = strings.ToLower(strings.TrimSpace(copyFilter.Status))
	if copyFilter.Status != "active" && copyFilter.Status != "expired" && copyFilter.Status != "all" {
		copyFilter.Status = "active"
	}
	if copyFilter.Page < 1 {
		copyFilter.Page = 1
	}
	if copyFilter.PageSize < 1 {
		copyFilter.PageSize = 20
	}
	if copyFilter.PageSize > 200 {
		copyFilter.PageSize = 200
	}
	return &copyFilter
}

func buildOpenAICodexTurnStateWhere(filter *service.OpenAICodexTurnStateFilter) (string, []any) {
	clauses := make([]string, 0, 2)
	args := make([]any, 0, 2)
	switch filter.Status {
	case "active":
		clauses = append(clauses, "c.expires_at > NOW()")
	case "expired":
		clauses = append(clauses, "c.expires_at <= NOW()")
	}
	if filter.AccountID != nil {
		args = append(args, *filter.AccountID)
		clauses = append(clauses, fmt.Sprintf("c.source_account_id = $%d", len(args)))
	}
	if len(clauses) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}
