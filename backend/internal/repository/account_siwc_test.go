package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestSIWCCredentialCASPreservesConcurrentConfiguration(t *testing.T) {
	for _, affected := range []int64{0, 1} {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		t.Cleanup(func() { _ = db.Close() })
		repo := &accountRepository{sql: db}
		expected := &service.Account{ID: 97, Platform: "openai", Type: "oauth", Credentials: map[string]any{"auth_mode": "siwc", "subject": "user", "client_id": "oaiapp_test", "ext_agent_host_id": "host", "access_token": "old-at", "refresh_token": "old-rt"}}
		mock.ExpectExec(`(?s)WITH updated AS .*credentials = credentials \|\| \$1::jsonb.*credentials->>'subject' = \$3.*credentials->>'client_id' = \$4.*credentials->>'ext_agent_host_id' = \$5.*credentials->>'access_token' = \$6.*refresh_token.*proxy_id IS NOT DISTINCT FROM \$8.*INSERT INTO scheduler_outbox`).
			WithArgs(`{"access_token":"new-at"}`, int64(97), "user", "oaiapp_test", "host", "old-at", "old-rt", nil, service.SchedulerOutboxEventAccountChanged).
			WillReturnResult(sqlmock.NewResult(0, affected))
		ok, err := repo.UpdateOpenAISiwcCredentials(context.Background(), expected, map[string]any{"access_token": "new-at"})
		require.NoError(t, err)
		require.Equal(t, affected == 1, ok)
		require.NoError(t, mock.ExpectationsWereMet())
	}
}

func TestSIWCEmptyCatalogClearsOnlyEntitlements(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := &accountRepository{sql: db}
	mock.ExpectExec(`(?s)WITH updated AS .*jsonb_set\(credentials, '\{siwc_models\}', \$1::jsonb\).*credentials->>'subject' = \$3.*credentials->>'client_id' = \$4.*INSERT INTO scheduler_outbox`).
		WithArgs(`[]`, int64(97), "user", "oaiapp_test", service.SchedulerOutboxEventAccountChanged).WillReturnResult(sqlmock.NewResult(0, 1))
	ok, err := repo.UpdateOpenAISiwcModels(context.Background(), 97, "user", "oaiapp_test", nil)
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, mock.ExpectationsWereMet())
}
