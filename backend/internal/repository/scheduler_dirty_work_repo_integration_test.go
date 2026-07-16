//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSchedulerSourcePromotionDerivesTargetsAndDrainsEvidence(t *testing.T) {
	ctx := context.Background()
	_, err := integrationDB.ExecContext(ctx, `TRUNCATE scheduler_dirty_account_sources, scheduler_dirty_group_sources, scheduler_dirty_membership_sources, scheduler_dirty_work`)
	require.NoError(t, err)
	suffix := time.Now().UnixNano()
	var accountID, groupID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `INSERT INTO groups(name) VALUES($1) RETURNING id`, fmt.Sprintf("promote-group-%d", suffix)).Scan(&groupID))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `INSERT INTO accounts(name,platform,type) VALUES($1,'openai','oauth') RETURNING id`, fmt.Sprintf("promote-account-%d", suffix)).Scan(&accountID))
	_, err = integrationDB.ExecContext(ctx, `INSERT INTO account_groups(account_id,group_id) VALUES($1,$2)`, accountID, groupID)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `TRUNCATE scheduler_dirty_account_sources, scheduler_dirty_group_sources, scheduler_dirty_membership_sources, scheduler_dirty_work`)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `UPDATE accounts SET priority=priority+1 WHERE id=$1`, accountID)
	require.NoError(t, err)

	owner, acquired, err := NewSchedulerOwnershipRepository(integrationDB).TryAcquire(ctx)
	require.NoError(t, err)
	require.True(t, acquired)
	t.Cleanup(func() { _ = owner.Close() })
	promoted, err := NewSchedulerDirtyWorkRepository(integrationDB).Promote(ctx, owner, 10)
	require.NoError(t, err)
	require.Equal(t, 1, promoted)

	for _, target := range []struct {
		kind int16
		id   int64
	}{{1, accountID}, {2, 0}, {2, groupID}} {
		var generation int64
		require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT generation FROM scheduler_dirty_work WHERE kind=$1 AND entity_id=$2`, target.kind, target.id).Scan(&generation))
		require.Equal(t, int64(1), generation)
	}
	var sources int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM scheduler_dirty_account_sources)+
		(SELECT count(*) FROM scheduler_dirty_group_sources)+
		(SELECT count(*) FROM scheduler_dirty_membership_sources)`).Scan(&sources))
	require.Zero(t, sources)
}

