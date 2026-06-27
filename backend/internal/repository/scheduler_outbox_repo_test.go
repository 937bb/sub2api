package repository

import (
	"context"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestSchedulerOutboxRepositoryListAfterAndReleaseDedupReleasesStrandedKeys(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := &schedulerOutboxRepository{db: db}
	mock.ExpectQuery(regexp.QuoteMeta(`
			WITH stranded AS (
				UPDATE scheduler_outbox
				SET dedup_key = NULL
				WHERE id <= $1
					AND dedup_key IS NOT NULL
				RETURNING id
			), selected AS MATERIALIZED (
				SELECT id, event_type, account_id, group_id, payload, created_at
				FROM scheduler_outbox
				WHERE id > $1
				ORDER BY id ASC
				LIMIT $2
				FOR UPDATE
			), released AS (
				UPDATE scheduler_outbox AS o
				SET dedup_key = NULL
				FROM selected AS s
				WHERE o.id = s.id
					AND o.dedup_key IS NOT NULL
				RETURNING o.id
			)
			SELECT s.id, s.event_type, s.account_id, s.group_id, s.payload, s.created_at
			FROM selected AS s
			CROSS JOIN (SELECT COUNT(*) FROM stranded) AS stranded_barrier
			CROSS JOIN (SELECT COUNT(*) FROM released) AS release_barrier
			ORDER BY s.id ASC
		`)).
		WithArgs(int64(42), 100).
		WillReturnRows(sqlmock.NewRows([]string{"id", "event_type", "account_id", "group_id", "payload", "created_at"}))

	events, err := repo.ListAfterAndReleaseDedup(context.Background(), 42, 100)

	require.NoError(t, err)
	require.Empty(t, events)
	require.NoError(t, mock.ExpectationsWereMet())
}

// buildSchedulerGroupPayload must return untyped nil for empty groups. Otherwise
// enqueueSchedulerOutbox would marshal a typed-nil map as JSON null and produce
// a different dedup key than literal nil for the same ungrouped-account event.
func TestEnqueueSchedulerOutbox_DedupConflictRefreshesLaggingRow(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectExec(regexp.QuoteMeta(`
				INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload, dedup_key)
				VALUES ($1, $2, $3, $4, $5)
				ON CONFLICT (dedup_key) WHERE dedup_key IS NOT NULL DO UPDATE
				SET id = EXCLUDED.id,
					event_type = EXCLUDED.event_type,
					account_id = EXCLUDED.account_id,
					group_id = EXCLUDED.group_id,
					payload = EXCLUDED.payload,
					created_at = EXCLUDED.created_at
				WHERE scheduler_outbox.id < (SELECT COALESCE(MAX(id), 0) FROM scheduler_outbox)
			`)).
		WithArgs("account_changed", nil, nil, nil, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = enqueueSchedulerOutbox(context.Background(), db, "account_changed", nil, nil, nil)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestEnqueueSchedulerOutbox_UngroupedAccountDedupesWithLiteralNilPayload(t *testing.T) {
	accountID := int64(42)

	keyLiteralNil := schedulerOutboxDedupKey("account_changed", &accountID, nil, nil)

	emptyGroupsPayload := buildSchedulerGroupPayload(nil)
	require.Nil(t, emptyGroupsPayload,
		"buildSchedulerGroupPayload(empty) must return untyped-nil any to avoid typed-nil marshal")

	var payloadJSON []byte
	if emptyGroupsPayload != nil {
		t.Fatalf("typed-nil regression: buildSchedulerGroupPayload(empty) interface should be nil")
	}
	keyEmptyGroups := schedulerOutboxDedupKey("account_changed", &accountID, nil, payloadJSON)

	require.Equal(t, keyLiteralNil, keyEmptyGroups,
		"ungrouped-account account_changed must share dedup_key with other nil-payload variants")
}
