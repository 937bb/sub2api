package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestLegacySIWCRepairRejectsConcurrentGrantOrProxyChange(t *testing.T) {
	for _, affected := range []int64{0, 1} {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		t.Cleanup(func() { _ = db.Close() })
		repo := &accountRepository{sql: db}
		account := &service.Account{ID: 97, Platform: "openai", Type: "oauth", Credentials: map[string]any{"client_id": "oaiapp_old"}}
		mock.ExpectExec(`(?s)WITH updated AS .*credentials = credentials \|\| \$1::jsonb.*credentials = \$3::jsonb AND proxy_id IS NOT DISTINCT FROM \$4.*INSERT INTO scheduler_outbox`).
			WithArgs(`{"auth_mode":"siwc"}`, int64(97), `{"client_id":"oaiapp_old"}`, nil, service.SchedulerOutboxEventAccountChanged).WillReturnResult(sqlmock.NewResult(0, affected))
		ok, err := repo.RepairLegacyOpenAISiwc(context.Background(), account, map[string]any{"auth_mode": "siwc"})
		require.NoError(t, err)
		require.Equal(t, affected == 1, ok)
		require.NoError(t, mock.ExpectationsWereMet())
	}
}
