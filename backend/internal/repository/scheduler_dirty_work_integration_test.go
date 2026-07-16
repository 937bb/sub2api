//go:build integration

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSchedulerDirtySourceTriggersAreLocalAtomicAndCoalescing(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)

	var accountID, oldGroupID, newGroupID int64
	suffix := time.Now().UnixNano()
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO groups (name) VALUES ($1) RETURNING id
`, fmt.Sprintf("dirty-old-%d", suffix)).Scan(&oldGroupID))
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO groups (name) VALUES ($1) RETURNING id
`, fmt.Sprintf("dirty-new-%d", suffix)).Scan(&newGroupID))
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO accounts (name, platform, type) VALUES ($1, 'openai', 'oauth') RETURNING id
`, fmt.Sprintf("dirty-account-%d", suffix)).Scan(&accountID))
	_, err := tx.ExecContext(ctx, `
INSERT INTO account_groups (account_id, group_id) VALUES ($1, $2)
`, accountID, oldGroupID)
	require.NoError(t, err)
	truncateSchedulerDirtyTables(t, tx)

	// Bucket-affecting writes accumulate a sticky bit on the account identity.
	_, err = tx.ExecContext(ctx, "UPDATE accounts SET priority = priority + 1 WHERE id = $1", accountID)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, "UPDATE accounts SET concurrency = concurrency + 1 WHERE id = $1", accountID)
	require.NoError(t, err)
	requireAccountSource(t, tx, accountID, 2, true)
	requireNoCanonicalDirty(t, tx)

	// Moving a membership preserves both OLD and NEW pair identities.
	_, err = tx.ExecContext(ctx, `
