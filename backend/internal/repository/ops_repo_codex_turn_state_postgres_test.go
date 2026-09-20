package repository

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// TestCodexTurnStateScanPostgresBinding exercises the real PostgreSQL bind/type
// inference path. sqlmock cannot detect incompatible VARCHAR/TEXT deductions.
// The explicit DSN must name an isolated test database; no migrations run and
// every write targets a connection-local temporary table shadowing production.
func TestCodexTurnStateScanPostgresBinding(t *testing.T) {
	dsn := os.Getenv("SUB2API_STATE_TEST_DSN")
	if dsn == "" {
		t.Skip("set SUB2API_STATE_TEST_DSN to an isolated PostgreSQL test database")
	}
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	require.NoError(t, db.PingContext(ctx))
	_, err = db.ExecContext(ctx, `CREATE TEMPORARY TABLE codex_turn_state_scans (
  account_id BIGINT NOT NULL,
  model VARCHAR(255) NOT NULL,
  status VARCHAR(20) NOT NULL DEFAULT 'pending',
  attempt_count INT NOT NULL DEFAULT 0,
  last_proxy_id BIGINT,
  last_proxy_url TEXT NOT NULL DEFAULT '',
  last_state_length INT NOT NULL DEFAULT 0,
  last_error TEXT NOT NULL DEFAULT '',
  last_attempt_at TIMESTAMPTZ,
  last_success_at TIMESTAMPTZ,
  next_attempt_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  lease_id TEXT,
  lease_until TIMESTAMPTZ,
  PRIMARY KEY (account_id, model)
)`)
	require.NoError(t, err)
	// Do not fall back to public tables if the connection is ever replaced.
	_, err = db.ExecContext(ctx, `SET search_path TO pg_temp`)
	require.NoError(t, err)
	repo := &opsRepository{db: db}
	now := time.Now().UTC().Truncate(time.Microsecond)
	proxyID := int64(17)
	model := "gpt-5.5"
	claimed, err := repo.ClaimOpenAICodexTurnStateScan(ctx, 42, model, "owner-a", now.Add(time.Minute))
	require.NoError(t, err)
	require.True(t, claimed)
	scan := &service.OpenAICodexTurnStateScan{
		AccountID: 42, Model: model, Status: "running", LeaseID: "owner-a",
		AttemptCount: 1, LastAttemptAt: &now,
	}
	// This call reproduced pq's inconsistent types deduced for parameter $2
	// before the INSERT SELECT parameters received explicit schema-matched casts.
	require.NoError(t, repo.UpsertOpenAICodexTurnStateScan(ctx, scan))
	scan.Status = "ready"
	scan.LastStateLength = 332
	scan.LastProxyID = &proxyID
	scan.LastProxyURL = "http://127.0.0.1:8080"
	scan.LastSuccessAt = &now
	next := now.Add(30 * time.Minute)
	scan.NextAttemptAt = &next
	require.NoError(t, repo.UpsertOpenAICodexTurnStateScan(ctx, scan))
	stored, err := repo.GetOpenAICodexTurnStateScan(ctx, 42, model)
	require.NoError(t, err)
	require.Equal(t, "ready", stored.Status)
	require.Equal(t, 332, stored.LastStateLength)
	require.Equal(t, &proxyID, stored.LastProxyID)
	require.WithinDuration(t, next, *stored.NextAttemptAt, time.Microsecond)

	for _, leaseID := range []string{"owner-b", ""} {
		other := *scan
		other.LeaseID = leaseID
		other.Status = "failed"
		require.ErrorContains(t, repo.UpsertOpenAICodexTurnStateScan(ctx, &other), "lease lost")
	}
	claimed, err = repo.ClaimOpenAICodexTurnStateScan(ctx, 42, model, "owner-b", now.Add(time.Minute))
	require.NoError(t, err)
	require.False(t, claimed)

	_, err = db.ExecContext(ctx, `UPDATE pg_temp.codex_turn_state_scans SET lease_until = NOW() - INTERVAL '1 second' WHERE account_id = 42`)
	require.NoError(t, err)
	require.ErrorContains(t, repo.UpsertOpenAICodexTurnStateScan(ctx, scan), "lease lost")
	claimed, err = repo.ClaimOpenAICodexTurnStateScan(ctx, 42, model, "owner-b", now.Add(time.Minute))
	require.NoError(t, err)
	require.True(t, claimed)
	require.ErrorContains(t, repo.UpsertOpenAICodexTurnStateScan(ctx, scan), "lease lost")
	scan.LeaseID = "owner-b"
	require.NoError(t, repo.UpsertOpenAICodexTurnStateScan(ctx, scan))
	require.NoError(t, repo.ReleaseOpenAICodexTurnStateScan(ctx, 42, model, "owner-b"))
	scan.LeaseID = ""
	scan.Status = "retry_wait"
	require.NoError(t, repo.UpsertOpenAICodexTurnStateScan(ctx, scan))

	missing := &service.OpenAICodexTurnStateScan{AccountID: 84, Model: model, Status: "running", LeaseID: "never-claimed"}
	require.ErrorContains(t, repo.UpsertOpenAICodexTurnStateScan(ctx, missing), "lease lost")
	missing.LeaseID = ""
	require.NoError(t, repo.UpsertOpenAICodexTurnStateScan(ctx, missing))
	var count int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pg_temp.codex_turn_state_scans`).Scan(&count))
	require.Equal(t, 2, count)
}
