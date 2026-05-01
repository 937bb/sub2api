package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type checkinRepository struct {
	client *dbent.Client
}

// NewCheckinRepository 创建签到仓储
func NewCheckinRepository(client *dbent.Client) service.CheckinRepository {
	return &checkinRepository{client: client}
}

func (r *checkinRepository) Create(ctx context.Context, record *service.CheckinRecord) error {
	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx, `
INSERT INTO checkin_records (user_id, checkin_date, streak, base_amount, milestone_amount, total_amount, balance_type, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
RETURNING id, created_at`,
		record.UserID, record.CheckinDate, record.Streak,
		record.BaseAmount, record.MilestoneAmount, record.TotalAmount, record.BalanceType,
	)
	if err != nil {
		return fmt.Errorf("insert checkin_record: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if rows.Next() {
		if err := rows.Scan(&record.ID, &record.CreatedAt); err != nil {
			return fmt.Errorf("scan checkin_record returning: %w", err)
		}
	}
	return rows.Err()
}

func (r *checkinRepository) GetByUserAndDate(ctx context.Context, userID int64, date time.Time) (*service.CheckinRecord, error) {
	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx, `
SELECT id, user_id, checkin_date, streak, base_amount, milestone_amount, total_amount, balance_type, created_at
FROM checkin_records
WHERE user_id = $1 AND checkin_date = $2`,
		userID, date,
	)
	if err != nil {
		return nil, fmt.Errorf("query checkin by date: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return nil, nil
	}
	return scanCheckinRecord(rows)
}

func (r *checkinRepository) GetLatestByUser(ctx context.Context, userID int64) (*service.CheckinRecord, error) {
	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx, `
SELECT id, user_id, checkin_date, streak, base_amount, milestone_amount, total_amount, balance_type, created_at
FROM checkin_records
WHERE user_id = $1
ORDER BY checkin_date DESC
LIMIT 1`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("query latest checkin: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return nil, nil
	}
	return scanCheckinRecord(rows)
}

func (r *checkinRepository) ListByUser(ctx context.Context, userID int64, offset, limit int) ([]*service.CheckinRecord, int64, error) {
	client := clientFromContext(ctx, r.client)

	// count
	var total int64
	countRows, err := client.QueryContext(ctx, `SELECT COUNT(*) FROM checkin_records WHERE user_id = $1`, userID)
	if err != nil {
		return nil, 0, fmt.Errorf("count checkin_records: %w", err)
	}
	defer func() { _ = countRows.Close() }()
	if countRows.Next() {
		if err := countRows.Scan(&total); err != nil {
			return nil, 0, fmt.Errorf("scan count: %w", err)
		}
	}

	rows, err := client.QueryContext(ctx, `
SELECT id, user_id, checkin_date, streak, base_amount, milestone_amount, total_amount, balance_type, created_at
FROM checkin_records
WHERE user_id = $1
ORDER BY checkin_date DESC
LIMIT $2 OFFSET $3`,
		userID, limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list checkin_records: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var records []*service.CheckinRecord
	for rows.Next() {
		rec, err := scanCheckinRecord(rows)
		if err != nil {
			return nil, 0, err
		}
		records = append(records, rec)
	}
	return records, total, rows.Err()
}

func (r *checkinRepository) CountByUser(ctx context.Context, userID int64) (int64, error) {
	client := clientFromContext(ctx, r.client)
	var count int64
	countRows, err := client.QueryContext(ctx, `SELECT COUNT(*) FROM checkin_records WHERE user_id = $1`, userID)
	if err != nil {
		return 0, fmt.Errorf("count checkin_records: %w", err)
	}
	defer func() { _ = countRows.Close() }()
	if countRows.Next() {
		if err := countRows.Scan(&count); err != nil {
			return 0, fmt.Errorf("scan count: %w", err)
		}
	}
	return count, nil
}

func (r *checkinRepository) GetMonthlyRecords(ctx context.Context, userID int64, year int, month int) ([]*service.CheckinRecord, error) {
	client := clientFromContext(ctx, r.client)
	startDate := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	endDate := startDate.AddDate(0, 1, 0)

	rows, err := client.QueryContext(ctx, `
SELECT id, user_id, checkin_date, streak, base_amount, milestone_amount, total_amount, balance_type, created_at
FROM checkin_records
WHERE user_id = $1 AND checkin_date >= $2 AND checkin_date < $3
ORDER BY checkin_date ASC`,
		userID, startDate, endDate,
	)
	if err != nil {
		return nil, fmt.Errorf("query monthly checkins: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var records []*service.CheckinRecord
	for rows.Next() {
		rec, err := scanCheckinRecord(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}

// scanCheckinRecord scans a single row into a CheckinRecord
func scanCheckinRecord(rows *sql.Rows) (*service.CheckinRecord, error) {
	r := &service.CheckinRecord{}
	if err := rows.Scan(
		&r.ID, &r.UserID, &r.CheckinDate, &r.Streak,
		&r.BaseAmount, &r.MilestoneAmount, &r.TotalAmount,
		&r.BalanceType, &r.CreatedAt,
	); err != nil {
		return nil, fmt.Errorf("scan checkin_record: %w", err)
	}
	return r, nil
}
