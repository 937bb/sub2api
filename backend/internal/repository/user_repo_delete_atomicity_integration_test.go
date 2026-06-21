//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// TestUserRepository_DeleteUser_AtomicWithAPIKeys covers the production wiring
// where repositories hold the base ent client and an outer service transaction
// is passed through context. Delete must join that transaction instead of
// opening and committing an independent one.
func TestUserRepository_DeleteUser_AtomicWithAPIKeys(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)

	userRepo := NewUserRepository(client, integrationDB)
	apiKeyRepo := newAPIKeyRepositoryWithSQL(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{})
	key1 := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: fmt.Sprintf("sk-atomic-a-%d", user.ID)})
	key2 := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: fmt.Sprintf("sk-atomic-b-%d", user.ID)})

	t.Cleanup(func() {
		_, _ = integrationDB.Exec(`DELETE FROM deleted_api_key_audits WHERE user_id = $1`, user.ID)
		_, _ = integrationDB.Exec(`DELETE FROM api_keys WHERE user_id = $1`, user.ID)
		_, _ = integrationDB.Exec(`DELETE FROM users WHERE id = $1`, user.ID)
	})

	listParams := pagination.PaginationParams{Page: 1, PageSize: 10}

	tx, err := client.Tx(ctx)
	require.NoError(t, err, "begin outer tx")
	opCtx := dbent.NewTxContext(ctx, tx)

	require.NoError(t, userRepo.Delete(opCtx, user.ID))
	deletedKeys, err := apiKeyRepo.DeleteByUserIDWithAudit(opCtx, user.ID)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{key1.Key, key2.Key}, deletedKeys)

	require.NoError(t, tx.Rollback(), "rollback outer tx")

	gotUser, err := userRepo.GetByID(ctx, user.ID)
	require.NoError(t, err, "rollback must restore user")
	require.Equal(t, user.ID, gotUser.ID)

	keys, _, err := apiKeyRepo.ListByUserID(ctx, user.ID, listParams, service.APIKeyListFilters{})
	require.NoError(t, err, "ListByUserID")
	require.Len(t, keys, 2, "rollback must restore API keys")

	var auditCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM deleted_api_key_audits WHERE user_id = $1`, user.ID).Scan(&auditCount))
	require.Zero(t, auditCount, "rollback must restore audit rows")

	tx2, err := client.Tx(ctx)
	require.NoError(t, err, "begin outer tx #2")
	opCtx2 := dbent.NewTxContext(ctx, tx2)

	require.NoError(t, userRepo.Delete(opCtx2, user.ID))
	deletedKeys, err = apiKeyRepo.DeleteByUserIDWithAudit(opCtx2, user.ID)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{key1.Key, key2.Key}, deletedKeys)

	require.NoError(t, tx2.Commit(), "commit outer tx")

	_, err = userRepo.GetByID(ctx, user.ID)
	require.Error(t, err, "committed delete should hide user")

	keysAfter, _, err := apiKeyRepo.ListByUserID(ctx, user.ID, listParams, service.APIKeyListFilters{})
	require.NoError(t, err, "ListByUserID")
	require.Empty(t, keysAfter, "committed delete should hide API keys")

	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM deleted_api_key_audits WHERE user_id = $1`, user.ID).Scan(&auditCount))
	require.Equal(t, 2, auditCount, "committed delete should audit every key")
}
