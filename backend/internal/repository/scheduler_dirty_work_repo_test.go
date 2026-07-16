package repository

import (
	"context"
	"database/sql/driver"
	"errors"
	"regexp"
	"sync"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestSchedulerSourcePromotionRejectsMissingOwnership(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	promoted, err := NewSchedulerDirtyWorkRepository(db).Promote(context.Background(), nil, 10)
	require.Zero(t, promoted)
	require.ErrorContains(t, err, "active ownership")
}

func TestSchedulerDirtyWorkListIsBoundedAndOrdered(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	now := time.Now().UTC()
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT kind, entity_id, generation, rebuild_buckets, updated_at
		FROM scheduler_dirty_work
		ORDER BY updated_at ASC, kind ASC, entity_id ASC
		LIMIT $1
	`)).WithArgs(maxDirtyWorkBatchSize).WillReturnRows(
		sqlmock.NewRows([]string{"kind", "entity_id", "generation", "rebuild_buckets", "updated_at"}).
			AddRow(int16(2), int64(9), int64(4), true, now),
	)

	items, err := NewSchedulerDirtyWorkRepository(db).List(context.Background(), maxDirtyWorkBatchSize+1)
	require.NoError(t, err)
	require.Equal(t, []service.SchedulerDirtyWork{{Kind: 2, EntityID: 9, Generation: 4, RebuildBuckets: true, UpdatedAt: now}}, items)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSchedulerDirtyWorkAcknowledgeIsGenerationConditional(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewSchedulerDirtyWorkRepository(db)
	item := service.SchedulerDirtyWork{Kind: 1, EntityID: 42, Generation: 7}
	query := regexp.QuoteMeta(`
		DELETE FROM scheduler_dirty_work
		WHERE kind = $1 AND entity_id = $2 AND generation = $3
	`)

	// Zero rows is the expected acknowledgement when a producer concurrently
	// advanced generation; the newer dirty state remains in PostgreSQL.
	mock.ExpectExec(query).WithArgs(item.Kind, item.EntityID, item.Generation).
		WillReturnResult(sqlmock.NewResult(0, 0))
	acknowledged, err := repo.Acknowledge(context.Background(), item)
	require.NoError(t, err)
	require.False(t, acknowledged)

	mock.ExpectExec(query).WithArgs(item.Kind, item.EntityID, item.Generation).
		WillReturnResult(sqlmock.NewResult(0, 1))
	acknowledged, err = repo.Acknowledge(context.Background(), item)
	require.NoError(t, err)
	require.True(t, acknowledged)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSchedulerDirtyWorkPendingStatsUsesWholePopulation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	now := time.Now().UTC()

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT COUNT(*), MIN(updated_at)
		FROM scheduler_dirty_work
	`)).WillReturnRows(sqlmock.NewRows([]string{"count", "min"}).AddRow(int64(3), now))
	stats, err := NewSchedulerDirtyWorkRepository(db).PendingStats(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(3), stats.Count)
	require.Equal(t, now, *stats.OldestUpdatedAt)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSchedulerOwnershipUsesDedicatedSession(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT pg_try_advisory_lock($1, $2)`)).
		WithArgs(lockArg(schedulerAdvisoryLockNamespace), lockArg(schedulerAdvisoryLockID)).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT pg_advisory_unlock($1, $2)`)).
		WithArgs(lockArg(schedulerAdvisoryLockNamespace), lockArg(schedulerAdvisoryLockID)).
		WillReturnRows(sqlmock.NewRows([]string{"pg_advisory_unlock"}).AddRow(true))

	repo := &schedulerOwnershipRepository{db: db, checkInterval: time.Hour}
	owner, acquired, err := repo.TryAcquire(context.Background())
	require.NoError(t, err)
	require.True(t, acquired)
	require.NotNil(t, owner)
	require.NoError(t, owner.Close())
	select {
	case <-owner.Lost():
	default:
		t.Fatal("ownership loss was not exposed on close")
	}
	require.ErrorIs(t, owner.Context().Err(), context.Canceled)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSchedulerOwnershipConcurrentCloseStoresOneResult(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT pg_try_advisory_lock($1, $2)`)).
		WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(true))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT pg_advisory_unlock($1, $2)`)).
		WillReturnRows(sqlmock.NewRows([]string{"unlocked"}).AddRow(true))

	owner, acquired, err := (&schedulerOwnershipRepository{db: db, checkInterval: time.Hour}).TryAcquire(context.Background())
	require.NoError(t, err)
	require.True(t, acquired)

	const callers = 16
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = owner.Close()
		}(i)
	}
	wg.Wait()
	for _, closeErr := range errs {
		require.NoError(t, closeErr)
	}
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSchedulerOwnershipUnlockFailureDiscardsSession(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT pg_try_advisory_lock($1, $2)`)).
		WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(true))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT pg_advisory_unlock($1, $2)`)).
		WillReturnError(errors.New("unlock failed"))
	mock.ExpectClose()

	owner, acquired, err := (&schedulerOwnershipRepository{db: db, checkInterval: time.Hour}).TryAcquire(context.Background())
	require.NoError(t, err)
	require.True(t, acquired)
	firstErr := owner.Close()
	require.ErrorContains(t, firstErr, "unlock failed")
	require.Equal(t, firstErr, owner.Close())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSchedulerOwnershipConnectionLossIsSurfacedAndDiscarded(t *testing.T) {
	lostErr := errors.New("connection lost")
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT pg_try_advisory_lock($1, $2)`)).
		WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(true))
	mock.ExpectPing().WillReturnError(lostErr)
	mock.ExpectClose()

	owner, acquired, err := (&schedulerOwnershipRepository{db: db, checkInterval: time.Millisecond}).TryAcquire(context.Background())
	require.NoError(t, err)
	require.True(t, acquired)
	select {
	case <-owner.Lost():
	case <-time.After(time.Second):
		t.Fatal("ownership loss was not bounded")
	}
	require.ErrorContains(t, owner.Err(), lostErr.Error())
	require.ErrorContains(t, owner.Close(), lostErr.Error())
	require.NoError(t, mock.ExpectationsWereMet())
}

func lockArg(v int32) driver.Value { return int64(v) }