func TestSchedulerMembershipPromotionIncludesRemovedAndCurrentGroups(t *testing.T) {
	ctx := context.Background()
	_, err := integrationDB.ExecContext(ctx, `TRUNCATE scheduler_dirty_account_sources, scheduler_dirty_group_sources, scheduler_dirty_membership_sources, scheduler_dirty_work`)
	require.NoError(t, err)
	suffix := time.Now().UnixNano()
	var accountID, oldGroupID, currentGroupID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `INSERT INTO groups(name) VALUES($1) RETURNING id`, fmt.Sprintf("removed-group-%d", suffix)).Scan(&oldGroupID))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `INSERT INTO groups(name) VALUES($1) RETURNING id`, fmt.Sprintf("current-group-%d", suffix)).Scan(&currentGroupID))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `INSERT INTO accounts(name,platform,type) VALUES($1,'openai','oauth') RETURNING id`, fmt.Sprintf("membership-account-%d", suffix)).Scan(&accountID))
	_, err = integrationDB.ExecContext(ctx, `INSERT INTO account_groups(account_id,group_id) VALUES($1,$2),($1,$3)`, accountID, oldGroupID, currentGroupID)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `TRUNCATE scheduler_dirty_account_sources, scheduler_dirty_group_sources, scheduler_dirty_membership_sources, scheduler_dirty_work`)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `DELETE FROM account_groups WHERE account_id=$1 AND group_id=$2`, accountID, oldGroupID)
	require.NoError(t, err)

	owner, acquired, err := NewSchedulerOwnershipRepository(integrationDB).TryAcquire(ctx)
	require.NoError(t, err)
	require.True(t, acquired)
	t.Cleanup(func() { _ = owner.Close() })
	promoted, err := NewSchedulerDirtyWorkRepository(integrationDB).Promote(ctx, owner, 10)
	require.NoError(t, err)
	require.Equal(t, 1, promoted)

	for _, target := range []struct {
		kind int16
		id   int64
	}{{1, accountID}, {2, 0}, {2, oldGroupID}, {2, currentGroupID}} {
		var count int
		require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT count(*) FROM scheduler_dirty_work WHERE kind=$1 AND entity_id=$2`, target.kind, target.id).Scan(&count))
		require.Equal(t, 1, count)
	}
}

func TestSchedulerAccountPromotionPagesGroupFanoutAndKeepsBucketIntent(t *testing.T) {
	ctx := context.Background()
	truncateSchedulerDirtyTables(t, integrationDB)
	suffix := time.Now().UnixNano()
	var accountID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `INSERT INTO accounts(name,platform,type) VALUES($1,'openai','oauth') RETURNING id`, fmt.Sprintf("paged-account-%d", suffix)).Scan(&accountID))
	groupIDs := make([]int64, 3)
	for i := range groupIDs {
		require.NoError(t, integrationDB.QueryRowContext(ctx, `INSERT INTO groups(name) VALUES($1) RETURNING id`, fmt.Sprintf("paged-group-%d-%d", suffix, i)).Scan(&groupIDs[i]))
		_, err := integrationDB.ExecContext(ctx, `INSERT INTO account_groups(account_id,group_id) VALUES($1,$2)`, accountID, groupIDs[i])
		require.NoError(t, err)
	}
	truncateSchedulerDirtyTables(t, integrationDB)
	_, err := integrationDB.ExecContext(ctx, `UPDATE accounts SET priority=priority+1 WHERE id=$1`, accountID)
	require.NoError(t, err)

	owner, acquired, err := NewSchedulerOwnershipRepository(integrationDB).TryAcquire(ctx)
	require.NoError(t, err)
	require.True(t, acquired)
	t.Cleanup(func() { _ = owner.Close() })
	repo := NewSchedulerDirtyWorkRepository(integrationDB)

	promoted, err := repo.Promote(ctx, owner, 1)
	require.NoError(t, err)
	require.Equal(t, 1, promoted)
	var cursor, generation int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT group_cursor,generation FROM scheduler_dirty_account_sources WHERE account_id=$1`, accountID).Scan(&cursor, &generation))
	require.Equal(t, groupIDs[0], cursor)
	require.Equal(t, int64(1), generation)

	// A concurrent producer restarts paging at zero and advances generation, so
	// the refreshed source cannot be removed using the previous observation.
	_, err = integrationDB.ExecContext(ctx, `UPDATE accounts SET concurrency=concurrency+1 WHERE id=$1`, accountID)
	require.NoError(t, err)
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT group_cursor,generation FROM scheduler_dirty_account_sources WHERE account_id=$1`, accountID).Scan(&cursor, &generation))
	require.Zero(t, cursor)
	require.Equal(t, int64(2), generation)

	for i := 0; i < len(groupIDs); i++ {
		promoted, err = repo.Promote(ctx, owner, 1)
		require.NoError(t, err)
		require.Equal(t, 1, promoted)
	}
	var sources int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT count(*) FROM scheduler_dirty_account_sources WHERE account_id=$1`, accountID).Scan(&sources))
	require.Zero(t, sources)

	for _, target := range append([]int64{0}, groupIDs...) {
		var rebuild bool
		require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT rebuild_buckets FROM scheduler_dirty_work WHERE kind=2 AND entity_id=$1`, target).Scan(&rebuild))
		require.True(t, rebuild)
	}
}

func TestSchedulerSourcePromotionServicesEveryKind(t *testing.T) {
	ctx := context.Background()
	truncateSchedulerDirtyTables(t, integrationDB)
	_, err := integrationDB.ExecContext(ctx, `
		INSERT INTO scheduler_dirty_account_sources(account_id) VALUES (910001);
		INSERT INTO scheduler_dirty_membership_sources(account_id,group_id) VALUES (910002,920002);
		INSERT INTO scheduler_dirty_group_sources(group_id) VALUES (920003)
	`)
	require.NoError(t, err)
	owner, acquired, err := NewSchedulerOwnershipRepository(integrationDB).TryAcquire(ctx)
	require.NoError(t, err)
	require.True(t, acquired)
	t.Cleanup(func() { _ = owner.Close() })

	promoted, err := NewSchedulerDirtyWorkRepository(integrationDB).Promote(ctx, owner, 1)
	require.NoError(t, err)
	require.Equal(t, 3, promoted)
	var sources int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM scheduler_dirty_account_sources)+
		(SELECT count(*) FROM scheduler_dirty_group_sources)+
		(SELECT count(*) FROM scheduler_dirty_membership_sources)`).Scan(&sources))
	require.Zero(t, sources)
}

func TestSchedulerSourcePromotionRequiresActiveOwnership(t *testing.T) {
	repo := NewSchedulerDirtyWorkRepository(integrationDB)
	promoted, err := repo.Promote(context.Background(), nil, 10)
	require.Zero(t, promoted)
	require.ErrorContains(t, err, "active ownership")
}

