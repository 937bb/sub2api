//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestListSchedulableAccountLoadsMatchesListSchedulable(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := newAccountRepositoryWithSQL(client, tx, nil)
	now := time.Now()
	past, future := now.Add(-time.Hour), now.Add(time.Hour)

	create := func(name string) *service.Account {
		return mustCreateAccount(t, client, &service.Account{Name: name, Schedulable: true})
	}
	positive := create("projection-positive")
	_, err := client.Account.UpdateOneID(positive.ID).SetConcurrency(2).SetLoadFactor(9).SetPriority(30).Save(ctx)
	require.NoError(t, err)
	nilFactor := create("projection-nil")
	_, err = client.Account.UpdateOneID(nilFactor.ID).SetConcurrency(4).SetPriority(10).Save(ctx)
	require.NoError(t, err)
	zero := create("projection-zero")
	_, err = client.Account.UpdateOneID(zero.ID).SetConcurrency(0).SetLoadFactor(0).SetPriority(20).Save(ctx)
	require.NoError(t, err)

	excluded := make([]*service.Account, 0, 6)
	disabled := create("projection-disabled")
	_, err = client.Account.UpdateOneID(disabled.ID).SetStatus(service.StatusDisabled).Save(ctx)
	require.NoError(t, err)
	excluded = append(excluded, disabled)
	unschedulable := create("projection-unschedulable")
	_, err = client.Account.UpdateOneID(unschedulable.ID).SetSchedulable(false).Save(ctx)
	require.NoError(t, err)
	excluded = append(excluded, unschedulable)
	expired := create("projection-expired")
	_, err = client.Account.UpdateOneID(expired.ID).SetExpiresAt(past).SetAutoPauseOnExpired(true).Save(ctx)
	require.NoError(t, err)
	excluded = append(excluded, expired)
	overloaded := create("projection-overloaded")
	_, err = client.Account.UpdateOneID(overloaded.ID).SetOverloadUntil(future).Save(ctx)
	require.NoError(t, err)
	excluded = append(excluded, overloaded)
	rateLimited := create("projection-rate-limited")
	_, err = client.Account.UpdateOneID(rateLimited.ID).SetRateLimitResetAt(future).Save(ctx)
	require.NoError(t, err)
	excluded = append(excluded, rateLimited)
	tempBlocked := create("projection-temp-blocked")
	_, err = client.Account.UpdateOneID(tempBlocked.ID).SetTempUnschedulableUntil(future).Save(ctx)
	require.NoError(t, err)
	excluded = append(excluded, tempBlocked)

	accounts, err := repo.ListSchedulable(ctx)
	require.NoError(t, err)
	loads, err := repo.ListSchedulableAccountLoads(ctx)
	require.NoError(t, err)

	wantIDs := make([]int64, 0, len(accounts))
	wantLoads := make(map[int64]int, len(accounts))
	for i := range accounts {
		wantIDs = append(wantIDs, accounts[i].ID)
		wantLoads[accounts[i].ID] = accounts[i].EffectiveLoadFactor()
	}
	gotIDs := make([]int64, 0, len(loads))
	gotLoads := make(map[int64]int, len(loads))
	for _, load := range loads {
		gotIDs = append(gotIDs, load.ID)
		gotLoads[load.ID] = load.MaxConcurrency
	}
	require.Equal(t, wantIDs, gotIDs)
	require.Equal(t, wantLoads, gotLoads)
	require.Equal(t, 4, gotLoads[nilFactor.ID])
	require.Equal(t, 1, gotLoads[zero.ID])
	require.Equal(t, 9, gotLoads[positive.ID])
	requireOrderedSubset(t, gotIDs, []int64{nilFactor.ID, zero.ID, positive.ID})
	for _, account := range excluded {
		require.NotContains(t, gotLoads, account.ID)
	}
}

func requireOrderedSubset(t *testing.T, ids, want []int64) {
	t.Helper()
	positions := make(map[int64]int, len(ids))
	for i, id := range ids {
		positions[id] = i
	}
	for i := 1; i < len(want); i++ {
		require.Less(t, positions[want[i-1]], positions[want[i]])
	}
}
