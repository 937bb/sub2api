//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpsRepositoryBatchInsertErrorLogs(t *testing.T) {
	ctx := context.Background()
	_, _ = integrationDB.ExecContext(ctx, "TRUNCATE ops_error_logs RESTART IDENTITY")

	repo := NewOpsRepository(integrationDB).(*opsRepository)
	now := time.Now().UTC()
	inserted, err := repo.BatchInsertErrorLogs(ctx, []*service.OpsInsertErrorLogInput{
		{
			RequestID:    "batch-ops-1",
			ErrorPhase:   "upstream",
			ErrorType:    "upstream_error",
			Severity:     "error",
			StatusCode:   429,
			ErrorMessage: "rate limited",
			CreatedAt:    now,
		},
		{
			RequestID:    "batch-ops-2",
			ErrorPhase:   "internal",
			ErrorType:    "api_error",
			Severity:     "error",
			StatusCode:   500,
			ErrorMessage: "internal error",
			CreatedAt:    now.Add(time.Millisecond),
		},
	})
	require.NoError(t, err)
	require.EqualValues(t, 2, inserted)

	var count int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM ops_error_logs WHERE request_id IN ('batch-ops-1', 'batch-ops-2')").Scan(&count))
	require.Equal(t, 2, count)
}

func TestOpsRepositoryBatchInsertSystemLogs(t *testing.T) {
	ctx := context.Background()
	_, _ = integrationDB.ExecContext(ctx, "TRUNCATE ops_system_logs RESTART IDENTITY")

	repo := NewOpsRepository(integrationDB).(*opsRepository)
	now := time.Now().UTC()
	userID := int64(101)
	accountID := int64(202)

	inputs := []*service.OpsInsertSystemLogInput{
		nil,
		{Level: "info", Component: "worker", Message: ""},
		{Level: "", Component: "worker", Message: "missing level"},
		{
			CreatedAt:       now,
			Level:           " WARN ",
			Component:       " ",
			Message:         " first message ",
			RequestID:       "system-batch-first",
			ClientRequestID: "client-system-batch-first",
			UserID:          &userID,
			AccountID:       &accountID,
			Platform:        "claude",
			Model:           "sonnet",
			ExtraJSON:       `{"kind":"first"}`,
		},
	}
	for i := 0; i < 501; i++ {
		inputs = append(inputs, &service.OpsInsertSystemLogInput{
			CreatedAt: now.Add(time.Duration(i+1) * time.Millisecond),
			Level:     "info",
			Component: "batch",
			Message:   fmt.Sprintf("batch message %03d", i),
			RequestID: fmt.Sprintf("system-batch-%03d", i),
		})
	}

	inserted, err := repo.BatchInsertSystemLogs(ctx, inputs)
	require.NoError(t, err)
	require.EqualValues(t, 502, inserted)

	var count int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM ops_system_logs").Scan(&count))
	require.Equal(t, 502, count)

	var level, component, message, extraKind string
	var gotUserID, gotAccountID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT level, component, message, user_id, account_id, extra->>'kind'
		FROM ops_system_logs
		WHERE request_id = 'system-batch-first'
	`).Scan(&level, &component, &message, &gotUserID, &gotAccountID, &extraKind))
	require.Equal(t, "warn", level)
	require.Equal(t, "app", component)
	require.Equal(t, "first message", message)
	require.Equal(t, userID, gotUserID)
	require.Equal(t, accountID, gotAccountID)
	require.Equal(t, "first", extraKind)

	var defaultExtra string
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT extra::text
		FROM ops_system_logs
		WHERE request_id = 'system-batch-000'
	`).Scan(&defaultExtra))
	require.Equal(t, `{}`, defaultExtra)
}

func TestEnqueueSchedulerOutbox_DeduplicatesIdempotentEvents(t *testing.T) {
	ctx := context.Background()
	_, _ = integrationDB.ExecContext(ctx, "TRUNCATE scheduler_outbox RESTART IDENTITY")

	accountID := int64(12345)
	require.NoError(t, enqueueSchedulerOutbox(ctx, integrationDB, service.SchedulerOutboxEventAccountChanged, &accountID, nil, nil))
	require.NoError(t, enqueueSchedulerOutbox(ctx, integrationDB, service.SchedulerOutboxEventAccountChanged, &accountID, nil, nil))

	var count int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM scheduler_outbox WHERE event_type = $1", service.SchedulerOutboxEventAccountChanged).Scan(&count))
	require.Equal(t, 1, count)

	time.Sleep(schedulerOutboxDedupWindow + 150*time.Millisecond)
	require.NoError(t, enqueueSchedulerOutbox(ctx, integrationDB, service.SchedulerOutboxEventAccountChanged, &accountID, nil, nil))
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM scheduler_outbox WHERE event_type = $1", service.SchedulerOutboxEventAccountChanged).Scan(&count))
	require.Equal(t, 2, count)
}

func TestEnqueueSchedulerOutbox_DoesNotDeduplicateLastUsed(t *testing.T) {
	ctx := context.Background()
	_, _ = integrationDB.ExecContext(ctx, "TRUNCATE scheduler_outbox RESTART IDENTITY")

	accountID := int64(67890)
	payload1 := map[string]any{"last_used": map[string]int64{"67890": 100}}
	payload2 := map[string]any{"last_used": map[string]int64{"67890": 200}}
	require.NoError(t, enqueueSchedulerOutbox(ctx, integrationDB, service.SchedulerOutboxEventAccountLastUsed, &accountID, nil, payload1))
	require.NoError(t, enqueueSchedulerOutbox(ctx, integrationDB, service.SchedulerOutboxEventAccountLastUsed, &accountID, nil, payload2))

	var count int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM scheduler_outbox WHERE event_type = $1", service.SchedulerOutboxEventAccountLastUsed).Scan(&count))
	require.Equal(t, 2, count)
}