func TestSchedulerDirtyWorkConcurrentGenerationPreservesDirtyKey(t *testing.T) {
	ctx := context.Background()
	_, err := integrationDB.ExecContext(ctx, "TRUNCATE scheduler_dirty_work")
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `
		INSERT INTO scheduler_dirty_work(kind, entity_id, generation)
		VALUES (1, 42, 1)
	`)
	require.NoError(t, err)

	repo := NewSchedulerDirtyWorkRepository(integrationDB)
	batch, err := repo.List(ctx, 1)
	require.NoError(t, err)
	require.Len(t, batch, 1)

	_, err = integrationDB.ExecContext(ctx, `
		INSERT INTO scheduler_dirty_work(kind, entity_id, generation, updated_at)
		VALUES (1, 42, 1, clock_timestamp())
		ON CONFLICT (kind, entity_id) DO UPDATE
		SET generation = scheduler_dirty_work.generation + 1,
			updated_at = EXCLUDED.updated_at
	`)
	require.NoError(t, err)

	acknowledged, err := repo.Acknowledge(ctx, batch[0])
	require.NoError(t, err)
	require.False(t, acknowledged)

	stats, err := repo.PendingStats(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), stats.Count)
	var generation int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT generation FROM scheduler_dirty_work WHERE kind = 1 AND entity_id = 42
	`).Scan(&generation))
	require.Equal(t, int64(2), generation)
}

func TestSchedulerOwnershipReleasedWithDedicatedConnection(t *testing.T) {
	ctx := context.Background()
	integrationDB.SetMaxOpenConns(1)
	integrationDB.SetMaxIdleConns(1)
	t.Cleanup(func() {
		integrationDB.SetMaxOpenConns(0)
		integrationDB.SetMaxIdleConns(2)
	})
	firstRepo := NewSchedulerOwnershipRepository(integrationDB)
	secondRepo := NewSchedulerOwnershipRepository(integrationDB)

	first, acquired, err := firstRepo.TryAcquire(ctx)
	require.NoError(t, err)
	require.True(t, acquired)
	firstOwner := first.(*postgresSchedulerOwnership)
	var firstPID int
	require.NoError(t, firstOwner.conn.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&firstPID))

	require.NoError(t, first.Close())

	second, acquired, err := secondRepo.TryAcquire(ctx)
	require.NoError(t, err)
	require.True(t, acquired)
	secondOwner := second.(*postgresSchedulerOwnership)
	var secondPID int
	require.NoError(t, secondOwner.conn.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&secondPID))
	require.Equal(t, firstPID, secondPID, "test must exercise same-session reuse")
	require.NoError(t, second.Close())
	requireSchedulerAdvisoryLockCount(t, 0)
}

func TestSchedulerOwnershipCloseActuallyUnlocksSession(t *testing.T) {
	ctx := context.Background()

	owner, acquired, err := NewSchedulerOwnershipRepository(integrationDB).TryAcquire(ctx)
	require.NoError(t, err)
	require.True(t, acquired)
	require.NoError(t, owner.Close())

	requireSchedulerAdvisoryLockCount(t, 0)
}

func TestSchedulerOwnershipConnectionLossIsBoundedAndLeaksNoLock(t *testing.T) {
	ctx := context.Background()
	repo := &schedulerOwnershipRepository{db: integrationDB, checkInterval: 10 * time.Millisecond}
	ownership, acquired, err := repo.TryAcquire(ctx)
	require.NoError(t, err)
	require.True(t, acquired)
	owner := ownership.(*postgresSchedulerOwnership)
	var pid int
	require.NoError(t, owner.conn.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&pid))

	var terminated bool
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT pg_terminate_backend($1)", pid).Scan(&terminated))
	require.True(t, terminated)
	select {
	case <-owner.Lost():
	case <-time.After(schedulerOwnershipProbeTimeout + time.Second):
		t.Fatal("connection loss was not surfaced within the probe bound")
	}
	require.ErrorContains(t, owner.Err(), "connection lost")
	require.Error(t, owner.Close())
	requireSchedulerAdvisoryLockCount(t, 0)
}

func requireSchedulerAdvisoryLockCount(t *testing.T, want int) {
	t.Helper()
	var held int
	require.NoError(t, integrationDB.QueryRowContext(context.Background(), `
		SELECT count(*)
		FROM pg_locks
		WHERE locktype = 'advisory'
		  AND classid = $1::bigint
		  AND objid = $2::bigint
		  AND granted
	`, int64(uint32(schedulerAdvisoryLockNamespace)), int64(uint32(schedulerAdvisoryLockID))).Scan(&held))
	require.Equal(t, want, held)
}
