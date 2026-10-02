package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUsageBillingAPIKeyEffectsIncludeSoftDeletedTombstone(t *testing.T) {
	t.Run("quota", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectBegin()
		tx, err := db.BeginTx(context.Background(), nil)
		require.NoError(t, err)
		mock.ExpectQuery(`WHERE id = \$2 RETURNING`).
			WithArgs(1.5, int64(7), service.StatusAPIKeyActive, service.StatusAPIKeyQuotaExhausted).
			WillReturnRows(sqlmock.NewRows([]string{"exhausted"}).AddRow(false))

		exhausted, err := incrementUsageBillingAPIKeyQuota(context.Background(), tx, 7, 1.5)
		require.NoError(t, err)
		require.False(t, exhausted)

		mock.ExpectRollback()
		require.NoError(t, tx.Rollback())
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("rate limits", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectBegin()
		tx, err := db.BeginTx(context.Background(), nil)
		require.NoError(t, err)
		mock.ExpectExec(`WHERE id = \$2$`).
			WithArgs(1.5, int64(7)).
			WillReturnResult(sqlmock.NewResult(0, 1))

		require.NoError(t, incrementUsageBillingAPIKeyRateLimit(context.Background(), tx, 7, 1.5))

		mock.ExpectRollback()
		require.NoError(t, tx.Rollback())
		require.NoError(t, mock.ExpectationsWereMet())
	})
}