UPDATE account_groups SET group_id = $1 WHERE account_id = $2 AND group_id = $3
`, newGroupID, accountID, oldGroupID)
	require.NoError(t, err)
	requireMembershipSource(t, tx, accountID, oldGroupID, 1)
	requireMembershipSource(t, tx, accountID, newGroupID, 1)
	requireNoCanonicalDirty(t, tx)

	// Observational last-used writes stay off the source hot path; runtime
	// last-used publication is monotonic and handled separately.
	truncateSchedulerDirtyTables(t, tx)
	usedAt := time.Now().UTC().Add(time.Hour)
	_, err = tx.ExecContext(ctx, "UPDATE accounts SET last_used_at = $1 WHERE id = $2", usedAt, accountID)
	require.NoError(t, err)
	var sourceCount int
	require.NoError(t, tx.QueryRowContext(ctx, "SELECT count(*) FROM scheduler_dirty_account_sources WHERE account_id=$1", accountID).Scan(&sourceCount))
	require.Zero(t, sourceCount)

	// Scheduler metadata changes still request account refresh without forcing a
	// bucket rebuild. Explicit administrative last-used clear remains valid.
	_, err = tx.ExecContext(ctx, "UPDATE accounts SET concurrency = concurrency + 1 WHERE id = $1", accountID)
	require.NoError(t, err)
	requireAccountSource(t, tx, accountID, 1, false)
	_, err = tx.ExecContext(ctx, "UPDATE accounts SET last_used_at = NULL WHERE id = $1", accountID)
	require.NoError(t, err)
	var stored *time.Time
	require.NoError(t, tx.QueryRowContext(ctx, "SELECT last_used_at FROM accounts WHERE id = $1", accountID).Scan(&stored))
	require.Nil(t, stored)

	// Business rollback also rolls back source-local evidence.
	truncateSchedulerDirtyTables(t, tx)
	_, err = tx.ExecContext(ctx, "SAVEPOINT scheduler_dirty_rollback")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, "UPDATE accounts SET priority = priority + 1 WHERE id = $1", accountID)
	require.NoError(t, err)
	requireAccountSource(t, tx, accountID, 1, true)
	_, err = tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT scheduler_dirty_rollback")
	require.NoError(t, err)
	sourceCount = 0
	require.NoError(t, tx.QueryRowContext(ctx, "SELECT count(*) FROM scheduler_dirty_account_sources WHERE account_id=$1", accountID).Scan(&sourceCount))
	require.Zero(t, sourceCount)
}

func TestSchedulerGroupPrimaryKeyUpdatePreservesOldAndNewSources(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)
	suffix := time.Now().UnixNano()
	var oldID int64
	require.NoError(t, tx.QueryRowContext(ctx, `INSERT INTO groups(name) VALUES($1) RETURNING id`, fmt.Sprintf("group-pk-%d", suffix)).Scan(&oldID))
	truncateSchedulerDirtyTables(t, tx)
	newID := oldID + 1000000
	_, err := tx.ExecContext(ctx, `UPDATE groups SET id=$1 WHERE id=$2`, newID, oldID)
	require.NoError(t, err)
	for _, id := range []int64{oldID, newID} {
		var generation int64
		require.NoError(t, tx.QueryRowContext(ctx, `SELECT generation FROM scheduler_dirty_group_sources WHERE group_id=$1`, id).Scan(&generation))
		require.Equal(t, int64(1), generation)
	}
}

func TestSchedulerDirtySourceStatementUpdatesAreSortedAndCoalesced(t *testing.T) {
	ctx := context.Background()
	tx := testTx(t)
	suffix := time.Now().UnixNano()

	var firstID, secondID int64
	require.NoError(t, tx.QueryRowContext(ctx, `INSERT INTO accounts(name,platform,type) VALUES($1,'openai','oauth') RETURNING id`, fmt.Sprintf("statement-a-%d", suffix)).Scan(&firstID))
	require.NoError(t, tx.QueryRowContext(ctx, `INSERT INTO accounts(name,platform,type) VALUES($1,'openai','oauth') RETURNING id`, fmt.Sprintf("statement-b-%d", suffix)).Scan(&secondID))
	truncateSchedulerDirtyTables(t, tx)

	_, err := tx.ExecContext(ctx, `UPDATE accounts SET priority=priority+1 WHERE id IN ($1,$2)`, secondID, firstID)
	require.NoError(t, err)
	requireAccountSource(t, tx, firstID, 1, true)
	requireAccountSource(t, tx, secondID, 1, true)
}

func truncateSchedulerDirtyTables(t *testing.T, exec interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}) {
	t.Helper()
	_, err := exec.ExecContext(context.Background(), `TRUNCATE scheduler_dirty_account_sources, scheduler_dirty_group_sources, scheduler_dirty_membership_sources, scheduler_dirty_work`)
	require.NoError(t, err)
}

func requireAccountSource(t *testing.T, tx queryRower, accountID, generation int64, bucketDirty bool) {
	t.Helper()
	var gotGeneration int64
	var gotBucketDirty bool
	require.NoError(t, tx.QueryRowContext(context.Background(), `
SELECT generation,bucket_dirty FROM scheduler_dirty_account_sources WHERE account_id=$1
`, accountID).Scan(&gotGeneration, &gotBucketDirty))
	require.Equal(t, generation, gotGeneration)
	require.Equal(t, bucketDirty, gotBucketDirty)
}

func requireMembershipSource(t *testing.T, tx queryRower, accountID, groupID, generation int64) {
	t.Helper()
	var got int64
	require.NoError(t, tx.QueryRowContext(context.Background(), `
SELECT generation FROM scheduler_dirty_membership_sources WHERE account_id=$1 AND group_id=$2
`, accountID, groupID).Scan(&got))
	require.Equal(t, generation, got)
}

func requireNoCanonicalDirty(t *testing.T, tx queryRower) {
	t.Helper()
	var count int
	require.NoError(t, tx.QueryRowContext(context.Background(), `SELECT count(*) FROM scheduler_dirty_work`).Scan(&count))
	require.Zero(t, count)
}

type queryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}
