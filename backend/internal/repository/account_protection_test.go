package repository

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

func TestPreserveLockedAccountProtectionKeepsCommittedPolicy(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })

	committed := []byte(`{
		"anti_degradation": true,
		"protection_scope": "codex_v3",
		"codex_fingerprint_mode": "device",
		"codex_fingerprint_seed": "11111111-1111-4111-8111-111111111111",
		"enable_tls_fingerprint": false,
		"anti_degrade": {
			"enabled": true,
			"mode": "mode1",
			"policy_version": 3,
			"max_concurrency": 100,
			"request_integrity": "strict"
		}
	}`)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT extra FROM accounts WHERE id = $1 AND deleted_at IS NULL")).
		WithArgs(int64(27)).
		WillReturnRows(sqlmock.NewRows([]string{"extra"}).AddRow(committed))

	account := &service.Account{
		ID:          27,
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeOAuth,
		Concurrency: 1,
		Extra: map[string]any{
			"unrelated": true,
		},
	}

	err = preserveLockedAccountProtection(context.Background(), client, account)

	require.NoError(t, err)
	require.Equal(t, true, account.Extra[service.AntiDegradationExtraKey])
	require.Equal(t, "codex_v3", account.Extra[service.ProtectionScopeExtraKey])
	require.Equal(t, "device", account.Extra["codex_fingerprint_mode"])
	require.Equal(t, true, account.Extra["unrelated"])
	require.Equal(t, 1, account.Concurrency)
	require.NoError(t, mock.ExpectationsWereMet())
}
