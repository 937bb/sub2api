package repository

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type balanceEntryRepository struct {
	client *dbent.Client
}

// NewBalanceEntryRepository 创建余额明细仓储
func NewBalanceEntryRepository(client *dbent.Client) service.BalanceEntryRepository {
	return &balanceEntryRepository{client: client}
}

func (r *balanceEntryRepository) Create(ctx context.Context, entry *service.BalanceEntry) error {
	client := clientFromContext(ctx, r.client)
	var expiresAt sql.NullTime
	if entry.ExpiresAt != nil {
		expiresAt = sql.NullTime{Time: *entry.ExpiresAt, Valid: true}
	}
	rows, err := client.QueryContext(ctx, `
INSERT INTO balance_entries (user_id, amount, remaining, balance_type, source, note, expires_at, expired, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
RETURNING id, created_at`,
		entry.UserID, entry.Amount, entry.Remaining, entry.BalanceType,
		entry.Source, entry.Note, expiresAt, entry.Expired,
	)
	if err != nil {
		return fmt.Errorf("insert balance_entry: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if rows.Next() {
		if err := rows.Scan(&entry.ID, &entry.CreatedAt); err != nil {
			return fmt.Errorf("scan balance_entry: %w", err)
		}
	}
	return rows.Close()
}

func (r *balanceEntryRepository) ListByUser(ctx context.Context, userID int64, offset, limit int) ([]*service.BalanceEntry, int64, error) {
	client := clientFromContext(ctx, r.client)

	// 总数
	var total int64
	countRows, err := client.QueryContext(ctx,
		`SELECT COUNT(*) FROM balance_entries WHERE user_id = $1`, userID)
	if err != nil {
		return nil, 0, fmt.Errorf("count balance_entries: %w", err)
	}
	defer func() { _ = countRows.Close() }()
	if countRows.Next() {
		if err := countRows.Scan(&total); err != nil {
			return nil, 0, err
		}
	}
	if err := countRows.Close(); err != nil {
		return nil, 0, err
	}

	// 明细
	rows, err := client.QueryContext(ctx, `
SELECT id, user_id, amount, remaining, balance_type, source, note, expires_at, expired, created_at
FROM balance_entries
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3`, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list balance_entries: %w", err)
	}
	defer func() { _ = rows.Close() }()

	entries := make([]*service.BalanceEntry, 0)
	for rows.Next() {
		e, err := scanBalanceEntry(rows)
		if err != nil {
			return nil, 0, err
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return entries, total, nil
}

func (r *balanceEntryRepository) GetAvailableEntries(ctx context.Context, userID int64) ([]*service.BalanceEntry, error) {
	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx, `
SELECT id, user_id, amount, remaining, balance_type, source, note, expires_at, expired, created_at
FROM balance_entries
WHERE user_id = $1 AND remaining > 0 AND expired = FALSE
  AND (expires_at IS NULL OR expires_at > NOW())
ORDER BY expires_at ASC NULLS LAST, created_at ASC`, userID)
	if err != nil {
		return nil, fmt.Errorf("get available balance_entries: %w", err)
	}
	defer func() { _ = rows.Close() }()

	entries := make([]*service.BalanceEntry, 0)
	for rows.Next() {
		e, err := scanBalanceEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func (r *balanceEntryRepository) DeductFromEntries(ctx context.Context, entries []*service.BalanceEntry, amount float64) (float64, error) {
	client := clientFromContext(ctx, r.client)
	var totalDeducted float64

	for _, e := range entries {
		if amount <= 0 {
			break
		}
		if e.Remaining <= 0 {
			continue
		}
		deduct := math.Min(e.Remaining, amount)
		newRemaining := e.Remaining - deduct

		_, err := client.ExecContext(ctx,
			`UPDATE balance_entries SET remaining = $1 WHERE id = $2`,
			newRemaining, e.ID)
		if err != nil {
			return totalDeducted, fmt.Errorf("deduct balance_entry %d: %w", e.ID, err)
		}

		e.Remaining = newRemaining
		totalDeducted += deduct
		amount -= deduct
	}

	return totalDeducted, nil
}

func (r *balanceEntryRepository) ExpireEntries(ctx context.Context, before time.Time) ([]*service.BalanceEntry, error) {
	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx, `
UPDATE balance_entries
SET expired = TRUE, remaining = 0
WHERE balance_type = 'expirable' AND expired = FALSE AND remaining > 0 AND expires_at <= $1
RETURNING id, user_id, amount, remaining, balance_type, source, note, expires_at, expired, created_at`, before)
	if err != nil {
		return nil, fmt.Errorf("expire balance_entries: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var entries []*service.BalanceEntry
	for rows.Next() {
		e, err := scanBalanceEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func (r *balanceEntryRepository) SumRemainingByUser(ctx context.Context, userID int64) (float64, error) {
	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx, `
SELECT COALESCE(SUM(remaining), 0)::double precision
FROM balance_entries
WHERE user_id = $1 AND expired = FALSE AND (expires_at IS NULL OR expires_at > NOW())`, userID)
	if err != nil {
		return 0, fmt.Errorf("sum remaining: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var total float64
	if rows.Next() {
		if err := rows.Scan(&total); err != nil {
			return 0, err
		}
	}
	return total, rows.Close()
}

func (r *balanceEntryRepository) GetUserBalanceSummary(ctx context.Context, userID int64, soonDays int) (*service.BalanceSummary, error) {
	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx, `
SELECT
    COALESCE(SUM(remaining), 0)::double precision AS total,
    COALESCE(SUM(CASE WHEN balance_type = 'permanent' THEN remaining ELSE 0 END), 0)::double precision AS permanent,
    COALESCE(SUM(CASE WHEN balance_type = 'expirable' THEN remaining ELSE 0 END), 0)::double precision AS expirable,
    COALESCE(SUM(CASE WHEN balance_type = 'expirable' AND expires_at <= NOW() + make_interval(days => $2) THEN remaining ELSE 0 END), 0)::double precision AS expiring_soon
FROM balance_entries
WHERE user_id = $1 AND expired = FALSE AND remaining > 0
  AND (expires_at IS NULL OR expires_at > NOW())`, userID, soonDays)
	if err != nil {
		return nil, fmt.Errorf("get balance summary: %w", err)
	}
	defer func() { _ = rows.Close() }()

	s := &service.BalanceSummary{}
	if rows.Next() {
		if err := rows.Scan(&s.TotalBalance, &s.PermanentBalance, &s.ExpirableBalance, &s.ExpiringSoon); err != nil {
			return nil, err
		}
	}
	return s, rows.Close()
}

// scanBalanceEntry 从 sql.Rows 扫描一条 BalanceEntry
func scanBalanceEntry(rows *sql.Rows) (*service.BalanceEntry, error) {
	e := &service.BalanceEntry{}
	var expiresAt sql.NullTime
	if err := rows.Scan(
		&e.ID, &e.UserID, &e.Amount, &e.Remaining, &e.BalanceType,
		&e.Source, &e.Note, &expiresAt, &e.Expired, &e.CreatedAt,
	); err != nil {
		return nil, fmt.Errorf("scan balance_entry: %w", err)
	}
	if expiresAt.Valid {
		e.ExpiresAt = &expiresAt.Time
	}
	return e, nil
}
