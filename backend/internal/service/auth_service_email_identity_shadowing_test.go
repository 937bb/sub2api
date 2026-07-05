//go:build unit

package service

import (
	"context"
	"database/sql"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func TestEnsureEmailAuthIdentitySurfacesCreateError(t *testing.T) {
	logSink, restoreLogs := captureStructuredLog(t)
	defer restoreLogs()

	db, err := sql.Open("sqlite", "file:auth_service_email_identity_shadowing?mode=memory&cache=shared")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	svc := &AuthService{entClient: client}
	identity, created := svc.ensureEmailAuthIdentity(context.Background(), &User{
		ID:    424242,
		Email: "shadowing@example.com",
	}, "test_create_error")

	require.Nil(t, identity)
	require.False(t, created)
	require.True(t, logSink.ContainsMessage("Failed to ensure email auth identity"))
	require.True(t, logSink.ContainsMessage("constraint failed"))
}
