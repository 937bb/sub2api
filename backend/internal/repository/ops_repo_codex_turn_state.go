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
  source_model, source_transport, issued_at, first_seen_at, last_seen_at, expires_at
) VALUES ($1,$2,$3,$4,NULLIF($5,''),NULLIF($6,''),$7,$8,$9,$10,$11)
ON CONFLICT (state_hash) DO UPDATE SET
  state_value = EXCLUDED.state_value,
  value_length = EXCLUDED.value_length,
  source_account_id = EXCLUDED.source_account_id,
  source_session_hash = EXCLUDED.source_session_hash,
  source_model = EXCLUDED.source_model,
  source_transport = EXCLUDED.source_transport,
  issued_at = EXCLUDED.issued_at,
  last_seen_at = EXCLUDED.last_seen_at,
  expires_at = EXCLUDED.expires_at
WHERE EXCLUDED.last_seen_at >= codex_turn_states.last_seen_at`

func (r *opsRepository) UpsertOpenAICodexTurnState(ctx context.Context, record *service.OpenAICodexTurnStateRecord) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("nil ops repository")
	}
	if record == nil || strings.TrimSpace(record.StateValue) == "" || strings.TrimSpace(record.StateHash) == "" {
		return fmt.Errorf("invalid Codex turn-state record")
	}
	_, err := r.db.ExecContext(ctx, upsertOpenAICodexTurnStateSQL,
		record.StateValue,
		record.StateHash,
		record.ValueLength,
		record.SourceAccountID,
		record.SourceSessionHash,
		record.SourceModel,
		record.SourceTransport,
		nullableOpenAICodexTurnStateTime(record.IssuedAt),
		record.FirstSeenAt,
		record.LastSeenAt,
		record.ExpiresAt,
	)
	return err
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

func (r *opsRepository) LoadActiveOpenAICodexTurnStates(ctx context.Context, now time.Time) ([]*service.OpenAICodexTurnStateRecord, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil ops repository")
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT id, state_value, state_hash, value_length, source_account_id,
       COALESCE(source_session_hash, ''), COALESCE(source_model, ''), source_transport,
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
       COALESCE(a.name, ''), COALESCE(c.source_session_hash, ''), COALESCE(c.source_model, ''), c.source_transport,
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
	summary := &service.OpenAICodexTurnStateSummary{ReuseTTLSeconds: int64(time.Hour / time.Second)}
	if err := r.db.QueryRowContext(ctx, `
SELECT COUNT(*) FILTER (WHERE expires_at > $1),
       COUNT(*) FILTER (WHERE expires_at <= $1)
FROM codex_turn_states`, now).Scan(&summary.ActiveCount, &summary.ExpiredCount); err != nil {
		return nil, err
	}

	record := &service.OpenAICodexTurnStateRecord{}
	err := r.db.QueryRowContext(ctx, `
SELECT c.id, c.state_value, c.state_hash, c.value_length, c.source_account_id,
       COALESCE(a.name, ''), COALESCE(c.source_session_hash, ''), COALESCE(c.source_model, ''), c.source_transport,
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
